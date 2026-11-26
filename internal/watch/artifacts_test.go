package watch

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"testing/synctest"
	"time"
)

// removeFeature deletes specs/<id> and its files.
func (h *harness) removeFeature(id string) {
	h.fs.mu.Lock()
	for name := range h.fs.m {
		if name == "specs/"+id || strings.HasPrefix(name, "specs/"+id+"/") {
			delete(h.fs.m, name)
		}
	}
	h.fs.mu.Unlock()
	h.source.update(func(s *fakeSource) {
		s.features = slices.DeleteFunc(s.features, func(f string) bool { return f == id })
		delete(s.states, id)
	})
}

func (h *harness) settle() {
	time.Sleep(time.Second)
	synctest.Wait()
}

func artifactsOf(t *testing.T, e decoded) ArtifactsChange {
	t.Helper()
	var a ArtifactsChange
	if err := json.Unmarshal(e.Data, &a); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestPlanCreatedChangesArtifactsThenNext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("f", "specified", "spec.md")
		stop := h.running(t, defaultOptions())
		h.fs.set("specs/f/plan.md", &fstest.MapFile{Data: []byte("# plan\n")})
		h.source.update(func(s *fakeSource) { s.next["f"] = "artifacts_inconsistent" })
		h.notify("/p/specs/f")
		h.settle()
		events := h.out.events(t)
		if got := fmt.Sprint(h.out.types(t)); got != fmt.Sprint([]Type{Snapshot, ArtifactsChanged, NextStepChanged}) {
			t.Fatalf("events %s", got)
		}
		a := artifactsOf(t, events[1])
		if a.Feature != "f" || len(a.Artifacts) != 3 || a.Artifacts[1].Name != "plan.md" || a.Artifacts[1].Status != "present" {
			t.Fatalf("artifacts %+v", a)
		}
		var n NextChange
		if err := json.Unmarshal(events[2].Data, &n); err != nil || !strings.Contains(string(n.Next), "artifacts_inconsistent") {
			t.Fatalf("next %s: %v", events[2].Data, err)
		}
		stop()
	})
}

func TestArtifactMetadataChangeOnly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("f", "specified", "spec.md")
		stop := h.running(t, defaultOptions())
		h.fs.set("specs/f/spec.md", &fstest.MapFile{Data: []byte("# spec, longer\n"), ModTime: time.Now()})
		h.notify("/p/specs/f/spec.md")
		h.settle()
		if got := fmt.Sprint(h.out.types(t)); got != fmt.Sprint([]Type{Snapshot, ArtifactsChanged}) {
			t.Fatalf("events %s, want artifacts_changed only", got)
		}
		stop()
	})
}

func TestFiveSavesAreOneChange(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("f", "specified", "spec.md")
		stop := h.running(t, defaultOptions())
		for i := range 5 {
			h.fs.set("specs/f/spec.md", &fstest.MapFile{Data: []byte(strings.Repeat("x", i+10)), ModTime: time.Now()})
			h.notify("/p/specs/f/spec.md")
			time.Sleep(40 * time.Millisecond)
		}
		h.settle()
		if got := fmt.Sprint(h.out.types(t)); got != fmt.Sprint([]Type{Snapshot, ArtifactsChanged}) {
			t.Fatalf("events %s, want one artifacts_changed", got)
		}
		stop()
	})
}

func TestFeatureAddedAndRemoved(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("a", "specified", "spec.md")
		stop := h.running(t, defaultOptions())
		h.addFeature("b", "planned", "spec.md", "plan.md")
		h.notify("/p/specs")
		h.notify("/p/specs/b") // its own events belong to feature_added
		h.settle()
		events := h.out.events(t)
		if got := fmt.Sprint(h.out.types(t)); got != fmt.Sprint([]Type{Snapshot, FeatureAdded}) {
			t.Fatalf("events %s, want feature_added only", got)
		}
		var added FeatureData
		if err := json.Unmarshal(events[1].Data, &added); err != nil {
			t.Fatal(err)
		}
		if added.Feature != "b" || !strings.Contains(string(added.State), `"planned"`) || len(added.Artifacts) != 3 || len(added.Next) == 0 {
			t.Fatalf("added %+v", added)
		}
		if !slices.Contains(h.watcher.lastSync(), "/p/specs/b") {
			t.Errorf("watch set %v lacks the new feature", h.watcher.lastSync())
		}

		h.removeFeature("a")
		h.notify("/p/specs")
		h.settle()
		events = h.out.events(t)
		last := events[len(events)-1]
		if last.Type != FeatureRemoved || !strings.Contains(string(last.Data), `"feature":"a"`) {
			t.Fatalf("last event %s %s, want feature_removed a", last.Type, last.Data)
		}
		if slices.Contains(h.watcher.lastSync(), "/p/specs/a") {
			t.Errorf("watch set %v still holds the removed feature", h.watcher.lastSync())
		}
		stop()
	})
}

func TestSpecsCreatedAfterStart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		stop := h.running(t, defaultOptions())
		if slices.Contains(h.watcher.lastSync(), "/p/specs") {
			t.Fatal("specs observed before it exists")
		}
		h.addFeature("a", "draft")
		h.notify("/p") // the project directory reports the new entry
		h.settle()
		if got := fmt.Sprint(h.out.types(t)); got != fmt.Sprint([]Type{Snapshot, FeatureAdded}) {
			t.Fatalf("events %s, want feature_added", got)
		}
		if sync := h.watcher.lastSync(); !slices.Contains(sync, "/p/specs") || !slices.Contains(sync, "/p/specs/a") {
			t.Fatalf("watch set %v lacks specs and its feature", sync)
		}
		stop()
	})
}

func TestFeatureFilterIgnoresOthers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("a", "specified", "spec.md")
		h.addFeature("b", "specified", "spec.md")
		options := defaultOptions()
		options.Features = []string{"a"}
		stop := h.running(t, options)
		h.setFeatureState("b", "planned")
		h.fs.set("specs/b/plan.md", &fstest.MapFile{Data: []byte("# plan\n")})
		h.notify("/p/specs/b")
		h.addFeature("c", "draft")
		h.notify("/p/specs")
		h.settle()
		if got := fmt.Sprint(h.out.types(t)); got != fmt.Sprint([]Type{Snapshot}) {
			t.Fatalf("events %s, want none for b or c", got)
		}
		stop()
	})
}

// No event carries an artifact's content (F015-FR-021).
func TestEventsCarryNoContent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		const marker = "SECRET-CONTENT-MARKER"
		h.addFeature("f", "specified")
		h.fs.set("specs/f/spec.md", &fstest.MapFile{Data: []byte(marker)})
		stop := h.running(t, defaultOptions())
		h.fs.set("specs/f/plan.md", &fstest.MapFile{Data: []byte(marker)})
		h.fs.set("specs/f/tasks.md", &fstest.MapFile{Data: []byte(marker), Mode: fs.ModePerm})
		h.notify("/p/specs/f")
		h.settle()
		stop()
		h.out.mu.Lock()
		out := h.out.buf.String()
		h.out.mu.Unlock()
		if strings.Contains(out, marker) {
			t.Fatalf("output carries artifact content:\n%s", out)
		}
		if !strings.Contains(out, `"artifacts_changed"`) {
			t.Fatalf("no artifacts_changed in:\n%s", out)
		}
	})
}

func (h *harness) setFeatureState(id, state string) {
	h.source.update(func(s *fakeSource) { s.states[id] = state })
}
