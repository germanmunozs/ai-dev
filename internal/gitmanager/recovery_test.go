package gitmanager

import (
	"context"
	"errors"
	"testing"
	"time"
)

var errProofFailed = errors.New("fresh inspection found an effect")

// laterAttempt is a new invocation against the same authorization; it must
// be refused as consumed, transition nothing and record its own result.
func laterAttempt(t *testing.T, f *kernelFixture) *GitApplyResult {
	t.Helper()
	before := f.durable()
	_, refusal := f.kernel.claim(context.Background(), f.request(testOtherApplyID))
	if refusal == nil {
		t.Fatal("later attempt claimed a consumed authorization")
	}
	after := f.durable()
	if after.State != before.State || after.StateVersion != before.StateVersion || after.OwnerApplyID != before.OwnerApplyID {
		t.Fatalf("later attempt changed the authorization: %+v -> %+v", before, after)
	}
	if len(f.recordedResult(testOtherApplyID)) == 0 || !f.lockFree() {
		t.Fatal("later attempt was not recorded or kept the lock")
	}
	return refusal
}

func expectConsumed(t *testing.T, f *kernelFixture, unresolved bool) {
	t.Helper()
	refusal := laterAttempt(t, f)
	durable := f.durable()
	if refusal.Status != ApplyStatusRefused || refusal.Reason != ReasonAuthorizationConsumed || refusal.ExitCode != 1 ||
		refusal.PriorAuthorizationState != durable.State || refusal.DurableAuthorizationState != durable.State ||
		refusal.RecoveryRequired != unresolved {
		t.Fatalf("later refusal %+v against %s", *refusal, durable.State)
	}
}

func TestAttemptRecordFailurePermitsNothing(t *testing.T) {
	for _, after := range []bool{false, true} {
		store := newFakeGitStore()
		store.faults.failCall("CreateAttempt", 0, errInjected, after)
		_, err := openAttemptJournal(context.Background(), store, GitApplyAttempt{
			SchemaVersion: GitApplyAttemptSchema, ApplyID: testApplyID, StartedAt: time.Unix(1, 0).UTC(), InvocationMode: InvocationModeDirect,
		})
		if err == nil {
			t.Fatalf("after=%t: attempt failure ignored", after)
		}
		if calls := store.callsSnapshot(); len(calls) != 1 {
			t.Fatalf("after=%t: store used after attempt failure: %v", after, calls)
		}
	}
}

