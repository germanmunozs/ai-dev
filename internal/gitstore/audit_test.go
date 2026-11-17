//go:build (darwin && !ios) || (linux && !android)

package gitstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func testAttempt(t *testing.T, project gitmanager.PhysicalProjectIdentity) gitmanager.GitApplyAttempt {
	t.Helper()
	return gitmanager.GitApplyAttempt{
		SchemaVersion: gitmanager.GitApplyAttemptSchema, ApplyID: testApplyID, AuthorizationID: testAuthorizationID,
		ProjectIdentity: project, StartedAt: time.Unix(20, 0).UTC(), InvocationMode: gitmanager.InvocationModeDirect,
	}
}

func auditEvent(project gitmanager.PhysicalProjectIdentity, sequence uint64, kind gitmanager.AuditEventKind, operation gitmanager.OperationID) gitmanager.AuditEvent {
	return gitmanager.AuditEvent{
		Kind: kind, ProjectIdentityDigest: project.Digest(), ApplyID: testApplyID, Sequence: sequence,
		AuthorizationID: testAuthorizationID, OperationID: operation, Record: []byte(`{"kind":"` + string(kind) + `"}`),
	}
}

// The attempt record precedes every event and result, is private and
// immutable, and an attempt ID is never reused.
func TestCreateAttemptAndResult(t *testing.T) {
	ctx := context.Background()
	store, path := openTestStore(t)
	project := testProject(t, privateTempDir(t))
	attempt := testAttempt(t, project)
	if _, err := store.AppendAuditEvent(ctx, auditEvent(project, 1, gitmanager.AuditEventMutationIntent, "")); err == nil {
		t.Fatal("event appended before its attempt record")
	}
	if _, err := store.RecordAttemptResult(ctx, testApplyID, []byte(`{}`)); err == nil {
		t.Fatal("result recorded before its attempt record")
	}
	if err := store.CreateAttempt(ctx, attempt); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(path, "attempts", string(testApplyID))
	want, _ := gitmanager.EncodeAttemptRecord(attempt)
	if data, err := os.ReadFile(filepath.Join(dir, "attempt.json")); err != nil || string(data) != string(want) {
		t.Fatalf("attempt record %q %v", data, err)
	}
	for _, sub := range []string{"", "events"} {
		if info, err := os.Lstat(filepath.Join(dir, sub)); err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("%s: %v %v", sub, info, err)
		}
	}
	if err := store.CreateAttempt(ctx, attempt); !errors.Is(err, ErrRecordExists) {
		t.Fatalf("attempt reused: %v", err)
	}
	digest, err := store.RecordAttemptResult(ctx, testApplyID, []byte(`{"status":"refused"}`))
	if err != nil || digest.Validate() != nil {
		t.Fatalf("result %q %v", digest, err)
	}
	if _, err := store.RecordAttemptResult(ctx, testApplyID, []byte(`{"status":"applied"}`)); !errors.Is(err, ErrRecordExists) {
		t.Fatalf("result replaced: %v", err)
	}
	// An attempt with no resolved project is still recorded.
	unresolved := gitmanager.GitApplyAttempt{SchemaVersion: gitmanager.GitApplyAttemptSchema, ApplyID: testOtherApplyID,
		StartedAt: time.Unix(20, 0).UTC(), InvocationMode: gitmanager.InvocationModeDirect}
	if err := store.CreateAttempt(ctx, unresolved); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateAttempt(ctx, gitmanager.GitApplyAttempt{ApplyID: testApplyID}); err == nil {
		t.Fatal("invalid attempt accepted")
	}
}

