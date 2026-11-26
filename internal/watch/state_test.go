package watch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
	"testing/synctest"
	"time"

	"github.com/germanmunozs/ai-dev/internal/fswatch"
)

// gateEntry is one recorded Human Gate entry, as the store writes it.
func gateEntry(decision, from, to, at string) string {
	return fmt.Sprintf(`{"feature_id":"f","decision":%q,"approver":"ana@example.com","from_state":%q,"to_state":%q,"decided_at":%q}`, decision, from, to, at)
}

// setRecord writes specs/f/workflow-state.json and the source's state; a
// nil history writes a record without one, and history "!" an invalid one.
func (h *harness) setRecord(state string, history ...string) {
	record := fmt.Sprintf(`{"state":%q,"human_gates":[%s]}`, state, strings.Join(history, ","))
	if len(history) == 1 && history[0] == "!" {
		record = fmt.Sprintf(`{"state":%q,"human_gates":{}}`, state)
	}
	h.fs.set("specs/f/workflow-state.json", &fstest.MapFile{Data: []byte(record)})
	h.source.update(func(s *fakeSource) { s.states["f"] = state })
}

func (h *harness) notify(path string) {
	h.watcher.events <- fswatch.Change{Path: path, Kind: fswatch.Changed}
}

// running starts the loop and returns a stop function giving its result.
func (h *harness) running(t *testing.T, options Options) func() Result {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan Result, 1)
	go func() { done <- Run(ctx, options, h.config) }()
	synctest.Wait()
	return func() Result {
		cancel()
		return <-done
	}
}

type stateEvent struct {
	Feature   string     `json:"feature"`
	From      *string    `json:"from"`
	To        *string    `json:"to"`
	Outcome   string     `json:"outcome"`
	HumanGate *HumanGate `json:"human_gate"`
}

func (h *harness) stateChanges(t *testing.T) []stateEvent {
	t.Helper()
	var out []stateEvent
	for _, e := range h.out.events(t) {
		if e.Type == WorkflowStateChanged {
			var s stateEvent
			if err := json.Unmarshal(e.Data, &s); err != nil {
				t.Fatal(err)
			}
			out = append(out, s)
		}
	}
	return out
}

func str(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

const (
	toImplemented = `{"feature_id":"f","decision":"approval","approver":"ana@example.com","from_state":"tasked","to_state":"implemented","decided_at":"2026-11-25T10:00:00Z"}`
	toApproved    = `{"feature_id":"f","decision":"approval","approver":"ana@example.com","from_state":"implemented","to_state":"approved","decided_at":"2026-11-25T14:32:07Z"}`
)

func TestAppliedGateCarriesEvidence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("f", "implemented", "spec.md", "plan.md", "tasks.md")
		h.setRecord("implemented", toImplemented)
		stop := h.running(t, defaultOptions())
		h.setRecord("approved", toImplemented, toApproved)
		h.notify("/p/specs/f")
		time.Sleep(249 * time.Millisecond)
		synctest.Wait()
		if got := h.stateChanges(t); len(got) != 0 {
			t.Fatalf("emitted before the debounce ended: %+v", got)
		}
		time.Sleep(time.Millisecond)
		synctest.Wait()
		got := h.stateChanges(t)
		if len(got) != 1 {
			t.Fatalf("state changes %+v, want exactly one", got)
		}
		s := got[0]
		if s.Feature != "f" || str(s.From) != "implemented" || str(s.To) != "approved" || s.Outcome != "state_reported" {
			t.Errorf("change %+v", s)
		}
		if s.HumanGate == nil || s.HumanGate.Decision != "approval" || s.HumanGate.ToState != "approved" ||
			s.HumanGate.DecidedAt != "2026-11-25T14:32:07Z" || s.HumanGate.Approver != "ana@example.com" {
			t.Errorf("evidence %+v", s.HumanGate)
		}
		stop()
	})
}

func TestRefusedRequestEmitsNothing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("f", "approved", "spec.md")
		h.setRecord("approved", toApproved)
		stop := h.running(t, defaultOptions())
		h.notify("/p/specs/f") // a refused request touches nothing that matters
		time.Sleep(time.Second)
		synctest.Wait()
		if types := h.out.types(t); len(types) != 1 {
			t.Fatalf("events %v, want the snapshot only", types)
		}
		stop()
	})
}

func TestChangeWithoutNewEntryHasNoEvidence(t *testing.T) {
	cases := map[string]func(h *harness){
		"manual edit":            func(h *harness) { h.setRecord("planned", toImplemented) },
		"unreadable history":     func(h *harness) { h.setRecord("planned", "!") },
		"entry to another state": func(h *harness) { h.setRecord("planned", toImplemented, toApproved) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newHarness()
				h.addFeature("f", "implemented", "spec.md")
				h.setRecord("implemented", toImplemented)
				stop := h.running(t, defaultOptions())
				change(h)
				h.notify("/p/specs/f")
				time.Sleep(time.Second)
				synctest.Wait()
				got := h.stateChanges(t)
				if len(got) != 1 || str(got[0].To) != "planned" || got[0].HumanGate != nil {
					t.Fatalf("changes %+v, want one to planned with no evidence", got)
				}
				stop()
			})
		})
	}
}