func TestClaimFaults(t *testing.T) {
	ctx := context.Background()
	t.Run("proven CAS failure", func(t *testing.T) {
		f := newKernelFixture(t)
		f.store.faults.failCall("TransitionAuthorization", 0, errInjected, false)
		_, refusal := f.kernel.claim(ctx, f.request(testApplyID))
		if refusal == nil || refusal.Reason != ReasonAuditUnavailable || refusal.DurableAuthorizationState != AuthorizationStateIssued {
			t.Fatalf("refusal %+v", refusal)
		}
		if durable := f.durable(); durable.State != AuthorizationStateIssued || !f.lockFree() {
			t.Fatalf("durable %+v", durable)
		}
		f.claim(testOtherApplyID)
	})
	t.Run("unverifiable CAS proven ours", func(t *testing.T) {
		f := newKernelFixture(t)
		f.store.faults.failCall("TransitionAuthorization", 0, ErrStoreOutcomeUnverifiable, true)
		_, refusal := f.kernel.claim(ctx, f.request(testApplyID))
		if refusal == nil || refusal.Status != ApplyStatusRefused || refusal.Reason != ReasonAuditUnavailable ||
			refusal.DurableAuthorizationState != AuthorizationStateIssued {
			t.Fatalf("refusal %+v", refusal)
		}
		// The reread proved the claim ours and no intent exists, so it was
		// released cleanly and the authorization is usable again.
		if durable := f.durable(); durable.State != AuthorizationStateIssued || !f.lockFree() {
			t.Fatalf("durable %+v", durable)
		}
		f.claim(testOtherApplyID)
	})
	t.Run("unverifiable CAS left claimed", func(t *testing.T) {
		f := newKernelFixture(t)
		f.store.faults.failCall("TransitionAuthorization", 0, ErrStoreOutcomeUnverifiable, true)
		f.store.faults.failCall("TransitionAuthorization", 1, errInjected, false)
		_, refusal := f.kernel.claim(ctx, f.request(testApplyID))
		if refusal == nil || refusal.Status != ApplyStatusRefused || refusal.Reason != ReasonAuditUnavailable {
			t.Fatalf("refusal %+v", refusal)
		}
		// The release failed too: with no intent the claim is an orphan
		// whose reassignment needs a fresh proof of no effects.
		if durable := f.durable(); durable.State != AuthorizationStateClaimed || durable.OwnerApplyID != testApplyID {
			t.Fatalf("durable %+v", durable)
		}
		expectConsumed(t, f, false)
		request := f.request("123e4567-e89b-42d3-a456-426614174021")
		request.proveNoEffects = func(context.Context) error { return errProofFailed }
		if _, refusal := f.kernel.claim(ctx, request); refusal == nil {
			t.Fatal("orphan reassigned without a proof")
		}
		request = f.request("123e4567-e89b-42d3-a456-426614174022")
		request.proveNoEffects = func(context.Context) error { return nil }
		session, refusal := f.kernel.claim(ctx, request)
		if refusal != nil {
			t.Fatalf("proven orphan not reassigned: %+v", *refusal)
		}
		if durable := f.durable(); durable.State != AuthorizationStateClaimed || durable.OwnerApplyID != "123e4567-e89b-42d3-a456-426614174022" {
			t.Fatalf("reassigned %+v", durable)
		}
		mustBegin(t, session, kernelOperations[0])
	})
	t.Run("claim audit failure releases the claim", func(t *testing.T) {
		f := newKernelFixture(t)
		f.store.faults.failCall("AppendAuditEvent", 0, errInjected, false)
		_, refusal := f.kernel.claim(ctx, f.request(testApplyID))
		if refusal == nil || refusal.Reason != ReasonAuditUnavailable {
			t.Fatalf("refusal %+v", refusal)
		}
		if durable := f.durable(); durable.State != AuthorizationStateIssued || durable.OwnerApplyID != "" {
			t.Fatalf("durable %+v", durable)
		}
	})
}

func TestMutationIntentFaults(t *testing.T) {
	ctx := context.Background()
	t.Run("proven failure releases the claim", func(t *testing.T) {
		f := newKernelFixture(t)
		session := f.claim(testApplyID)
		f.store.faults.failCall("AppendAuditEvent", 1, errInjected, false)
		if _, err := session.begin(ctx, kernelOperations[0]); err == nil {
			t.Fatal("mutation permitted without durable intent")
		}
		result := session.close(ctx, terminalClosure{})
		if result.Status != ApplyStatusRefused || result.Reason != ReasonAuditUnavailable || result.DurableAuthorizationState != AuthorizationStateIssued || result.RecoveryRequired {
			t.Fatalf("result %+v", result)
		}
		if durable := f.durable(); durable.State != AuthorizationStateIssued || !f.lockFree() {
			t.Fatalf("durable %+v", durable)
		}
		f.claim(testOtherApplyID)
	})
	t.Run("unverifiable intent consumes the claim", func(t *testing.T) {
		f := newKernelFixture(t)
		session := f.claim(testApplyID)
		f.store.faults.failCall("AppendAuditEvent", 1, ErrStoreOutcomeUnverifiable, true)
		if _, err := session.begin(ctx, kernelOperations[0]); err == nil {
			t.Fatal("mutation permitted with unverifiable intent")
		}
		result := session.close(ctx, terminalClosure{})
		if result.Status != ApplyStatusRefused || result.Reason != ReasonAuditUnavailable || result.DurableAuthorizationState != AuthorizationStateClaimed || !result.RecoveryRequired {
			t.Fatalf("result %+v", result)
		}
		if durable := f.durable(); durable.State != AuthorizationStateClaimed {
			t.Fatalf("durable %+v", durable)
		}
		expectConsumed(t, f, true)
	})
}

