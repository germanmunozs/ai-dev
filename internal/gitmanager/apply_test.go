//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"context"
	"crypto/rand"
	"path/filepath"
	"testing"
	"time"
)

// fakeExecutor runs nothing; it checks every permit and returns scripted
// outcomes, revalidation refusals and evidence.
type fakeExecutor struct {
	t            *testing.T
	authorized   AuthorizationID
	executed     []OperationKind
	outcomes     map[int]OperationOutcome
	revalidation map[int]*PlanRejection
	evidence     ApplyEvidence
	revalidated  int
}

func (e *fakeExecutor) Revalidate(_ context.Context, _ GitInitializationPlan, operation DerivedOperation) *PlanRejection {
	e.revalidated++
	return e.revalidation[int(operation.Ordinal)]
}

func (e *fakeExecutor) Execute(_ context.Context, permit MutationPermit, _ GitInitializationPlan, operation DerivedOperation) OperationOutcome {
	if err := permit.authorizes(e.authorized, operation.OperationID); err != nil {
		e.t.Fatalf("operation %d executed without its permit: %v", operation.Ordinal, err)
	}
	e.executed = append(e.executed, operation.Kind)
	if outcome, ok := e.outcomes[int(operation.Ordinal)]; ok {
		return outcome
	}
	return OperationCompleted
}

func (e *fakeExecutor) Verify(context.Context, GitInitializationPlan) ApplyEvidence {
	return e.evidence
}

const issuedID = "123e4567-e89b-42d3-a456-426614174050"

type applyFixture struct {
	*planFixture
	store    *fakeGitStore
	executor *fakeExecutor
	plan     GitInitializationPlan
	file     []byte
	now      time.Time
}

func newApplyFixture(t *testing.T, req StrategyRequest) *applyFixture {
	f := &applyFixture{planFixture: newPlanFixture(t), store: newFakeGitStore(), now: time.Unix(1000, 0).UTC()}
	f.plan = mustPlan(t, f.planFixture, req).Plan
	data, err := EncodePlanFile(f.plan)
	if err != nil {
		t.Fatal(err)
	}
	f.file = data
	f.executor = &fakeExecutor{t: t, authorized: issuedID, evidence: ApplyEvidence{VerifiedComplete: true}}
	return f
}

func (f *applyFixture) deps() ApplyDependencies {
	return ApplyDependencies{
		Store: f.store, Clock: fixedClock{f.now}, Random: rand.Reader, Planner: f.planFixture.deps, Tool: f.request.Tool,
		Environment: validEnvironmentProfile(), Executor: f.executor,
		Inspect: func(context.Context, []byte) (GitInspection, error) { return f.inspect(), nil },
	}
}

func (f *applyFixture) issue(issue IssueRequest) GitApplyResult {
	return Apply(context.Background(), f.deps(), ApplyRequest{PlanFile: f.file, ConfirmPlan: string(f.plan.PlanDigest),
		AuthorizationID: issuedID, Issue: &issue})
}

func (f *applyFixture) consume() GitApplyResult {
	return Apply(context.Background(), f.deps(), ApplyRequest{PlanFile: f.file, ConfirmPlan: string(f.plan.PlanDigest), AuthorizationID: issuedID})
}

func operator() IssueRequest { return IssueRequest{AuthorizedBy: "operator"} }

func expectResult(t *testing.T, result GitApplyResult, status ApplyStatus, reason Reason, exit int) {
	t.Helper()
	if result.Status != status || result.Reason != reason || result.ExitCode != exit || result.Validate() != nil {
		t.Fatalf("result %+v, want %s/%s exit %d", result, status, reason, exit)
	}
}

func (f *applyFixture) durable(t *testing.T) (GitAuthorization, bool) {
	authorization, err := f.store.LoadAuthorization(context.Background(), f.plan.PreconditionSnapshot.ProjectIdentity.Digest(), issuedID)
	return authorization, err == nil
}

func (f *applyFixture) recorded(id ApplyID) bool {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()
	_, ok := f.store.results[id]
	return ok
}

func TestApplyIssueAndConsumeOnce(t *testing.T) {
	f := newApplyFixture(t, basicNewRepo())
	result := f.issue(operator())
	expectResult(t, result, ApplyStatusApplied, "", 0)
	want := []OperationKind{OperationKindRepositoryInit, OperationKindCreateProjectConfigDirectory, OperationKindVerification, OperationKindProjectConfigWrite}
	if len(f.executor.executed) != len(want) {
		t.Fatalf("executed %v", f.executor.executed)
	}
	authorization, ok := f.durable(t)
	if !ok || authorization.State != AuthorizationStateApplied || authorization.DeclaredOperator != "operator" ||
		len(authorization.AuthorizedOperationIDs()) != len(f.plan.DerivedOperations()) || !f.recorded(result.ApplyID) {
		t.Fatalf("authorization %+v", authorization)
	}
	// One mutation sequence: a later attempt is refused and recorded.
	again := f.consume()
	expectResult(t, again, ApplyStatusRefused, ReasonAuthorizationConsumed, 1)
	if again.PriorAuthorizationState != AuthorizationStateApplied || !f.recorded(again.ApplyID) || len(f.executor.executed) != len(want) {
		t.Fatalf("later attempt %+v", again)
	}
}

