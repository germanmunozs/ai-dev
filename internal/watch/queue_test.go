package watch

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// gateWriter records each Write; while blocked, Write waits until release.
type gateWriter struct {
	mu      sync.Mutex
	lines   []string
	blocked bool
	release chan struct{}
	fail    error
}

func newGateWriter() *gateWriter { return &gateWriter{release: make(chan struct{})} }

func (w *gateWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	blocked, release, fail := w.blocked, w.release, w.fail
	w.mu.Unlock()
	if blocked {
		<-release
	}
	if fail != nil {
		return 0, fail
	}
	w.mu.Lock()
	w.lines = append(w.lines, string(p))
	w.mu.Unlock()
	return len(p), nil
}

func (w *gateWriter) block() {
	w.mu.Lock()
	w.blocked = true
	w.release = make(chan struct{})
	w.mu.Unlock()
}

func (w *gateWriter) unblock() {
	w.mu.Lock()
	w.blocked = false
	close(w.release)
	w.mu.Unlock()
}

func (w *gateWriter) written() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.lines...)
}

func testEncode(e Event, seq int64) []byte { return []byte(fmt.Sprintf("%d %s\n", seq, e.Type)) }

func heartbeat() Event { return Event{Type: Heartbeat, Data: struct{}{}} }

func TestQueueWritesInOrderWithSeq(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := newGateWriter()
		q := newQueue(w, testEncode, 10)
		defer q.Stop()
		q.Push(Event{Type: Snapshot})
		q.Push(heartbeat())
		q.Push(heartbeat())
		synctest.Wait()
		if got, want := strings.Join(w.written(), ""), "1 snapshot\n2 heartbeat\n3 heartbeat\n"; got != want {
			t.Fatalf("written %q, want %q", got, want)
		}
	})
}

func TestQueueOverflowKeepsOnlyResync(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := newGateWriter()
		q := newQueue(w, testEncode, 3)
		defer q.Stop()
		q.Push(Event{Type: Snapshot}) // written at once
		synctest.Wait()
		w.block()
		q.Push(heartbeat()) // taken by the writer, which blocks
		synctest.Wait()
		for i := 0; i < 4; i++ {
			q.Push(heartbeat())
		}
		if q.Push(heartbeat()) {
			t.Fatal("Push accepted an event while paused")
		}
		select {
		case <-q.Resumed():
			t.Fatal("resumed before resync_required was written")
		default:
		}
		w.unblock()
		synctest.Wait()
		select {
		case <-q.Resumed():
		default:
			t.Fatal("not resumed after resync_required was written")
		}
		if !q.Push(Event{Type: Snapshot}) {
			t.Fatal("Push refused after resuming")
		}
		synctest.Wait()
		got := strings.Join(w.written(), "")
		if want := "1 snapshot\n2 heartbeat\n3 resync_required\n4 snapshot\n"; got != want {
			t.Fatalf("written %q, want %q", got, want)
		}
	})
}

func TestQueueWriteErrorStopsOutput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := newGateWriter()
		w.fail = errors.New("broken pipe")
		q := newQueue(w, testEncode, 10)
		defer q.Stop()
		q.Push(heartbeat())
		synctest.Wait()
		select {
		case err := <-q.Err():
			if err == nil || !strings.Contains(err.Error(), "broken pipe") {
				t.Fatalf("Err = %v", err)
			}
		default:
			t.Fatal("no error reported")
		}
		w.fail = nil
		if q.Push(heartbeat()) {
			t.Fatal("Push accepted after a write error")
		}
		synctest.Wait()
		if len(w.written()) != 0 {
			t.Fatalf("written after a write error: %v", w.written())
		}
	})
}

func TestQueueFlush(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := newGateWriter()
		q := newQueue(w, testEncode, 10)
		q.Push(heartbeat())
		if !q.Flush(2 * time.Second) {
			t.Fatal("Flush failed with a reading client")
		}
		w.block()
		q.Push(Event{Type: WatchStopped})
		start := time.Now()
		if q.Flush(2 * time.Second) {
			t.Fatal("Flush succeeded with a blocked writer")
		}
		if waited := time.Since(start); waited != 2*time.Second {
			t.Fatalf("Flush waited %v, want 2s", waited)
		}
		q.Stop()
		w.unblock()
	})
}