func TestMutationStartCASFaults(t *testing.T) {
	ctx := context.Background()
	t.Run("proven failure", func(t *testing.T) {
		f := newKernelFixture(t)
		session := f.claim(testApplyID)
		f.store.faults.failCall("TransitionAuthorization", 1, errInjected, false)
		if _, err := session.begin(ctx, kernelOperations[0]); err == nil {
			t.Fatal("mutation permitted without the start CAS")
		}
		result := session.close(ctx, terminalClosure{})
		if result.Status != ApplyStatusRefused || result.Reason != ReasonAuditUnavailable || result.DurableAuthorizationState != AuthorizationStateClaimed ||
			!result.RecoveryRequired || result.ExitCode != 1 {
			t.Fatalf("result %+v", result)
		}
		expectConsumed(t, f, true)
	})
	t.Run("unverifiable CAS", func(t *testing.T) {
		f := newKernelFixture(t)
		session := f.claim(testApplyID)
		f.store.faults.failCall("TransitionAuthorization", 1, ErrStoreOutcomeUnverifiable, true)
		if _, err := session.begin(ctx, kernelOperations[0]); err == nil {
			t.Fatal("mutation permitted after an unverifiable start CAS")
		}
		result := session.close(ctx, terminalClosure{})
		if result.Status != ApplyStatusIndeterminate || result.Reason != ReasonAuditFailedAfterMutation || result.DurableAuthorizationState != AuthorizationStateClaimed ||
			!result.RecoveryRequired || result.ExitCode != 1 {
			t.Fatalf("result %+v", result)
		}
		// The CAS did happen in the store: the authorization is consumed.
		if durable := f.durable(); durable.State != AuthorizationStateMutationStarted {
			t.Fatalf("durable %+v", durable)
		}
		expectConsumed(t, f, true)
	})
}

// After the start CAS, a failed operation_started record permits nothing
// further; closure still records the indeterminate outcome terminally.
func TestOperationStartedFault(t *testing.T) {
	ctx := context.Background()
	f := newKernelFixture(t)
	session := f.claim(testApplyID)
	f.store.faults.failCall("AppendAuditEvent", 3, errInjected, false)
	if _, err := session.begin(ctx, kernelOperations[0]); err == nil {
		t.Fatal("permit without a durable operation_started")
	}
	if _, err := session.begin(ctx, kernelOperations[1]); err == nil {
		t.Fatal("stopped session began another operation")
	}
	result := session.close(ctx, terminalClosure{})
	if result.Status != ApplyStatusIndeterminate || result.Reason != ReasonAuditFailedAfterMutation || result.DurableAuthorizationState != AuthorizationStateIndeterminate {
		t.Fatalf("result %+v", result)
	}
	if durable := f.durable(); durable.State != AuthorizationStateIndeterminate || durable.TerminalResultDigest == "" {
		t.Fatalf("durable %+v", durable)
	}
	expectConsumed(t, f, true)
}

func TestOperationResultFault(t *testing.T) {
	ctx := context.Background()
	f := newKernelFixture(t)
	session := f.claim(testApplyID)
	permit := mustBegin(t, session, kernelOperations[0])
	f.store.faults.failCall("AppendAuditEvent", 4, errInjected, false)
	if err := session.finish(ctx, permit, operationCompleted); err == nil {
		t.Fatal("unrecorded operation result accepted")
	}
	if _, err := session.begin(ctx, kernelOperations[1]); err == nil {
		t.Fatal("mutation continued after an audit failure")
	}
	result := session.close(ctx, terminalClosure{VerifiedComplete: true})
	if result.Status != ApplyStatusIndeterminate || result.Reason != ReasonAuditFailedAfterMutation {
		t.Fatalf("result %+v", result)
	}
}

