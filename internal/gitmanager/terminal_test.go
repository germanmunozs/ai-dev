package gitmanager

import (
	"bytes"
	"context"
	"reflect"
	"testing"
)

func TestClassifyEffects(t *testing.T) {
	completedBoth := operationSummary{completed: kernelOperations}
	completedOne := operationSummary{completed: kernelOperations[:1], notStarted: kernelOperations[1:]}
	failedFirst := operationSummary{failed: kernelOperations[0], notStarted: kernelOperations[1:]}
	open := operationSummary{completed: kernelOperations[:1], indeterminate: kernelOperations[1:]}
	for name, test := range map[string]struct {
		closure     terminalClosure
		operations  operationSummary
		auditFailed bool
		status      ApplyStatus
		reason      Reason
	}{
		"verified complete":             {terminalClosure{VerifiedComplete: true}, completedBoth, false, ApplyStatusApplied, ""},
		"complete with dev ref pending": {terminalClosure{VerifiedComplete: true, PrimaryReason: ReasonDevelopmentBranchPendingInitialHistory}, completedBoth, false, ApplyStatusApplied, ReasonDevelopmentBranchPendingInitialHistory},
		// A complete result cannot carry a failure cause; the contradiction
		// is classified conservatively.
		"complete claim with failure cause": {terminalClosure{VerifiedComplete: true, PrimaryReason: ReasonConcurrentChange}, completedBoth, false, ApplyStatusIndeterminate, ReasonConcurrentChange},
		"zero effects proven":               {terminalClosure{ZeroEffectsProven: true}, failedFirst, false, ApplyStatusFailed, ReasonOperationFailed},
		"zero effects contradicted":         {terminalClosure{ZeroEffectsProven: true}, completedOne, false, ApplyStatusIndeterminate, ReasonOperationFailed},
		"partial":                           {terminalClosure{PrimaryReason: ReasonConcurrentChange}, completedOne, false, ApplyStatusPartiallyApplied, ReasonConcurrentChange},
		"unverifiable effect":               {terminalClosure{EffectsUnverifiable: true}, completedBoth, false, ApplyStatusIndeterminate, ReasonOperationFailed},
		"open operation":                    {terminalClosure{VerifiedComplete: true}, open, false, ApplyStatusIndeterminate, ReasonOperationFailed},
		"no proof either way":               {terminalClosure{}, failedFirst, false, ApplyStatusIndeterminate, ReasonOperationFailed},
		"audit failure":                     {terminalClosure{VerifiedComplete: true}, completedBoth, true, ApplyStatusIndeterminate, ReasonAuditFailedAfterMutation},
	} {
		status, reason := classifyEffects(test.closure, test.operations, test.auditFailed)
		if status != test.status || reason != test.reason {
			t.Errorf("%s: %s/%s, want %s/%s", name, status, reason, test.status, test.reason)
		}
	}
}

