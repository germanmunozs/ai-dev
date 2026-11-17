//go:build (darwin && !ios) || (linux && !android)

package gitstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ai-dev/internal/gitmanager"
)

// These tests cover the append-only event layer every audit record goes
// through: naming, ordering, no-replace publication, durability faults and
// rejection of an inconsistent event directory. Record content, effect
// claims and reconciliation records arrive with the audit writer (T030),
// whose own tests are written first in that task.

func TestEventNamesOrderBySequence(t *testing.T) {
	name := eventName(7, gitmanager.AuditEventOperationStarted)
	if name != "00000000000000000007-operation_started.json" {
		t.Fatalf("event name %q", name)
	}
	sequence, kind, err := parseEventName(name)
	if err != nil || sequence != 7 || kind != gitmanager.AuditEventOperationStarted {
		t.Fatalf("parsed %d %q %v", sequence, kind, err)
	}
	// Zero padding keeps lexical and numeric order identical.
	if eventName(9, gitmanager.AuditEventMutationIntent) >= eventName(10, gitmanager.AuditEventMutationIntent) {
		t.Fatal("event names do not sort numerically")
	}
	for _, bad := range []string{
		"7-operation_started.json", "00000000000000000000-attempt_created.json", "00000000000000000001-unknown.json",
		"00000000000000000001-attempt_created.txt", "0000000000000000000x-attempt_created.json", "attempt_created.json",
	} {
		if _, _, err := parseEventName(bad); err == nil {
			t.Errorf("malformed event name %q accepted", bad)
		}
	}
}

func TestEventsArePublishedOnceAndListedInOrder(t *testing.T) {
	r, dir, _ := newTestRecords(t)
	kinds := []gitmanager.AuditEventKind{gitmanager.AuditEventAttemptCreated, gitmanager.AuditEventMutationIntent, gitmanager.AuditEventOperationStarted}
	// Published out of order, listed in sequence order.
	for _, i := range []int{2, 0, 1} {
		if _, err := r.publishEvent("records", uint64(i+1), kinds[i], []byte(string(kinds[i]))); err != nil {
			t.Fatal(err)
		}
	}
	events, temporaries, err := r.listEvents("records")
	if err != nil || len(temporaries) != 0 {
		t.Fatalf("list: %v temporaries %v", err, temporaries)
	}
	for i, event := range events {
		if event.sequence != uint64(i+1) || event.kind != kinds[i] {
			t.Fatalf("event %d is %+v", i, event)
		}
	}
	// An event is immutable: the same sequence and kind cannot be written
	// twice, and the original bytes survive.
	if _, err := r.publishEvent("records", 2, kinds[1], []byte("rewritten")); !errors.Is(err, ErrRecordExists) {
		t.Fatalf("republish: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, eventName(2, kinds[1]))); string(data) != string(kinds[1]) {
		t.Fatalf("event rewritten to %q", data)
	}
	if _, err := r.publishEvent("records", 0, kinds[0], []byte("x")); err == nil {
		t.Fatal("sequence zero accepted")
	}
	if _, err := r.publishEvent("records", 4, "bogus", []byte("x")); err == nil {
		t.Fatal("unknown event kind accepted")
	}
	if _, err := r.publishEvent("records", 4, kinds[0], nil); err == nil {
		t.Fatal("empty event accepted")
	}
}

// A failed durable write publishes nothing; an unverifiable directory sync
// is reported as such rather than as success.
func TestEventDurabilityFaults(t *testing.T) {
	for _, step := range []string{"write", "sync-file", "link"} {
		r, dir, _ := newTestRecords(t)
		r.ops = failAt(r.ops, step)
		if _, err := r.publishEvent("records", 1, gitmanager.AuditEventAttemptCreated, []byte("x")); !errors.Is(err, errInjectedFault) {
			t.Fatalf("%s: %v", step, err)
		}
		if names := entries(t, dir); len(names) != 0 {
			t.Fatalf("%s left %v", step, names)
		}
	}
	r, _, _ := newTestRecords(t)
	r.ops = failAt(r.ops, "sync-directory")
	if _, err := r.publishEvent("records", 1, gitmanager.AuditEventAttemptCreated, []byte("x")); !errors.Is(err, gitmanager.ErrStoreOutcomeUnverifiable) {
		t.Fatalf("directory sync failure: %v", err)
	}
}

// An event directory that a single writer could not have produced is
// rejected as a whole rather than partially trusted.
func TestInconsistentEventDirectoriesAreRejected(t *testing.T) {
	for name, setup := range map[string]func(t *testing.T, r records, dir string){
		"sequence gap": func(t *testing.T, r records, dir string) {
			mustPublishEvent(t, r, 1, gitmanager.AuditEventAttemptCreated)
			mustPublishEvent(t, r, 3, gitmanager.AuditEventMutationIntent)
		},
		"duplicate sequence": func(t *testing.T, r records, dir string) {
			mustPublishEvent(t, r, 1, gitmanager.AuditEventAttemptCreated)
			mustPublishEvent(t, r, 1, gitmanager.AuditEventMutationIntent)
		},
		"not starting at one": func(t *testing.T, r records, dir string) {
			mustPublishEvent(t, r, 2, gitmanager.AuditEventAttemptCreated)
		},
		"malformed name": func(t *testing.T, r records, dir string) {
			mustPublishEvent(t, r, 1, gitmanager.AuditEventAttemptCreated)
			writeFile(t, filepath.Join(dir, "notes.txt"), 0o600)
		},
		"unknown kind": func(t *testing.T, r records, dir string) {
			writeFile(t, filepath.Join(dir, "00000000000000000001-bogus.json"), 0o600)
		},
		"non-private event": func(t *testing.T, r records, dir string) {
			writeFile(t, filepath.Join(dir, eventName(1, gitmanager.AuditEventAttemptCreated)), 0o644)
		},
		"directory entry": func(t *testing.T, r records, dir string) {
			if err := os.Mkdir(filepath.Join(dir, eventName(1, gitmanager.AuditEventAttemptCreated)), 0o700); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			r, dir, _ := newTestRecords(t)
			setup(t, r, dir)
			if _, _, err := r.listEvents("records"); !errors.Is(err, ErrInconsistentEvents) {
				t.Fatalf("listing: %v", err)
			}
		})
	}
}

// Crash evidence: a temporary is reported for supervised reconciliation
// and never counted as an event or removed.
func TestEventTemporariesAreReconciliationEvidence(t *testing.T) {
	r, dir, _ := newTestRecords(t)
	mustPublishEvent(t, r, 1, gitmanager.AuditEventAttemptCreated)
	writeFile(t, filepath.Join(dir, temporaryPrefix+"0123456789abcdef"), 0o600)
	events, temporaries, err := r.listEvents("records")
	if err != nil || len(events) != 1 || len(temporaries) != 1 {
		t.Fatalf("events %v temporaries %v err %v", events, temporaries, err)
	}
	if _, err := os.Stat(filepath.Join(dir, temporaryPrefix+"0123456789abcdef")); err != nil {
		t.Fatal("crash evidence removed")
	}
}

func mustPublishEvent(t *testing.T, r records, sequence uint64, kind gitmanager.AuditEventKind) {
	t.Helper()
	if _, err := r.publishEvent("records", sequence, kind, []byte(string(kind))); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}