func TestTerminalClosureFaults(t *testing.T) {
	ctx := context.Background()
	run := func(t *testing.T, f *kernelFixture) GitApplyResult {
		session := f.claim(testApplyID)
		for _, op := range kernelOperations {
			mustFinish(t, session, mustBegin(t, session, op), operationCompleted)
		}
		return session.close(ctx, terminalClosure{VerifiedComplete: true})
	}
	t.Run("prepared result", func(t *testing.T) {
		f := newKernelFixture(t)
		// Events: claim transition, intent, start transition, 2x(started,
		// completed), lock release start and result, then the prepared result.
		f.store.faults.failCall("AppendAuditEvent", 9, errInjected, false)
		result := run(t, f)
		if result.Status != ApplyStatusIndeterminate || result.Reason != ReasonAuditFailedAfterMutation || result.DurableAuthorizationState != AuthorizationStateMutationStarted ||
			!result.RecoveryRequired {
			t.Fatalf("result %+v", result)
		}
		if durable := f.durable(); durable.State != AuthorizationStateMutationStarted || durable.TerminalResultDigest != "" {
			t.Fatalf("durable %+v", durable)
		}
		expectConsumed(t, f, true)
	})
	t.Run("apply lock release", func(t *testing.T) {
		f := newKernelFixture(t)
		f.store.faults.failCall("ApplyLock.Release", 0, errInjected, false)
		result := run(t, f)
		failures := result.SecondaryFailures()
		if result.Status != ApplyStatusApplied || result.Reason != ReasonResourceReleaseFailed || result.ExitCode != 1 ||
			len(failures) != 1 || failures[0].ResourceRole != ResourceRoleApplyLock {
			t.Fatalf("result %+v %+v", result, failures)
		}
		if durable := f.durable(); durable.State != AuthorizationStateApplied {
			t.Fatalf("durable %+v", durable)
		}
	})
	t.Run("proven terminal CAS failure", func(t *testing.T) {
		f := newKernelFixture(t)
		f.store.faults.failCall("TransitionAuthorization", 2, errInjected, false)
		result := run(t, f)
		if result.Status != ApplyStatusIndeterminate || result.Reason != ReasonAuditFailedAfterMutation || result.DurableAuthorizationState != AuthorizationStateMutationStarted {
			t.Fatalf("result %+v", result)
		}
		if durable := f.durable(); durable.State != AuthorizationStateMutationStarted {
			t.Fatalf("durable %+v", durable)
		}
		expectConsumed(t, f, true)
	})
	t.Run("unverifiable terminal CAS", func(t *testing.T) {
		f := newKernelFixture(t)
		f.store.faults.failCall("TransitionAuthorization", 2, ErrStoreOutcomeUnverifiable, true)
		result := run(t, f)
		// The invocation cannot claim the outcome even though the store
		// holds it; a later read observes the terminal state.
		if result.Status != ApplyStatusIndeterminate || result.Reason != ReasonAuditFailedAfterMutation || result.DurableAuthorizationState != AuthorizationStateMutationStarted {
			t.Fatalf("result %+v", result)
		}
		if durable := f.durable(); durable.State != AuthorizationStateApplied {
			t.Fatalf("durable %+v", durable)
		}
		expectConsumed(t, f, false)
	})
}

// A conclusive authorization is consumed for every later attempt without a
// pending recovery.
func TestConsumedAfterConclusiveClosure(t *testing.T) {
	f := newKernelFixture(t)
	session := f.claim(testApplyID)
	for _, op := range kernelOperations {
		mustFinish(t, session, mustBegin(t, session, op), operationCompleted)
	}
	session.close(context.Background(), terminalClosure{VerifiedComplete: true})
	expectConsumed(t, f, false)
}

