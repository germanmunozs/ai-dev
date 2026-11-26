package watch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/germanmunozs/ai-dev/internal/fswatch"
)

const fakeProject = "/p"

// eventLog records reads and syncs in order, shared by the fakes.
type eventLog struct {
	mu      sync.Mutex
	entries []string
}

func (l *eventLog) add(entry string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, entry)
}

func (l *eventLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.entries)
}

// lockedFS is a fstest.MapFS that tests change while the loop reads it.
type lockedFS struct {
	mu sync.Mutex
	m  fstest.MapFS
}

func newLockedFS() *lockedFS { return &lockedFS{m: fstest.MapFS{}} }

func (l *lockedFS) set(name string, file *fstest.MapFile) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if file == nil {
		delete(l.m, name)
		return
	}
	l.m[name] = file
}

func (l *lockedFS) Open(name string) (fs.File, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.m.Open(name)
}

func (l *lockedFS) Lstat(name string) (fs.FileInfo, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.m.Lstat(name)
}

func (l *lockedFS) ReadLink(name string) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.m.ReadLink(name)
}

// fakeSource answers from maps the test changes; log records each read.
type fakeSource struct {
	mu       sync.Mutex
	features []string
	states   map[string]string // feature -> recognized state, or "" for state_invalid
	next     map[string]string // feature -> next outcome
	setup    string            // setup outcome
	checks   map[string]string // check id -> pass or fail
	err      error
	panicOn  string
	log      *eventLog
}

func newFakeSource(log *eventLog) *fakeSource {
	return &fakeSource{states: map[string]string{}, next: map[string]string{}, setup: "configured",
		checks: map[string]string{"git-tool": "pass"}, log: log}
}

func (s *fakeSource) Features() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.log.add("source:features")
	if s.panicOn == "features" {
		panic("source failed")
	}
	return slices.Clone(s.features), s.err
}

func (s *fakeSource) State(id string) StateResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.log.add("source:state:" + id)
	state, known := s.states[id]
	switch {
	case !known:
		return StateResult{JSON: []byte(fmt.Sprintf(`{"feature_id":%q,"outcome":"feature_unknown"}`+"\n", id)), Outcome: "feature_unknown"}
	case state == "":
		return StateResult{JSON: []byte(fmt.Sprintf(`{"feature_id":%q,"outcome":"state_invalid"}`+"\n", id)), Outcome: "state_invalid"}
	}
	return StateResult{JSON: []byte(fmt.Sprintf(`{"feature_id":%q,"state":%q,"outcome":"state_reported"}`+"\n", id, state)),
		State: state, Recognized: true, Outcome: "state_reported"}
}

func (s *fakeSource) Next(id string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	outcome := s.next[id]
	if outcome == "" {
		outcome = "feature_unknown"
		if _, known := s.states[id]; known {
			outcome = "step_recommended"
		}
	}
	return []byte(fmt.Sprintf(`{"feature_id":%q,"outcome":%q}`+"\n", id, outcome))
}

func (s *fakeSource) Setup() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return []byte(fmt.Sprintf(`{"schema_version":"ai-dev.setup/v1","command":"show","outcome":%q}`+"\n", s.setup))
}

func (s *fakeSource) Environment(context.Context) EnvironmentResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.checks))
	for id := range s.checks {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	result := EnvironmentResult{}
	var parts []string
	overall := "ready"
	for _, id := range ids {
		result.Checks = append(result.Checks, CheckStatus{ID: id, Status: s.checks[id], Remediation: "fix " + id})
		parts = append(parts, fmt.Sprintf(`{"id":%q,"status":%q}`, id, s.checks[id]))
		if s.checks[id] == "fail" {
			overall = "not_ready"
		}
	}
	result.JSON = []byte(fmt.Sprintf(`{"overall":%q,"checks":[%s]}`+"\n", overall, strings.Join(parts, ",")))
	return result
}

func (s *fakeSource) update(f func(*fakeSource)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(s)
}

// fakeWatcher delivers the changes the test sends and records every Sync.
type fakeWatcher struct {
	mu      sync.Mutex
	events  chan fswatch.Change
	syncs   [][]string
	syncErr error
	closed  bool
	log     *eventLog
}

func newFakeWatcher(log *eventLog) *fakeWatcher {
	return &fakeWatcher{events: make(chan fswatch.Change, 64), log: log}
}

func (w *fakeWatcher) Sync(dirs, files []string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.log.add("watcher:sync")
	w.syncs = append(w.syncs, append(slices.Clone(dirs), files...))
	return w.syncErr
}

func (w *fakeWatcher) Events() <-chan fswatch.Change { return w.events }

func (w *fakeWatcher) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.closed {
		w.closed = true
		close(w.events)
	}
	return nil
}

func (w *fakeWatcher) lastSync() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.syncs) == 0 {
		return nil
	}
	return w.syncs[len(w.syncs)-1]
}

// lineBuffer collects written lines safely.
type lineBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lineBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// decoded is one emitted JSON line.
type decoded struct {
	Seq  int64           `json:"seq"`
	Type Type            `json:"type"`
	Data json.RawMessage `json:"data"`
}

func (b *lineBuffer) events(t *testing.T) []decoded {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []decoded
	for _, line := range strings.Split(strings.TrimSuffix(b.buf.String(), "\n"), "\n") {
		if line == "" {
			continue
		}
		var d decoded
		if err := json.Unmarshal([]byte(line), &d); err != nil {
			t.Fatalf("invalid line %q: %v", line, err)
		}
		out = append(out, d)
	}
	return out
}

func (b *lineBuffer) types(t *testing.T) []Type {
	var types []Type
	for _, e := range b.events(t) {
		types = append(types, e.Type)
	}
	return types
}

// harness wires a loop to fakes.
type harness struct {
	fs      *lockedFS
	source  *fakeSource
	watcher *fakeWatcher
	out     *lineBuffer
	log     *eventLog
	config  Config
}

func newHarness() *harness {
	h := &harness{fs: newLockedFS(), out: &lineBuffer{}, log: &eventLog{}}
	h.source = newFakeSource(h.log)
	h.watcher = newFakeWatcher(h.log)
	h.config = Config{
		Source:     h.source,
		NewWatcher: func() (fswatch.Watcher, error) { return h.watcher, nil },
		Root:       h.fs,
		Project:    fakeProject,
		Output:     h.out,
		Encode:     EncodeJSON,
		Identity:   func() (string, error) { return "dev:1", nil },
	}
	return h
}

// addFeature creates specs/<id> with the given artifacts and a state.
func (h *harness) addFeature(id, state string, artifacts ...string) {
	h.fs.set("specs", &fstest.MapFile{Mode: fs.ModeDir | 0o755})
	h.fs.set("specs/"+id, &fstest.MapFile{Mode: fs.ModeDir | 0o755})
	for _, a := range artifacts {
		h.fs.set("specs/"+id+"/"+a, &fstest.MapFile{Data: []byte("# " + a + "\n"), Mode: 0o644})
	}
	h.source.update(func(s *fakeSource) {
		if !slices.Contains(s.features, id) {
			s.features = append(s.features, id)
			slices.Sort(s.features)
		}
		s.states[id] = state
	})
}

func defaultOptions() Options {
	return Options{Debounce: debounceDefault, Heartbeat: heartbeatDefault, EnvironmentInterval: environmentDefault}
}