// The conclusive result is the prepared record whose digest the terminal
// CAS bound, published after every release, with the apply lock last.
func TestTerminalClosureIsConclusiveOnlyThroughTheCAS(t *testing.T) {
	ctx := context.Background()
	f := newKernelFixture(t)
	session := f.claim(testApplyID)
	index := &fakeResource{}
	if err := session.addResource(ownedRelease{Role: ResourceRoleIndexLock, Operation: kernelOperations[1], Ordinal: 1, Release: index.release}); err != nil {
		t.Fatal(err)
	}
	if err := session.addResource(ownedRelease{Role: ResourceRoleApplyLock, Ordinal: 9, Release: index.release}); err == nil {
		t.Fatal("a second apply lock was registered")
	}
	for _, op := range kernelOperations {
		mustFinish(t, session, mustBegin(t, session, op), operationCompleted)
	}
	result := session.close(ctx, terminalClosure{VerifiedComplete: true, AfterSnapshotDigest: testSHA256})
	if result.Status != ApplyStatusApplied || result.Reason != "" || result.ExitCode != 0 || result.DurableAuthorizationState != AuthorizationStateApplied ||
		result.PriorAuthorizationState != AuthorizationStateIssued || !reflect.DeepEqual(result.CompletedOperationIDs(), kernelOperations) || result.RecoveryRequired {
		t.Fatalf("result %+v", result)
	}
	events := f.store.eventsSnapshot()
	var kinds []AuditEventKind
	for _, event := range events {
		kinds = append(kinds, event.Kind)
	}
	tail := []AuditEventKind{
		AuditEventResourceReleaseStarted, AuditEventResourceReleaseResult, AuditEventResourceReleaseStarted, AuditEventResourceReleaseResult,
		AuditEventPreparedTerminalResult, AuditEventTerminalAuthorizationCAS,
	}
	if !reflect.DeepEqual(kinds[len(kinds)-len(tail):], tail) {
		t.Fatalf("closing events %v", kinds)
	}
	if index.released != 1 {
		t.Fatal("owned index lock not released")
	}
	prepared := events[len(events)-2]
	durable := f.durable()
	if durable.State != AuthorizationStateApplied || durable.TerminalResultDigest != recordDigest(prepared.Record) {
		t.Fatalf("terminal digest %q, prepared %q", durable.TerminalResultDigest, recordDigest(prepared.Record))
	}
	// The prepared record embeds exactly the rendered result, and the same
	// bytes are the attempt result.
	rendered, err := encodeApplyResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(prepared.Record, rendered) || !bytes.Equal(f.recordedResult(testApplyID), rendered) {
		t.Fatalf("prepared %s\nrendered %s", prepared.Record, rendered)
	}
	if !f.lockFree() {
		t.Fatal("apply lock still held")
	}
}

// A secondary failure never changes effect-based status, keeps an earlier
// primary reason, and forces exit 1.
func TestTerminalClosureSecondaryFailures(t *testing.T) {
	ctx := context.Background()
	run := func(t *testing.T, closure terminalClosure, finishSecond bool) GitApplyResult {
		f := newKernelFixture(t)
		session := f.claim(testApplyID)
		temporary := &fakeResource{err: errInjected}
		if err := session.addResource(ownedRelease{Role: ResourceRoleConfigTemporary, Operation: kernelOperations[0], Ordinal: 1, Release: temporary.release}); err != nil {
			t.Fatal(err)
		}
		mustFinish(t, session, mustBegin(t, session, kernelOperations[0]), operationCompleted)
		if finishSecond {
			mustFinish(t, session, mustBegin(t, session, kernelOperations[1]), operationFailed)
		}
		return session.close(ctx, closure)
	}
	sole := run(t, terminalClosure{VerifiedComplete: true}, false)
	// Only the first operation ran, so the result is partial, not applied.
	if sole.Status != ApplyStatusPartiallyApplied || sole.Reason != ReasonOperationFailed || sole.ExitCode != 1 || len(sole.SecondaryFailures()) != 1 {
		t.Fatalf("partial with release failure %+v", sole)
	}
	primary := run(t, terminalClosure{PrimaryReason: ReasonConcurrentChange}, true)
	failures := primary.SecondaryFailures()
	if primary.Status != ApplyStatusPartiallyApplied || primary.Reason != ReasonConcurrentChange || primary.ExitCode != 1 ||
		len(failures) != 1 || failures[0].ResourceRole != ResourceRoleConfigTemporary || failures[0].OperationID != kernelOperations[0] ||
		primary.FailedOperationID != kernelOperations[1] {
		t.Fatalf("primary plus secondary %+v %+v", primary, failures)
	}
}

func TestTerminalClosureFailedNeedsProofOfZeroEffects(t *testing.T) {
	ctx := context.Background()
	for _, proven := range []bool{true, false} {
		f := newKernelFixture(t)
		session := f.claim(testApplyID)
		mustFinish(t, session, mustBegin(t, session, kernelOperations[0]), operationFailed)
		result := session.close(ctx, terminalClosure{ZeroEffectsProven: proven})
		want := ApplyStatusIndeterminate
		if proven {
			want = ApplyStatusFailed
		}
		if result.Status != want || result.FailedOperationID != kernelOperations[0] || !reflect.DeepEqual(result.NotStartedOperationIDs(), kernelOperations[1:]) {
			t.Fatalf("proven=%t: %+v", proven, result)
		}
		if durable := f.durable(); string(durable.State) != string(want) {
			t.Fatalf("proven=%t: durable %s", proven, durable.State)
		}
	}
}
