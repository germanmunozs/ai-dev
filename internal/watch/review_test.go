package watch

import (
	"context"
	"io/fs"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"testing/synctest"
	"time"
)

// Tests for the findings of the independent review (tasks.md T046).

// F2: a change that lands between reading a newly found path and starting
// to observe it is not lost: every path Sync adds is evaluated once more.
func TestChangeBeforeNewPathIsObservedIsNotLost(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("a", "draft")
		stop := h.running(t, defaultOptions())
		var fired atomic.Bool
		h.watcher.onSync = func(added []string) {
			if slices.Contains(added, "/p/specs/b") && !fired.Swap(true) {
				// The write lands after the read of b, before its watch.
				h.setFeatureState("b", "specified")
			}
		}
		h.addFeature("b", "draft")
		h.notify("/p/specs")
		h.settle()
		h.settle()
		types := h.out.types(t)
		if !slices.Contains(types, FeatureAdded) || !slices.Contains(types, WorkflowStateChanged) {
			t.Fatalf("events %v, want feature_added then the state change made before b was observed", types)
		}
		stop()
	})
}

// F2: a feature directory created after the start's registration but before
// its first read is evaluated once it is observed.
func TestStartMarksPathsAddedAfterTheSnapshot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("a", "draft")
		syncs := 0
		h.watcher.onSync = func([]string) {
			syncs++
			if syncs == 2 { // between the second registration and the read
				h.addFeature("b", "draft")
			}
		}
		stop := h.running(t, defaultOptions())
		if sync := h.watcher.lastSync(); !slices.Contains(sync, "/p/specs/b") {
			t.Fatalf("watch set %v lacks b right after the snapshot", sync)
		}
		stop()
	})
}

// F3: evidence is attached only for this feature's own new entries, starting
// from the state observed before, with the old history kept.
func TestEvidenceMustBelongToThisFeatureAndChain(t *testing.T) {
	foreign := `{"feature_id":"other","decision":"approval","approver":"ana@example.com","from_state":"implemented","to_state":"approved","decided_at":"2026-11-25T14:32:07Z"}`
	fromTasked := `{"feature_id":"f","decision":"approval","approver":"ana@example.com","from_state":"tasked","to_state":"approved","decided_at":"2026-11-25T14:32:07Z"}`
	rewritten := `{"feature_id":"f","decision":"approval","approver":"eve","from_state":"draft","to_state":"implemented","decided_at":"2026-11-20T10:00:00Z"}`
	cases := map[string][]string{
		"foreign feature_id":    {toImplemented, foreign},
		"from_state mismatch":   {toImplemented, fromTasked},
		"rewritten history":     {rewritten, toApproved},
		"copied from elsewhere": {foreign},
	}
	for name, history := range cases {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newHarness()
				h.addFeature("f", "implemented", "spec.md", "plan.md", "tasks.md")
				h.setRecord("implemented", toImplemented)
				stop := h.running(t, defaultOptions())
				h.setRecord("approved", history...)
				h.notify("/p/specs/f")
				h.settle()
				got := h.stateChanges(t)
				if len(got) != 1 || got[0].HumanGate != nil {
					t.Fatalf("changes %+v, want one with no evidence", got)
				}
				stop()
			})
		})
	}
}

// F5: --once stops waiting for a client that does not read when a signal
// arrives, and exits 0 with no further event.
func TestOnceStopsOnSignalWithBlockedClient(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		w := newGateWriter()
		w.block()
		h.config.Output = w
		options := defaultOptions()
		options.Once = true
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan Result, 1)
		go func() { done <- Run(ctx, options, h.config) }()
		time.Sleep(time.Minute)
		synctest.Wait()
		select {
		case r := <-done:
			t.Fatalf("returned %+v before the signal", r)
		default:
		}
		cancel()
		if r := <-done; r.ExitCode != 0 {
			t.Fatalf("exit %d, want 0", r.ExitCode)
		}
		w.unblock()
	})
}

// F6: a --feature name that is not a feature never reads through it.
func TestUnknownNamedFeatureReadsNothing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.fs.set("specs/link", &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("../outside")})
		h.fs.set("outside/spec.md", &fstest.MapFile{Data: []byte("x")})
		options := defaultOptions()
		options.Once = true
		options.Features = []string{"link"}
		Run(context.Background(), options, h.config)
		d := snapshotData(t, h.out.events(t)[0])
		for _, a := range d.Features[0].Artifacts {
			if a.Status != "absent" {
				t.Fatalf("artifact %+v of a feature that does not exist", a)
			}
		}
	})
}

// F7: a Human Gate that lands between the state read and the history read
// is not lost: the observation is repeated until both agree.
func TestGateBetweenStateAndHistoryKeepsEvidence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("f", "implemented", "spec.md", "plan.md", "tasks.md")
		h.setRecord("implemented", toImplemented)
		stop := h.running(t, defaultOptions())
		var once atomic.Bool
		h.source.update(func(s *fakeSource) {
			s.onState = func(id string) {
				if id == "f" && !once.Swap(true) {
					h.setRecord("approved", toImplemented, toApproved)
				}
			}
		})
		h.notify("/p/specs/f")
		h.settle()
		got := h.stateChanges(t)
		if len(got) != 1 || str(got[0].To) != "approved" || got[0].HumanGate == nil {
			t.Fatalf("changes %+v, want one to approved with its evidence", got)
		}
		stop()
	})
}

// F10: a panic in the writer ends the run with exit 2 and, when it can
// still be written, watch_stopped internal_error as the last line.
func TestWriterPanicIsInternalError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("f", "planned")
		var calls atomic.Int32
		h.config.Encode = func(e Event, seq int64) []byte {
			if e.Type == Heartbeat && calls.Add(1) == 1 {
				panic("encoder failed")
			}
			return EncodeJSON(e, seq)
		}
		options := defaultOptions()
		options.Heartbeat = 5 * time.Second
		r := Run(context.Background(), options, h.config)
		events := h.out.events(t)
		last := events[len(events)-1]
		if s := stoppedOf(t, last); r.ExitCode != 2 || s.Reason != "internal_error" || !strings.Contains(s.Detail, "encoder failed") {
			t.Fatalf("exit %d, last %+v", r.ExitCode, s)
		}
	})
}

// F12: a change to an entry the loop does not observe, such as a file at the
// project's top level, triggers no evaluation; a new entry of specs/ does.
func TestUnobservedEntriesAreIgnored(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("a", "draft")
		stop := h.running(t, defaultOptions())
		before := len(h.log.all())
		h.notify("/p/build.log")
		h.settle()
		if after := len(h.log.all()); after != before {
			t.Fatalf("a top-level file caused %d reads: %v", after-before, h.log.all()[before:])
		}
		h.addFeature("b", "draft")
		h.notify("/p/specs/b") // Linux names the new entry, not specs itself
		h.settle()
		if !slices.Contains(h.out.types(t), FeatureAdded) {
			t.Fatalf("events %v, want feature_added for a new entry of specs", h.out.types(t))
		}
		stop()
	})
}
