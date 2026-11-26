package watch

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// QueueLimit is the most events kept while the client is not reading
// (F015-FR-017). It is fixed: no option, variable or build tag changes it.
const QueueLimit = 1000

// queue holds the events not yet written and the one goroutine that writes
// them, one Write per line (research R-06). seq is assigned at emission, so
// emitted lines are always consecutive (owner decision 2). On overflow the
// queue keeps only resync_required and refuses events until that line is
// written, which means the client reads again; Resumed then tells the loop
// to queue a new snapshot.
type queue struct {
	w      io.Writer
	encode func(Event, int64) []byte
	limit  int

	mu      sync.Mutex
	items   []Event
	writing bool
	paused  bool
	failed  bool
	stopped bool

	wake    chan struct{}
	idle    chan struct{}
	resumed chan struct{}
	errc    chan error
}

func newQueue(w io.Writer, encode func(Event, int64) []byte, limit int) *queue {
	q := &queue{w: w, encode: encode, limit: limit,
		wake: make(chan struct{}, 1), idle: make(chan struct{}, 1),
		resumed: make(chan struct{}, 1), errc: make(chan error, 1)}
	go q.run()
	return q
}

// Push queues e. It reports false when e was not queued: the queue is paused
// after an overflow, output failed, or the queue just overflowed.
func (q *queue) Push(e Event) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.failed || q.paused || q.stopped {
		return false
	}
	if len(q.items) >= q.limit {
		q.items = []Event{{Type: ResyncRequired, Time: e.Time, Project: e.Project, Data: Resync{Reason: "queue_overflow"}}}
		q.paused = true
		q.signal(q.wake)
		return false
	}
	q.items = append(q.items, e)
	q.signal(q.wake)
	return true
}

// safeEncode encodes e, turning a panic of the encoder into an error.
func (q *queue) safeEncode(e Event, seq int64) (line []byte, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("event %s cannot be encoded: %v", e.Type, recovered)
		}
	}()
	return q.encode(e, seq), nil
}

// Final queues the last event even while paused or full, so watch_stopped
// is always attempted.
func (q *queue) Final(e Event) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.failed {
		return
	}
	q.items = append(q.items, e)
	q.signal(q.wake)
}

// Failed reports whether a write failed.
func (q *queue) Failed() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.failed
}

// Resumed is signalled after the resync_required of an overflow is written.
func (q *queue) Resumed() <-chan struct{} { return q.resumed }

// Err delivers the first write error; nothing is written after it.
func (q *queue) Err() <-chan error { return q.errc }

// Flush waits until every queued event is written, at most timeout.
func (q *queue) Flush(timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		q.mu.Lock()
		done := len(q.items) == 0 && !q.writing || q.failed
		q.mu.Unlock()
		if done {
			return true
		}
		select {
		case <-q.idle:
		case <-timer.C:
			return false
		}
	}
}

// Stop ends the writer once it is idle; an event being written is finished.
func (q *queue) Stop() {
	q.mu.Lock()
	q.stopped = true
	q.mu.Unlock()
	q.signal(q.wake)
}

func (q *queue) signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (q *queue) run() {
	var seq int64
	for {
		q.mu.Lock()
		for len(q.items) == 0 {
			stopped := q.stopped
			q.mu.Unlock()
			q.signal(q.idle)
			if stopped {
				return
			}
			<-q.wake
			q.mu.Lock()
		}
		e := q.items[0]
		q.items = q.items[1:]
		q.writing = true
		q.mu.Unlock()

		seq++
		line, failure := q.safeEncode(e, seq)
		var err error
		if failure != nil {
			// The encoder failed: end with watch_stopped internal_error when
			// that line can still be encoded (review F10).
			stop := Event{Type: WatchStopped, Time: e.Time, Project: e.Project, Data: Stopped{Reason: "internal_error", Detail: failure.Error()}}
			if last, failed := q.safeEncode(stop, seq); failed == nil {
				_, _ = q.w.Write(last)
			}
			err = failure
		} else {
			_, err = q.w.Write(line)
		}

		q.mu.Lock()
		q.writing = false
		if err != nil {
			q.failed = true
			q.items = nil
			q.mu.Unlock()
			q.errc <- err
			q.signal(q.idle)
			return
		}
		resumed := e.Type == ResyncRequired && q.paused
		if resumed {
			q.paused = false
		}
		q.mu.Unlock()
		if resumed {
			q.signal(q.resumed)
		}
	}
}
