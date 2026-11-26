package watch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/germanmunozs/ai-dev/internal/fswatch"
)

// Options are the validated options of ai-dev watch (contracts/watch.md).
type Options struct {
	Features            []string
	Debounce            time.Duration
	Heartbeat           time.Duration
	EnvironmentInterval time.Duration // 0 disables revalidation after the snapshot
	Once                bool
}

const (
	debounceDefault    = 250 * time.Millisecond
	heartbeatDefault   = 15 * time.Second
	environmentDefault = 60 * time.Second
	// stopFlushTimeout bounds the wait to write watch_stopped (owner
	// decision 4, research R-07).
	stopFlushTimeout = 2 * time.Second
)

// Config wires the loop to its ports.
type Config struct {
	Source     Source
	NewWatcher func() (fswatch.Watcher, error)
	Root       fs.FS  // the project, read-only
	Project    string // the project's absolute path
	Output     io.Writer
	Encode     func(Event, int64) []byte
	// Identity names the project directory (device and inode); a different
	// value, or an error, means the project is gone.
	Identity func() (string, error)

	queueLimit int // QueueLimit when zero; tests set a smaller one
}

// Result is how the run ended.
type Result struct {
	ExitCode int
}

type loop struct {
	ctx      context.Context
	opts     Options
	cfg      Config
	obs      observer
	q        *queue
	watcher  fswatch.Watcher
	identity string
	picture  picture

	// pending maps each change key to the end of its debounce window; a
	// window opens at the first notification and never restarts (owner
	// decision 5, research R-04).
	pending map[string]time.Time
	timer   *time.Timer
}

// Change keys (data-model.md).
const (
	keyProject       = "project"
	keySpecs         = "specs"
	keySetup         = "setup"
	keyFeaturePrefix = "feature:"
)

// stopped ends the run with a result.
type stopped struct{ result Result }

// Run reports the project until ctx is cancelled or watching stops
// (F015-FR-005, F015-FR-015 to FR-018).
func Run(ctx context.Context, opts Options, cfg Config) (result Result) {
	limit := cfg.queueLimit
	if limit == 0 {
		limit = QueueLimit
	}
	l := &loop{ctx: ctx, opts: opts, cfg: cfg, q: newQueue(cfg.Output, cfg.Encode, limit),
		obs: observer{source: cfg.Source, root: cfg.Root, project: cfg.Project, filter: opts.Features}}
	defer l.q.Stop()
	defer func() {
		if recovered := recover(); recovered != nil {
			result = l.stop(Stopped{Reason: "internal_error", Detail: fmt.Sprint(recovered)}, 2)
		}
	}()
	if opts.Once {
		p, err := l.obs.build(ctx)
		if err != nil {
			return l.stop(unreadable(err), 1)
		}
		l.push(Snapshot, p.snapshot())
		return l.finish(0)
	}

	watcher, err := cfg.NewWatcher()
	if err != nil {
		return l.stop(unavailable(err), 1)
	}
	l.watcher = watcher
	defer watcher.Close()
	p, err := l.start()
	if err != nil {
		return l.stop(unavailable(err), 1)
	}
	l.picture, l.pending = p, map[string]time.Time{}

	for {
		var due <-chan time.Time
		if l.timer != nil {
			due = l.timer.C
		}
		var end *stopped
		select {
		case <-ctx.Done():
			return l.stop(Stopped{Reason: "signal"}, 0)
		case <-l.q.Err():
			return Result{ExitCode: 2}
		case c, ok := <-watcher.Events():
			if !ok {
				return l.stop(Stopped{Reason: "watch_unavailable", Detail: "watching stopped unexpectedly"}, 1)
			}
			end = l.notified(c)
		case <-due:
			l.timer = nil
			end = l.evaluateDue()
		}
		if end != nil {
			return end.result
		}
	}
}