// Crash evidence in the owner's trail means intent cannot be proven absent,
// so the claim is consumed rather than an orphan.
func TestCrashEvidenceConsumesClaim(t *testing.T) {
	f := newKernelFixture(t)
	f.claim(testApplyID)
	f.store.mu.Lock()
	f.store.crashEvidence[testApplyID] = true
	f.store.locks = make(map[string]ApplyID) // the owner process is gone
	f.store.mu.Unlock()
	request := f.request(testOtherApplyID)
	request.proveNoEffects = func(context.Context) error { return nil }
	_, refusal := f.kernel.claim(context.Background(), request)
	if refusal == nil || refusal.Reason != ReasonAuthorizationConsumed || !refusal.RecoveryRequired {
		t.Fatalf("refusal %+v", refusal)
	}
}

func TestAssessAuthorization(t *testing.T) {
	f := newKernelFixture(t)
	now := time.Unix(100, 0).UTC()
	assessment := assessAuthorization(context.Background(), f.store, f.authorization, now)
	if !assessment.usable || assessment.refusal != "" {
		t.Fatalf("issued %+v", assessment)
	}
	expired := assessAuthorization(context.Background(), f.store, f.authorization, f.authorization.ExpiresAt)
	if expired.usable || expired.refusal != ReasonAuthorizationExpired {
		t.Fatalf("expired %+v", expired)
	}
	unexpiring := f.authorization
	unexpiring.ExpiresAt = time.Time{}
	if a := assessAuthorization(context.Background(), f.store, unexpiring, now.Add(1e6*time.Hour)); !a.usable {
		t.Fatalf("authorization without expiry %+v", a)
	}
	for _, state := range []AuthorizationState{AuthorizationStateApplied, AuthorizationStateFailed, AuthorizationStatePartiallyApplied} {
		terminal := f.authorization
		terminal.State = state
		if a := assessAuthorization(context.Background(), f.store, terminal, now); a.usable || a.refusal != ReasonAuthorizationConsumed || a.unresolved {
			t.Errorf("%s: %+v", state, a)
		}
	}
	for _, state := range []AuthorizationState{AuthorizationStateMutationStarted, AuthorizationStateIndeterminate} {
		consumed := f.authorization
		consumed.State = state
		if a := assessAuthorization(context.Background(), f.store, consumed, now); a.usable || a.refusal != ReasonAuthorizationConsumed || !a.unresolved {
			t.Errorf("%s: %+v", state, a)
		}
	}
	// A claim whose owner attempt cannot be read is consumed, not orphaned.
	unknownOwner := f.authorization
	unknownOwner.State, unknownOwner.OwnerApplyID = AuthorizationStateClaimed, testOtherApplyID
	if a := assessAuthorization(context.Background(), f.store, unknownOwner, now); a.orphan || a.refusal != ReasonAuthorizationConsumed || !a.unresolved {
		t.Errorf("unreadable owner trail: %+v", a)
	}
}

// T126: abandon is idempotent once closed, and after intent it is a
// closure, never a release of the claim to issued.
func TestAbandonAfterClosureAndAfterIntent(t *testing.T) {
	ctx := context.Background()
	f := newKernelFixture(t)
	session := f.claim(testApplyID)
	first := session.abandon(ctx, ReasonPlanStale)
	calls := len(f.store.callsSnapshot())
	if again := session.abandon(ctx, ReasonOperationFailed); again.ApplyID != first.ApplyID || again.Reason != first.Reason ||
		len(f.store.callsSnapshot()) != calls {
		t.Fatalf("a second abandon %+v after %+v touched the store", again, first)
	}

	f = newKernelFixture(t)
	session = f.claim(testApplyID)
	mustBegin(t, session, kernelOperations[0])
	result := session.abandon(ctx, ReasonPlanStale)
	if result.Status == ApplyStatusRefused || result.DurableAuthorizationState == AuthorizationStateIssued {
		t.Fatalf("abandon after intent %+v", result)
	}
	if durable := f.durable(); durable.State == AuthorizationStateIssued || durable.State == AuthorizationStateClaimed {
		t.Fatalf("durable %s after intent", durable.State)
	}
}

