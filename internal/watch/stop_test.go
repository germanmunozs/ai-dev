package watch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/germanmunozs/ai-dev/internal/fswatch"
)

func stoppedOf(t *testing.T, e decoded) Stopped {
	t.Helper()
	if e.Type != WatchStopped {
		t.Fatalf("event %s %s, want watch_stopped", e.Type, e.Data)
	}
	var s Stopped
	if err := json.Unmarshal(e.Data, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func decodeLines(t *testing.T, lines []string) []decoded {
	t.Helper()
	var out []decoded
	for _, line := range lines {
		var d decoded
		if err := json.Unmarshal([]byte(line), &d); err != nil {
			t.Fatalf("invalid line %q", line)
		}
		out = append(out, d)
	}
	return out
}

func TestSignalStops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("f", "planned")
		stop := h.running(t, defaultOptions())
		if r := stop(); r.ExitCode != 0 {
			t.Fatalf("exit %d", r.ExitCode)
		}
		events := h.out.events(t)
		if s := stoppedOf(t, events[len(events)-1]); s.Reason != "signal" {
			t.Fatalf("reason %q", s.Reason)
		}
		if !h.watcher.closed {
			t.Error("watcher not closed")
		}
	})
}

// A client that does not read cannot keep watch alive: it waits at most 2 s
// to write watch_stopped (owner decision 4).
func TestSignalWithBlockedClient(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		w := newGateWriter()
		w.block()
		h.config.Output = w
		stop := h.running(t, defaultOptions())
		start := time.Now()
		if r := stop(); r.ExitCode != 0 {
			t.Fatalf("exit %d", r.ExitCode)
		}
		if waited := time.Since(start); waited != stopFlushTimeout {
			t.Fatalf("waited %v, want %v", waited, stopFlushTimeout)
		}
		w.unblock()
	})
}

func TestProjectGone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		ctx := context.Background()
		done := make(chan Result, 1)
		go func() { done <- Run(ctx, defaultOptions(), h.config) }()
		synctest.Wait()
		h.watcher.events <- fswatch.Change{Path: "/p", Kind: fswatch.Gone}
		r := <-done
		events := h.out.events(t)
		if s := stoppedOf(t, events[len(events)-1]); r.ExitCode != 1 || s.Reason != "project_gone" {
			t.Fatalf("exit %d, reason %q", r.ExitCode, s.Reason)
		}
	})
}

func TestProjectIdentityChanged(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("f", "planned")
		var moved atomic.Bool
		h.config.Identity = func() (string, error) {
			if moved.Load() {
				return "dev:2", nil
			}
			return "dev:1", nil
		}
		done := make(chan Result, 1)
		go func() { done <- Run(context.Background(), defaultOptions(), h.config) }()
		synctest.Wait()
		moved.Store(true)
		h.notify("/p/specs/f")
		r := <-done
		events := h.out.events(t)
		if s := stoppedOf(t, events[len(events)-1]); r.ExitCode != 1 || s.Reason != "project_gone" {
			t.Fatalf("exit %d, reason %q", r.ExitCode, s.Reason)
		}
	})
}

func TestWatcherUnavailableAtStart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.config.NewWatcher = func() (fswatch.Watcher, error) {
			return nil, fswatch.Unavailable{Reason: "watch_limit", Detail: "limit reached", Remedy: "raise it"}
		}
		r := Run(context.Background(), defaultOptions(), h.config)
		events := h.out.events(t)
		if r.ExitCode != 1 || len(events) != 1 {
			t.Fatalf("exit %d, events %v; want only watch_stopped and exit 1", r.ExitCode, h.out.types(t))
		}
		if s := stoppedOf(t, events[0]); s.Reason != "watch_unavailable" || s.Detail != "limit reached" || s.Remedy != "raise it" {
			t.Fatalf("stopped %+v", s)
		}
	})
}