// notified marks the key of one notification.
func (l *loop) notified(c fswatch.Change) *stopped {
	switch c.Kind {
	case fswatch.Overflow:
		return l.resync("watch_overflow")
	case fswatch.Gone:
		if l.keyOf(c.Path) == keyProject {
			return &stopped{l.stop(Stopped{Reason: "project_gone", Detail: l.cfg.Project + " was removed or moved"}, 1)}
		}
		l.mark(keySpecs) // a feature directory, specs or .ai-dev was removed
	}
	if key := l.keyOf(c.Path); key != "" {
		l.mark(key)
	}
	return nil
}

// keyOf maps an observed path to its change key.
func (l *loop) keyOf(path string) string {
	rel, err := filepath.Rel(l.cfg.Project, path)
	if err != nil {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	switch {
	case rel == ".":
		return keyProject
	case parts[0] == ".ai-dev":
		return keySetup
	case parts[0] == "specs" && len(parts) == 1:
		return keySpecs
	case parts[0] == "specs":
		return keyFeaturePrefix + parts[1]
	}
	return ""
}

func (l *loop) mark(key string) {
	if _, open := l.pending[key]; open {
		return
	}
	l.pending[key] = time.Now().Add(l.opts.Debounce)
	l.schedule()
}

// schedule sets the timer to the earliest window end.
func (l *loop) schedule() {
	if l.timer != nil {
		l.timer.Stop()
		l.timer = nil
	}
	var earliest time.Time
	for _, end := range l.pending {
		if earliest.IsZero() || end.Before(earliest) {
			earliest = end
		}
	}
	if !earliest.IsZero() {
		l.timer = time.NewTimer(time.Until(earliest))
	}
}

// evaluateDue evaluates every key whose window has closed, the project and
// the listings before single features, then observes the new watch set.
func (l *loop) evaluateDue() *stopped {
	now := time.Now()
	var keys []string
	for key, end := range l.pending {
		if !end.After(now) {
			keys = append(keys, key)
			delete(l.pending, key)
		}
	}
	defer l.schedule()
	if len(keys) == 0 {
		return nil
	}
	if id, err := l.cfg.Identity(); err != nil || id != l.identity {
		return &stopped{l.stop(Stopped{Reason: "project_gone", Detail: l.cfg.Project + " was removed or moved"}, 1)}
	}
	rank := func(key string) int {
		switch key {
		case keyProject:
			return 0
		case keySetup:
			return 1
		case keySpecs:
			return 2
		}
		return 3
	}
	slices.SortFunc(keys, func(a, b string) int {
		if c := rank(a) - rank(b); c != 0 {
			return c
		}
		return strings.Compare(a, b)
	})
	for _, key := range keys {
		switch {
		case key == keyProject:
			l.evaluateSetup()
			if end := l.evaluateSpecs(); end != nil {
				return end
			}
		case key == keySetup:
			l.evaluateSetup()
		case key == keySpecs:
			if end := l.evaluateSpecs(); end != nil {
				return end
			}
		default:
			l.evaluateFeature(strings.TrimPrefix(key, keyFeaturePrefix))
		}
	}
	return l.sync()
}

func (l *loop) evaluateFeature(id string) {
	prev, observed := l.picture.features[id]
	if !observed || !prev.exists {
		return // not observed, or its appearance is the listing's
	}
	cur := l.obs.feature(id, true)
	l.picture.features[id] = cur
	for _, e := range featureEvents(id, prev, cur) {
		l.push(e.Type, e.Data)
	}
}

// evaluateSpecs reports the features that appeared or disappeared
// (feature_added, feature_removed).
func (l *loop) evaluateSpecs() *stopped {
	ids, existing, err := l.obs.featureIDs()
	if err != nil {
		return &stopped{l.stop(Stopped{Reason: "watch_unavailable", Detail: "the project directory cannot be read: " + err.Error()}, 1)}
	}
	for _, id := range ids {
		prev, observed := l.picture.features[id]
		if existing[id] && (!observed || !prev.exists) {
			cur := l.obs.feature(id, true)
			l.picture.features[id] = cur
			delete(l.pending, keyFeaturePrefix+id)
			l.push(FeatureAdded, cur.data(id))
		}
	}
	for id, prev := range l.picture.features {
		if prev.exists && !existing[id] {
			if slices.Contains(l.opts.Features, id) {
				l.picture.features[id] = l.obs.feature(id, false)
			} else {
				delete(l.picture.features, id)
			}
			delete(l.pending, keyFeaturePrefix+id)
			l.push(FeatureRemoved, FeatureRef{Feature: id})
		}
	}
	return nil
}

func (l *loop) evaluateSetup() {
	cur := l.obs.source.Setup()
	if !bytes.Equal(cur, l.picture.setup) {
		l.picture.setup = cur
		l.push(AgentsConfigChanged, SetupChange{Setup: cur})
	}
}

// sync observes the watch set of the features now known.
func (l *loop) sync() *stopped {
	var ids []string
	for id, v := range l.picture.features {
		if v.exists {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	if err := l.watcher.Sync(l.obs.watchSet(ids)); err != nil {
		return &stopped{l.stop(unavailable(err), 1)}
	}
	return nil
}

// resync reports that events were lost and replaces the client's state
// with a new snapshot (F015-FR-005).
func (l *loop) resync(reason string) *stopped {
	l.push(ResyncRequired, Resync{Reason: reason})
	p, err := l.obs.build(l.ctx)
	if err != nil {
		return &stopped{l.stop(unreadable(err), 1)}
	}
	l.picture = p
	l.pending = map[string]time.Time{}
	l.push(Snapshot, p.snapshot())
	return l.sync()
}

// start registers the watches before the project is read, so no change is
// lost between reading and watching (research R-02): first the top-level
// directories, then, once the features are known, their directories and
// files, and only then their contents.
func (l *loop) start() (picture, error) {
	identity, err := l.cfg.Identity()
	if err != nil {
		return picture{}, errProjectUnreadable{err}
	}
	l.identity = identity
	if err := l.watcher.Sync(l.obs.watchSet(nil)); err != nil {
		return picture{}, err
	}
	ids, _, err := l.obs.featureIDs()
	if err != nil {
		return picture{}, errProjectUnreadable{err}
	}
	if err := l.watcher.Sync(l.obs.watchSet(ids)); err != nil {
		return picture{}, err
	}
	p, err := l.obs.build(l.ctx)
	if err != nil {
		return picture{}, errProjectUnreadable{err}
	}
	l.push(Snapshot, p.snapshot())
	return p, nil
}

func (l *loop) push(t Type, data any) bool {
	return l.q.Push(Event{Type: t, Time: time.Now(), Project: l.cfg.Project, Data: data})
}

// stop writes watch_stopped as the last event and waits at most
// stopFlushTimeout for it (owner decision 4).
func (l *loop) stop(s Stopped, code int) Result {
	l.q.Final(Event{Type: WatchStopped, Time: time.Now(), Project: l.cfg.Project, Data: s})
	l.q.Flush(stopFlushTimeout)
	if l.q.Failed() {
		return Result{ExitCode: 2}
	}
	return Result{ExitCode: code}
}

// finish waits for everything queued to be written.
func (l *loop) finish(code int) Result {
	for !l.q.Flush(time.Hour) {
	}
	if l.q.Failed() {
		return Result{ExitCode: 2}
	}
	return Result{ExitCode: code}
}

type errProjectUnreadable struct{ err error }

func (e errProjectUnreadable) Error() string { return e.err.Error() }

func unreadable(err error) Stopped {
	return Stopped{Reason: "watch_unavailable", Detail: "the project directory cannot be read: " + err.Error()}
}

// unavailable turns an error into watch_stopped watch_unavailable, with the
// adapter's detail and remedy when it gives them (F015-FR-016).
func unavailable(err error) Stopped {
	var u fswatch.Unavailable
	if errors.As(err, &u) {
		return Stopped{Reason: "watch_unavailable", Detail: u.Detail, Remedy: u.Remedy}
	}
	var p errProjectUnreadable
	if errors.As(err, &p) {
		return unreadable(p.err)
	}
	return Stopped{Reason: "watch_unavailable", Detail: err.Error()}
}