func TestApplyIssuanceInputs(t *testing.T) {
	for name, test := range map[string]struct {
		issue  IssueRequest
		reason Reason
	}{
		"no operator":              {IssueRequest{}, ReasonAuthorizationInputIncomplete},
		"control in operator":      {IssueRequest{AuthorizedBy: "op\x01"}, ReasonAuthorizationInputIncomplete},
		"expiry in the past":       {IssueRequest{AuthorizedBy: "op", ExpiresAt: "1970-01-01T00:00:01Z"}, ReasonAuthorizationInputIncomplete},
		"malformed expiry":         {IssueRequest{AuthorizedBy: "op", ExpiresAt: "tomorrow"}, ReasonAuthorizationInputIncomplete},
		"approval without commit":  {IssueRequest{AuthorizedBy: "op", Approvals: []string{"a|100644|" + testSHA256 + "|blob:" + testSHA1OID}}, ReasonAuthorizationMismatch},
		"commit confirmation only": {IssueRequest{AuthorizedBy: "op", ConfirmInitialCommit: true}, ReasonAuthorizationMismatch},
	} {
		t.Run(name, func(t *testing.T) {
			f := newApplyFixture(t, basicNewRepo())
			result := f.issue(test.issue)
			expectResult(t, result, ApplyStatusRefused, test.reason, 1)
			if _, exists := f.durable(t); exists || len(f.executor.executed) != 0 || !f.recorded(result.ApplyID) {
				t.Fatal("refused issuance left authority, ran operations or was not recorded")
			}
		})
	}
	t.Run("invalid ID", func(t *testing.T) {
		f := newApplyFixture(t, basicNewRepo())
		result := Apply(context.Background(), f.deps(), ApplyRequest{PlanFile: f.file, ConfirmPlan: string(f.plan.PlanDigest),
			AuthorizationID: "not-a-uuid", Issue: &IssueRequest{AuthorizedBy: "op"}})
		expectResult(t, result, ApplyStatusRefused, ReasonAuthorizationInputIncomplete, 1)
	})
	t.Run("existing ID", func(t *testing.T) {
		f := newApplyFixture(t, basicNewRepo())
		f.executor.revalidation = map[int]*PlanRejection{0: reject(ReasonRepositoryStateIncompatible, "drift")}
		f.issue(operator()) // issued, then refused before the claim
		before, _ := f.durable(t)
		result := f.issue(IssueRequest{AuthorizedBy: "someone-else"})
		expectResult(t, result, ApplyStatusRefused, ReasonAuthorizationIDExists, 1)
		after, _ := f.durable(t)
		if after.DeclaredOperator != before.DeclaredOperator || after.State != AuthorizationStateIssued {
			t.Fatalf("existing authorization changed: %+v", after)
		}
	})
}

func TestApplyBindsTheAuthorization(t *testing.T) {
	t.Run("confirmed digest", func(t *testing.T) {
		f := newApplyFixture(t, basicNewRepo())
		result := Apply(context.Background(), f.deps(), ApplyRequest{PlanFile: f.file, ConfirmPlan: testSHA256, AuthorizationID: issuedID, Issue: &IssueRequest{AuthorizedBy: "op"}})
		expectResult(t, result, ApplyStatusRefused, ReasonAuthorizationMismatch, 1)
	})
	t.Run("missing", func(t *testing.T) {
		f := newApplyFixture(t, basicNewRepo())
		expectResult(t, f.consume(), ApplyStatusRefused, ReasonAuthorizationMissing, 1)
	})
	t.Run("expired", func(t *testing.T) {
		f := newApplyFixture(t, basicNewRepo())
		f.executor.revalidation = map[int]*PlanRejection{0: reject(ReasonRepositoryStateIncompatible, "drift")}
		f.issue(IssueRequest{AuthorizedBy: "op", ExpiresAt: "1970-01-01T01:00:00Z"})
		f.executor.revalidation = nil
		f.now = time.Unix(7200, 0).UTC()
		expectResult(t, f.consume(), ApplyStatusRefused, ReasonAuthorizationExpired, 1)
	})
	t.Run("another plan's authorization", func(t *testing.T) {
		f := newApplyFixture(t, basicNewRepo())
		f.executor.revalidation = map[int]*PlanRejection{0: reject(ReasonRepositoryStateIncompatible, "drift")}
		f.issue(operator())
		other := mustPlan(t, f.planFixture, newRepo("basic", map[string]string{"initial-branch": "trunk"})).Plan
		data, _ := EncodePlanFile(other)
		f.executor.revalidation = nil
		result := Apply(context.Background(), f.deps(), ApplyRequest{PlanFile: data, ConfirmPlan: string(other.PlanDigest), AuthorizationID: issuedID})
		expectResult(t, result, ApplyStatusRefused, ReasonAuthorizationMismatch, 1)
		if authorization, _ := f.durable(t); authorization.State != AuthorizationStateIssued {
			t.Fatal("a mismatched attempt consumed the authorization")
		}
	})
}