// T126: a refusal whose result record cannot be published is returned as
// audit_unavailable with the audit status unavailable.
func TestUnrecordedRefusalIsAuditUnavailable(t *testing.T) {
	f := newKernelFixture(t)
	session := f.claim(testApplyID)
	f.store.faults.failCall("RecordAttemptResult", 0, errInjected, false)
	result := session.abandon(context.Background(), ReasonPlanStale)
	if result.Status != ApplyStatusRefused || result.Reason != ReasonAuditUnavailable || result.AuditStatus != auditStatusUnavailable || result.ExitCode != 1 {
		t.Fatalf("result %+v", result)
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
}

// T126: render never returns an invalid result. A combination the model
// rejects is rendered conservatively: an outcome as operation_failed and a
// refusal as audit_unavailable. As a last resort it is an unrecorded
// refusal before any mutation intent, and state_indeterminate needing
// recovery once a mutation may have happened, never a refusal.
func TestRenderFallsBackToAValidResult(t *testing.T) {
	f := newKernelFixture(t)
	before := f.claim(testApplyID)
	failed := GitApplyResultInput{Status: ApplyStatusFailed, Reason: "not-a-reason", DurableAuthorizationState: AuthorizationStateFailed}
	for name, test := range map[string]struct {
		in     GitApplyResultInput
		status ApplyStatus
		reason Reason
	}{
		"refusal with an invalid reason": {GitApplyResultInput{Status: ApplyStatusRefused, Reason: "not-a-reason", DurableAuthorizationState: AuthorizationStateClaimed},
			ApplyStatusRefused, ReasonAuditUnavailable},
		"failure with an invalid reason": {failed, ApplyStatusFailed, ReasonOperationFailed},
		"invalid status":                 {GitApplyResultInput{Status: "not-a-status", Reason: ReasonPlanStale}, ApplyStatusRefused, ReasonAuditUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			result := before.render(test.in)
			if err := result.Validate(); err != nil || result.Status != test.status || result.Reason != test.reason {
				t.Fatalf("render %+v: %v", result, err)
			}
		})
	}
	f = newKernelFixture(t)
	after := f.claim(testApplyID)
	mustBegin(t, after, kernelOperations[0])
	for name, in := range map[string]GitApplyResultInput{
		"invalid status":            {Status: "not-a-status"},
		"failure without its state": {Status: ApplyStatusFailed, Reason: "not-a-reason"},
	} {
		t.Run("after intent "+name, func(t *testing.T) {
			result := after.render(in)
			if err := result.Validate(); err != nil || result.Status != ApplyStatusIndeterminate || !result.RecoveryRequired ||
				result.AuthorizationID != f.authorization.AuthorizationID {
				t.Fatalf("render %+v: %v", result, err)
			}
		})
	}
}

// T126: the exit code follows validateResultExit for every rule.
func TestExitCodeFor(t *testing.T) {
	for _, test := range []struct {
		status    ApplyStatus
		reason    Reason
		secondary bool
		want      int
	}{
		{ApplyStatusApplied, "", false, 0},
		{ApplyStatusAlreadyApplied, "", false, 0},
		{ApplyStatusApplied, "", true, 1},
		{ApplyStatusApplied, ReasonOutputFailed, false, 2},
		{ApplyStatusRefused, ReasonInteractiveTerminalRequired, false, 2},
		{ApplyStatusRefused, ReasonInvalidStrategy, false, 2},
		{ApplyStatusRefused, ReasonMissingRequiredBranch, false, 2},
		{ApplyStatusRefused, ReasonOutputExists, false, 2},
		{ApplyStatusRefused, ReasonUnsafeOutputPath, false, 2},
		{ApplyStatusRefused, ReasonPlanStale, false, 1},
		{ApplyStatusIndeterminate, ReasonOperationFailed, false, 1},
	} {
		if got := exitCodeFor(test.status, test.reason, test.secondary); got != test.want {
			t.Errorf("exitCodeFor(%s, %s, %v) = %d, want %d", test.status, test.reason, test.secondary, got, test.want)
		}
	}
}