// A history that becomes readable again never presents an old entry as
// the cause (analysis U5).
func TestHistoryReadableAgainHasNoEvidence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("f", "implemented", "spec.md")
		h.setRecord("implemented", "!")
		stop := h.running(t, defaultOptions())
		h.setRecord("approved", toImplemented, toApproved)
		h.notify("/p/specs/f")
		time.Sleep(time.Second)
		synctest.Wait()
		abandon := gateEntry("abandonment", "approved", "abandoned", "2026-11-26T08:00:00Z")
		h.setRecord("abandoned", toImplemented, toApproved, abandon)
		h.notify("/p/specs/f")
		time.Sleep(time.Second)
		synctest.Wait()
		got := h.stateChanges(t)
		if len(got) != 2 || got[0].HumanGate != nil || got[1].HumanGate == nil || got[1].HumanGate.Decision != "abandonment" {
			t.Fatalf("changes %+v, want none then the abandonment", got)
		}
		stop()
	})
}

func TestTwoEntriesInOneWindow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("f", "tasked", "spec.md", "plan.md", "tasks.md")
		h.setRecord("tasked")
		stop := h.running(t, defaultOptions())
		h.setRecord("implemented", toImplemented)
		h.notify("/p/specs/f")
		time.Sleep(100 * time.Millisecond)
		h.setRecord("approved", toImplemented, toApproved)
		h.notify("/p/specs/f")
		time.Sleep(time.Second)
		synctest.Wait()
		got := h.stateChanges(t)
		if len(got) != 1 || str(got[0].From) != "tasked" || str(got[0].To) != "approved" || got[0].HumanGate == nil || got[0].HumanGate.ToState != "approved" {
			t.Fatalf("changes %+v, want one tasked to approved with the last entry", got)
		}
		stop()
	})
}

func TestRecordCreatedFromDraft(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("f", "draft")
		stop := h.running(t, defaultOptions())
		h.setRecord("abandoned", gateEntry("abandonment", "draft", "abandoned", "2026-11-26T08:00:00Z"))
		h.notify("/p/specs/f")
		time.Sleep(time.Second)
		synctest.Wait()
		got := h.stateChanges(t)
		if len(got) != 1 || str(got[0].From) != "draft" || got[0].HumanGate == nil {
			t.Fatalf("changes %+v, want one from draft with evidence", got)
		}
		stop()
	})
}

func TestChangeToInvalidState(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("f", "planned", "spec.md", "plan.md")
		stop := h.running(t, defaultOptions())
		h.source.update(func(s *fakeSource) { s.states["f"] = "" })
		h.notify("/p/specs/f")
		time.Sleep(time.Second)
		synctest.Wait()
		got := h.stateChanges(t)
		if len(got) != 1 || str(got[0].From) != "planned" || got[0].To != nil || got[0].Outcome != "state_invalid" {
			t.Fatalf("changes %+v, want planned to null, state_invalid", got)
		}
		stop()
	})
}

// The window starts at the first notification and does not restart
// (owner decision 5).
func TestFixedDebounceWindow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("f", "planned", "spec.md", "plan.md")
		stop := h.running(t, defaultOptions())
		reads := func() int {
			n := 0
			for _, entry := range h.log.all() {
				if entry == "source:state:f" {
					n++
				}
			}
			return n
		}
		before := reads()
		h.notify("/p/specs/f")
		time.Sleep(100 * time.Millisecond)
		h.notify("/p/specs/f")
		time.Sleep(100 * time.Millisecond)
		h.notify("/p/specs/f")
		time.Sleep(49 * time.Millisecond)
		synctest.Wait()
		if reads() != before {
			t.Fatal("evaluated before the window closed")
		}
		time.Sleep(time.Millisecond)
		synctest.Wait()
		if got := reads() - before; got != 1 {
			t.Fatalf("evaluated %d times at 250ms, want once", got)
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if got := reads() - before; got != 1 {
			t.Fatalf("evaluated %d times, want once", got)
		}
		stop()
	})
}

// A notification queued while the snapshot was computed yields an event
// only for a change the snapshot does not include (R-02).
func TestNotificationDuringSnapshot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("f", "planned", "spec.md", "plan.md")
		h.notify("/p/specs/f") // already reflected in the snapshot
		stop := h.running(t, defaultOptions())
		time.Sleep(time.Second)
		synctest.Wait()
		if got := h.stateChanges(t); len(got) != 0 {
			t.Fatalf("changes %+v, want none", got)
		}
		stop()

		h = newHarness()
		h.addFeature("f", "planned", "spec.md", "plan.md")
		h.notify("/p/specs/f")
		stop = h.running(t, defaultOptions())
		h.setRecord("tasked") // after the snapshot, before the window closes
		time.Sleep(time.Second)
		synctest.Wait()
		if got := h.stateChanges(t); len(got) != 1 || str(got[0].To) != "tasked" {
			t.Fatalf("changes %+v, want one to tasked", got)
		}
		stop()
	})
}

func TestStateArtifactsNextOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("f", "specified", "spec.md")
		stop := h.running(t, defaultOptions())
		h.fs.set("specs/f/plan.md", &fstest.MapFile{Data: []byte("# plan\n")})
		h.setRecord("planned")
		h.source.update(func(s *fakeSource) { s.next["f"] = "workflow_terminal" })
		h.notify("/p/specs/f")
		time.Sleep(time.Second)
		synctest.Wait()
		types := h.out.types(t)
		want := []Type{Snapshot, WorkflowStateChanged, ArtifactsChanged, NextStepChanged}
		if fmt.Sprint(types) != fmt.Sprint(want) {
			t.Fatalf("events %v, want %v", types, want)
		}
		stop()
	})
}