func TestSyncUnavailableAfterStart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("f", "planned")
		done := make(chan Result, 1)
		go func() { done <- Run(context.Background(), defaultOptions(), h.config) }()
		synctest.Wait()
		h.watcher.mu.Lock()
		h.watcher.syncErr = fswatch.Unavailable{Reason: "watch_limit", Detail: "limit reached", Remedy: "raise it"}
		h.watcher.mu.Unlock()
		h.addFeature("g", "draft")
		h.notify("/p/specs")
		r := <-done
		events := h.out.events(t)
		if s := stoppedOf(t, events[len(events)-1]); r.ExitCode != 1 || s.Reason != "watch_unavailable" || s.Remedy != "raise it" {
			t.Fatalf("exit %d, stopped %+v", r.ExitCode, s)
		}
	})
}

func TestKernelOverflowResyncs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("f", "planned")
		stop := h.running(t, defaultOptions())
		h.setFeatureState("f", "tasked")
		h.watcher.events <- fswatch.Change{Kind: fswatch.Overflow}
		synctest.Wait()
		events := h.out.events(t)
		if got := fmt.Sprint(h.out.types(t)); got != fmt.Sprint([]Type{Snapshot, ResyncRequired, Snapshot}) {
			t.Fatalf("events %s", got)
		}
		if !strings.Contains(string(events[1].Data), `"watch_overflow"`) || !strings.Contains(string(events[2].Data), `"tasked"`) {
			t.Fatalf("resync %s, snapshot %s", events[1].Data, events[2].Data)
		}
		stop()
	})
}

// A full queue keeps only resync_required; the new snapshot is computed
// once the client reads again, so it shows changes made meanwhile
// (F015-FR-017, SC-005).
func TestQueueOverflowResyncsAfterResume(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("f", "planned")
		w := newGateWriter()
		h.config.Output = w
		h.config.queueLimit = 3
		options := defaultOptions()
		options.Heartbeat = 5 * time.Second
		options.EnvironmentInterval = 0
		stop := h.running(t, options)
		w.block()
		time.Sleep(5 * time.Second) // one heartbeat, taken by the blocked writer
		synctest.Wait()
		time.Sleep(20 * time.Second) // four more: three fit, the fourth overflows
		synctest.Wait()
		h.setFeatureState("f", "tasked")
		w.unblock()
		synctest.Wait()
		events := decodeLines(t, w.written())
		var types []Type
		for i, e := range events {
			types = append(types, e.Type)
			if e.Seq != int64(i+1) {
				t.Fatalf("seq %d at line %d", e.Seq, i+1)
			}
		}
		if got := fmt.Sprint(types[:4]); got != fmt.Sprint([]Type{Snapshot, Heartbeat, ResyncRequired, Snapshot}) {
			t.Fatalf("events %v", types)
		}
		if !strings.Contains(string(events[2].Data), `"queue_overflow"`) || !strings.Contains(string(events[3].Data), `"tasked"`) {
			t.Fatalf("resync %s, snapshot %s", events[2].Data, events[3].Data)
		}
		stop()
	})
}

func TestSourcePanicIsInternalError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.addFeature("f", "planned")
		done := make(chan Result, 1)
		go func() { done <- Run(context.Background(), defaultOptions(), h.config) }()
		synctest.Wait()
		h.source.update(func(s *fakeSource) { s.panicOn = "features" })
		h.notify("/p/specs")
		r := <-done
		events := h.out.events(t)
		if s := stoppedOf(t, events[len(events)-1]); r.ExitCode != 2 || s.Reason != "internal_error" || !strings.Contains(s.Detail, "source failed") {
			t.Fatalf("exit %d, stopped %+v", r.ExitCode, s)
		}
		if !h.watcher.closed {
			t.Error("watcher not closed")
		}
	})
}

func TestClosedOutputExits2(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		w := newGateWriter()
		w.fail = errors.New("broken pipe")
		h.config.Output = w
		r := Run(context.Background(), defaultOptions(), h.config)
		if r.ExitCode != 2 || len(w.written()) != 0 {
			t.Fatalf("exit %d, written %v", r.ExitCode, w.written())
		}
		if !h.watcher.closed {
			t.Error("watcher not closed")
		}
	})
}