func TestCreateAttemptDurabilityFaults(t *testing.T) {
	ctx := context.Background()
	store, path := openTestStore(t)
	project := testProject(t, privateTempDir(t))
	store.records.ops = failAt(store.records.ops, "sync-file")
	if err := store.CreateAttempt(ctx, testAttempt(t, project)); !errors.Is(err, errInjectedFault) {
		t.Fatalf("attempt with sync fault: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(path, "attempts", string(testApplyID), "attempt.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed attempt record published")
	}
	store.records.ops = failAt(defaultRecordOps(), "sync-directory")
	attempt := testAttempt(t, project)
	attempt.ApplyID = testOtherApplyID
	if err := store.CreateAttempt(ctx, attempt); !errors.Is(err, gitmanager.ErrStoreOutcomeUnverifiable) {
		t.Fatalf("attempt with directory sync fault: %v", err)
	}
}

// The store accepts only the next sequence and only an order a correct
// apply could have produced, so no record can claim an effect that has no
// started operation, or a closure that has no prepared result.
func TestAuditEventOrdering(t *testing.T) {
	ctx := context.Background()
	store, _ := openTestStore(t)
	project := testProject(t, privateTempDir(t))
	if err := store.CreateAttempt(ctx, testAttempt(t, project)); err != nil {
		t.Fatal(err)
	}
	op := gitmanager.OperationID("0:" + testDigest)
	appendAt := func(sequence uint64, kind gitmanager.AuditEventKind, operation gitmanager.OperationID) error {
		_, err := store.AppendAuditEvent(ctx, auditEvent(project, sequence, kind, operation))
		return err
	}
	mustAppend := func(sequence uint64, kind gitmanager.AuditEventKind, operation gitmanager.OperationID) {
		t.Helper()
		if err := appendAt(sequence, kind, operation); err != nil {
			t.Fatalf("%d %s: %v", sequence, kind, err)
		}
	}
	refuse := func(sequence uint64, kind gitmanager.AuditEventKind, operation gitmanager.OperationID) {
		t.Helper()
		if err := appendAt(sequence, kind, operation); err == nil {
			t.Fatalf("%d %s accepted", sequence, kind)
		}
	}
	refuse(1, gitmanager.AuditEventAttemptCreated, "")
	refuse(2, gitmanager.AuditEventAuthorizationIssued, "")
	refuse(1, gitmanager.AuditEventOperationStarted, op)
	refuse(1, gitmanager.AuditEventOperationCompleted, op)
	refuse(1, gitmanager.AuditEventPreparedTerminalResult, "")
	refuse(1, gitmanager.AuditEventTerminalAuthorizationCAS, "")
	mustAppend(1, gitmanager.AuditEventAuthorizationIssued, "")
	mustAppend(2, gitmanager.AuditEventAuthorizationTransition, "")
	mustAppend(3, gitmanager.AuditEventMutationIntent, "")
	refuse(4, gitmanager.AuditEventMutationIntent, "")
	refuse(4, gitmanager.AuditEventAuthorizationIssued, "")
	mustAppend(4, gitmanager.AuditEventAuthorizationTransition, "")
	mustAppend(5, gitmanager.AuditEventOperationStarted, op)
	refuse(6, gitmanager.AuditEventOperationStarted, op)
	refuse(6, gitmanager.AuditEventResourceReleaseStarted, "")
	refuse(6, gitmanager.AuditEventPreparedTerminalResult, "")
	mustAppend(6, gitmanager.AuditEventOperationCompleted, op)
	refuse(7, gitmanager.AuditEventOperationFailed, op)
	refuse(7, gitmanager.AuditEventResourceReleaseResult, "")
	mustAppend(7, gitmanager.AuditEventResourceReleaseStarted, "")
	refuse(8, gitmanager.AuditEventOperationStarted, op)
	refuse(8, gitmanager.AuditEventPreparedTerminalResult, "")
	mustAppend(8, gitmanager.AuditEventResourceReleaseResult, "")
	refuse(9, gitmanager.AuditEventTerminalAuthorizationCAS, "")
	refuse(10, gitmanager.AuditEventPreparedTerminalResult, "")
	mustAppend(9, gitmanager.AuditEventPreparedTerminalResult, "")
	refuse(10, gitmanager.AuditEventResourceReleaseStarted, "")
	refuse(10, gitmanager.AuditEventPreparedTerminalResult, "")
	mustAppend(10, gitmanager.AuditEventTerminalAuthorizationCAS, "")
	refuse(11, gitmanager.AuditEventAuthorizationTransition, "")

	trail, err := store.ListAuditEvents(ctx, testApplyID)
	if err != nil || len(trail.Events) != 10 || trail.CrashEvidence || trail.Events[2] != gitmanager.AuditEventMutationIntent {
		t.Fatalf("trail %+v %v", trail, err)
	}
}

func TestAuditTrailReportsCrashEvidence(t *testing.T) {
	ctx := context.Background()
	store, path := openTestStore(t)
	project := testProject(t, privateTempDir(t))
	if err := store.CreateAttempt(ctx, testAttempt(t, project)); err != nil {
		t.Fatal(err)
	}
	trail, err := store.ListAuditEvents(ctx, testApplyID)
	if err != nil || len(trail.Events) != 0 || trail.CrashEvidence {
		t.Fatalf("fresh trail %+v %v", trail, err)
	}
	events := filepath.Join(path, "attempts", string(testApplyID), "events")
	writeFile(t, filepath.Join(events, temporaryPrefix+"0123456789abcdef"), 0o600)
	trail, err = store.ListAuditEvents(ctx, testApplyID)
	if err != nil || !trail.CrashEvidence || trail.Contains(gitmanager.AuditEventMutationIntent) {
		t.Fatalf("trail with temporary %+v %v", trail, err)
	}
	writeFile(t, filepath.Join(events, eventName(2, gitmanager.AuditEventMutationIntent)), 0o600)
	if _, err := store.ListAuditEvents(ctx, testApplyID); !errors.Is(err, ErrInconsistentEvents) {
		t.Fatalf("gapped trail: %v", err)
	}
	if _, err := store.ListAuditEvents(ctx, testOtherApplyID); err == nil {
		t.Fatal("trail of an unknown attempt")
	}
}