func TestApplyTreatsThePlanFileAsUntrusted(t *testing.T) {
	f := newApplyFixture(t, basicNewRepo())
	tampered := append([]byte(" "), f.file...)
	result := Apply(context.Background(), f.deps(), ApplyRequest{PlanFile: tampered, ConfirmPlan: string(f.plan.PlanDigest),
		AuthorizationID: issuedID, Issue: &IssueRequest{AuthorizedBy: "op"}})
	expectResult(t, result, ApplyStatusRefused, ReasonPlanInvalid, 1)
	if !f.recorded(result.ApplyID) {
		t.Fatal("refusal not recorded")
	}
	// Drift between planning and apply is plan_stale.
	mustWrite(t, filepath.Join(f.root, ".ai-dev", "config.json"), "{}")
	expectResult(t, f.issue(operator()), ApplyStatusRefused, ReasonPlanStale, 1)
	if _, exists := f.durable(t); exists {
		t.Fatal("a stale plan was authorized")
	}
}

func TestApplyRevalidationPhases(t *testing.T) {
	t.Run("before the first mutation", func(t *testing.T) {
		f := newApplyFixture(t, basicNewRepo())
		f.executor.revalidation = map[int]*PlanRejection{0: reject(ReasonRepositoryStateIncompatible, "changed")}
		expectResult(t, f.issue(operator()), ApplyStatusRefused, ReasonPlanStale, 1)
		if authorization, _ := f.durable(t); authorization.State != AuthorizationStateIssued || len(f.executor.executed) != 0 {
			t.Fatalf("refusal before mutation consumed authority: %+v", authorization)
		}
		if lock, err := f.store.AcquireApplyLock(context.Background(), f.plan.PreconditionSnapshot.ProjectIdentity, testOtherApplyID); err != nil {
			t.Fatal("apply lock left held")
		} else {
			lock.Release(context.Background())
		}
	})
	t.Run("after a mutation", func(t *testing.T) {
		f := newApplyFixture(t, basicNewRepo())
		f.executor.revalidation = map[int]*PlanRejection{1: reject(ReasonRepositoryStateIncompatible, "changed")}
		f.executor.evidence = ApplyEvidence{VerifiedEffects: []Condition{{Kind: ConditionRepositoryKind, Expected: "normal"}}}
		result := f.issue(operator())
		expectResult(t, result, ApplyStatusPartiallyApplied, ReasonConcurrentChange, 1)
		if authorization, _ := f.durable(t); authorization.State != AuthorizationStatePartiallyApplied {
			t.Fatalf("durable %s", authorization.State)
		}
	})
}

func TestApplyEffectClassification(t *testing.T) {
	t.Run("zero effects proven", func(t *testing.T) {
		f := newApplyFixture(t, basicNewRepo())
		f.executor.outcomes = map[int]OperationOutcome{0: OperationFailed}
		f.executor.evidence = ApplyEvidence{ZeroEffectsProven: true}
		expectResult(t, f.issue(operator()), ApplyStatusFailed, ReasonOperationFailed, 1)
	})
	t.Run("unverifiable", func(t *testing.T) {
		f := newApplyFixture(t, basicNewRepo())
		f.executor.outcomes = map[int]OperationOutcome{0: OperationIndeterminate}
		result := f.issue(operator())
		expectResult(t, result, ApplyStatusIndeterminate, ReasonOperationFailed, 1)
		if !result.RecoveryRequired {
			t.Fatal("indeterminate result without recovery")
		}
	})
	t.Run("Git Flow without history", func(t *testing.T) {
		f := newApplyFixture(t, gitFlowRequest("avh"))
		f.executor.authorized = issuedID
		expectResult(t, f.issue(operator()), ApplyStatusApplied, ReasonDevelopmentBranchPendingInitialHistory, 0)
	})
}

func TestApplyAttemptRecordFailure(t *testing.T) {
	f := newApplyFixture(t, basicNewRepo())
	f.store.faults.failCall("CreateAttempt", 0, errInjected, false)
	result := f.issue(operator())
	expectResult(t, result, ApplyStatusRefused, ReasonAuditUnavailable, 1)
	if _, exists := f.durable(t); exists || len(f.executor.executed) != 0 {
		t.Fatal("apply continued without an attempt record")
	}
}
