package watch

import (
	"context"
	"encoding/json"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"testing/synctest"
	"time"

	"github.com/germanmunozs/ai-dev/internal/fswatch"
)

func snapshotData(t *testing.T, e decoded) SnapshotData {
	t.Helper()
	if e.Type != Snapshot {
		t.Fatalf("event %s, want snapshot", e.Type)
	}
	var d SnapshotData
	if err := json.Unmarshal(e.Data, &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestSnapshotOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("b-feature", "planned", "spec.md", "plan.md")
		h.addFeature("a-feature", "", "spec.md")
		h.fs.set("specs/b-feature/tasks.md", &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("../a-feature/spec.md")})
		h.fs.set("specs/a-feature/plan.md", &fstest.MapFile{Mode: fs.ModeDir | 0o755})
		h.fs.set("specs/b-feature/spec.md", &fstest.MapFile{Data: []byte("12345"), ModTime: time.Date(2026, 11, 26, 9, 0, 0, 5, time.UTC)})
		constructed := false
		h.config.NewWatcher = func() (fswatch.Watcher, error) { constructed = true; return h.watcher, nil }
		options := defaultOptions()
		options.Once = true

		result := Run(context.Background(), options, h.config)
		if result.ExitCode != 0 || constructed {
			t.Fatalf("Once: exit %d, watcher constructed %v", result.ExitCode, constructed)
		}
		events := h.out.events(t)
		if len(events) != 1 || events[0].Seq != 1 {
			t.Fatalf("Once emitted %+v, want one snapshot with seq 1", events)
		}
		d := snapshotData(t, events[0])
		if len(d.Features) != 2 || d.Features[0].Feature != "a-feature" || d.Features[1].Feature != "b-feature" {
			t.Fatalf("features %+v, want a-feature then b-feature", d.Features)
		}
		if got, want := string(d.Features[1].State), `{"feature_id":"b-feature","state":"planned","outcome":"state_reported"}`; got != want {
			t.Errorf("state %s, want the source's bytes %s", got, want)
		}
		if !strings.Contains(string(d.Features[0].State), `"state_invalid"`) || !strings.Contains(string(d.Features[1].Next), `"step_recommended"`) {
			t.Errorf("state or next not the source's: %+v", d.Features)
		}
		b := d.Features[1].Artifacts
		if len(b) != 3 || b[0].Name != "spec.md" || b[1].Name != "plan.md" || b[2].Name != "tasks.md" {
			t.Fatalf("artifacts %+v, want spec, plan, tasks", b)
		}
		if b[0].Status != "present" || b[0].Size == nil || *b[0].Size != 5 || b[0].Modified != "2026-11-26T09:00:00.000000005Z" {
			t.Errorf("spec.md %+v", b[0])
		}
		if b[1].Status != "present" || b[2].Status != "not_regular_file" || b[2].Size != nil {
			t.Errorf("plan.md %+v, tasks.md (symbolic link) %+v", b[1], b[2])
		}
		a := d.Features[0].Artifacts
		if a[1].Status != "not_regular_file" || a[2].Status != "absent" || a[2].Size != nil || a[2].Modified != "" {
			t.Errorf("a-feature plan.md (directory) %+v, tasks.md %+v", a[1], a[2])
		}
		if !strings.Contains(string(d.Setup), `"configured"`) || !strings.Contains(string(d.Environment), `"overall":"ready"`) {
			t.Errorf("setup %s environment %s", d.Setup, d.Environment)
		}
	})
}

func TestSnapshotFeatureFilter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("a", "planned", "spec.md")
		h.addFeature("b", "tasked", "spec.md")
		options := defaultOptions()
		options.Once = true
		options.Features = []string{"zz", "b"}
		Run(context.Background(), options, h.config)
		d := snapshotData(t, h.out.events(t)[0])
		if len(d.Features) != 2 || d.Features[0].Feature != "b" || d.Features[1].Feature != "zz" {
			t.Fatalf("features %+v, want b and zz", d.Features)
		}
		zz := d.Features[1]
		if !strings.Contains(string(zz.State), "feature_unknown") || !strings.Contains(string(zz.Next), "feature_unknown") {
			t.Errorf("unknown feature %+v", zz)
		}
		for _, a := range zz.Artifacts {
			if a.Status != "absent" {
				t.Errorf("unknown feature artifact %+v", a)
			}
		}
	})
}

// Watches are registered before the project is first read, so no change is
// lost (R-02).
func TestSnapshotRegistersWatchesFirst(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("a", "planned", "spec.md")
		h.fs.set(".ai-dev", &fstest.MapFile{Mode: fs.ModeDir | 0o755})
		h.fs.set(".ai-dev/agents.json", &fstest.MapFile{Data: []byte("{}")})
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan Result)
		go func() { done <- Run(ctx, defaultOptions(), h.config) }()
		synctest.Wait()
		log := h.log.all()
		first := slices.Index(log, "watcher:sync")
		read := slices.Index(log, "source:features")
		if first < 0 || read < 0 || first > read {
			t.Fatalf("log %v: the watch set must be registered before the first read", log)
		}
		sync := h.watcher.lastSync()
		for _, want := range []string{"/p", "/p/.ai-dev", "/p/specs", "/p/specs/a", "/p/.ai-dev/agents.json", "/p/specs/a/spec.md"} {
			if !slices.Contains(sync, want) {
				t.Errorf("watch set %v lacks %s", sync, want)
			}
		}
		for _, absent := range []string{"/p/specs/a/plan.md", "/p/specs/a/workflow-state.json"} {
			if slices.Contains(sync, absent) {
				t.Errorf("watch set %v holds the missing file %s", sync, absent)
			}
		}
		if types := h.out.types(t); len(types) != 1 || types[0] != Snapshot {
			t.Fatalf("events %v, want the snapshot only", types)
		}
		cancel()
		if r := <-done; r.ExitCode != 0 {
			t.Fatalf("exit %d", r.ExitCode)
		}
	})
}

func TestOnceWorksWithoutWatching(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("a", "planned")
		h.config.NewWatcher = func() (fswatch.Watcher, error) {
			return nil, fswatch.Unavailable{Reason: "platform_unsupported", Detail: "x"}
		}
		options := defaultOptions()
		options.Once = true
		if r := Run(context.Background(), options, h.config); r.ExitCode != 0 {
			t.Fatalf("exit %d", r.ExitCode)
		}
		if types := h.out.types(t); len(types) != 1 || types[0] != Snapshot {
			t.Fatalf("events %v", types)
		}
	})
}

func TestOnceWithUnreadableProject(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.source.update(func(s *fakeSource) { s.err = fs.ErrPermission })
		options := defaultOptions()
		options.Once = true
		if r := Run(context.Background(), options, h.config); r.ExitCode != 1 {
			t.Fatalf("exit %d, want 1", r.ExitCode)
		}
		events := h.out.events(t)
		if len(events) != 1 || events[0].Type != WatchStopped || !strings.Contains(string(events[0].Data), `"reason":"watch_unavailable"`) {
			t.Fatalf("events %+v, want only watch_stopped watch_unavailable", events)
		}
	})
}
