//go:build (darwin && !ios) || (linux && !android)

package gitmanager_test

import (
	"bytes"
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/germanmunozs/ai-dev/internal/gitmanager"
)

const (
	reconcilingAuthorizationID = "123e4567-e89b-42d3-a456-426614174103"
	unrelatedAuthorizationID   = "123e4567-e89b-42d3-a456-426614174104"
)

// scriptedExecutor refuses preflight on demand, returns one outcome for
// every operation and the given evidence, and counts executions.
type scriptedExecutor struct {
	preflight *gitmanager.PlanRejection
	outcome   gitmanager.OperationOutcome
	evidence  gitmanager.ApplyEvidence
	delay     time.Duration
	executed  atomic.Int64
}

func (e *scriptedExecutor) Preflight(context.Context, gitmanager.GitInitializationPlan) *gitmanager.PlanRejection {
	return e.preflight
}
func (e *scriptedExecutor) Revalidate(context.Context, gitmanager.GitInitializationPlan, gitmanager.DerivedOperation) *gitmanager.PlanRejection {
	return nil
}
func (e *scriptedExecutor) Execute(context.Context, gitmanager.MutationPermit, gitmanager.GitInitializationPlan, gitmanager.DerivedOperation) gitmanager.OperationOutcome {
	e.executed.Add(1)
	time.Sleep(e.delay)
	return e.outcome
}
func (e *scriptedExecutor) Verify(context.Context, gitmanager.GitInitializationPlan) gitmanager.ApplyEvidence {
	return e.evidence
}

func (it *integration) applyWith(executor gitmanager.OperationExecutor, id string, issue *gitmanager.IssueRequest) gitmanager.GitApplyResult {
	deps := it.deps
	deps.Executor = executor
	return gitmanager.Apply(context.Background(), deps, gitmanager.ApplyRequest{PlanFile: it.file, ConfirmPlan: string(it.plan.PlanDigest),
		AuthorizationID: id, Issue: issue})
}

func (it *integration) durable(id string) gitmanager.GitAuthorization {
	it.t.Helper()
	authorization, err := it.store.LoadAuthorization(context.Background(), it.plan.PreconditionSnapshot.ProjectIdentity.Digest(), gitmanager.AuthorizationID(id))
	if err != nil {
		it.t.Fatal(err)
	}
	return authorization
}

func encoded(t *testing.T, authorization gitmanager.GitAuthorization) []byte {
	data, err := gitmanager.EncodeAuthorizationRecord(authorization)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// T103/AC-006/AC-021: an indeterminate authorization is never retried. The
// only way on is a read-only reconciliation inspection, a new plan and a
// new authorization linked to the prior one, which stays byte-identical;
// unrelated authorizations remain possible (F008-FR-128–129, 168, 180).
func TestSupervisedReconciliation(t *testing.T) {
	it := newIntegration(t)
	it.planBasic()
	indeterminate := &scriptedExecutor{outcome: gitmanager.OperationIndeterminate, evidence: gitmanager.ApplyEvidence{EffectsUnverifiable: true}}
	first := it.applyWith(indeterminate, integrationAuthorizationID, &gitmanager.IssueRequest{AuthorizedBy: "operator"})
	if first.Status != gitmanager.ApplyStatusIndeterminate || indeterminate.executed.Load() != 1 {
		t.Fatalf("setup %+v", first)
	}
	prior := it.durable(integrationAuthorizationID)
	priorBytes := encoded(t, prior)

	// No retry: a later attempt only records its own refusal.
	again := it.applyWith(indeterminate, integrationAuthorizationID, nil)
	if again.Status != gitmanager.ApplyStatusRefused || again.Reason != gitmanager.ReasonAuthorizationConsumed || !again.RecoveryRequired ||
		indeterminate.executed.Load() != 1 {
		t.Fatalf("retry %+v", again)
	}

	// Steps 1–2: read-only inspection of the prior outcome.
	deps := gitmanager.ReconciliationDependencies{Store: it.store, Inspect: it.deps.Inspect}
	report, rejection := gitmanager.InspectReconciliation(context.Background(), deps,
		gitmanager.ReconciliationRequest{PlanFile: it.file, AuthorizationID: integrationAuthorizationID})
	if rejection != nil || !report.Unresolved || !report.TrailReadable || !report.Trail.Contains(gitmanager.AuditEventMutationIntent) ||
		report.Prior.State != gitmanager.AuthorizationStateIndeterminate || len(report.Absent) == 0 || report.GrantsMutationAuthority() {
		t.Fatalf("report %+v %+v", report, rejection)
	}
	if _, rejection := gitmanager.InspectReconciliation(context.Background(), deps,
		gitmanager.ReconciliationRequest{PlanFile: it.file, AuthorizationID: unrelatedAuthorizationID}); rejection == nil ||
		rejection.Reason != gitmanager.ReasonAuthorizationMissing {
		t.Fatalf("missing prior %+v", rejection)
	}

	// Steps 3–4: a new plan and a new linked authorization.
	it.planBasic()
	completing := &scriptedExecutor{outcome: gitmanager.OperationCompleted, evidence: gitmanager.ApplyEvidence{VerifiedComplete: true}}
	linked := it.applyWith(completing, reconcilingAuthorizationID,
		&gitmanager.IssueRequest{AuthorizedBy: "operator", ReconcilesAuthorizationID: integrationAuthorizationID})
	if linked.Status != gitmanager.ApplyStatusApplied {
		t.Fatalf("reconciliation apply %+v", linked)
	}
	issued := it.durable(reconcilingAuthorizationID)
	if issued.PreviousAuthorizationID != integrationAuthorizationID || issued.PreviousApplyID != prior.OwnerApplyID ||
		issued.PreviousPlanDigest != prior.PlanDigest {
		t.Fatalf("linked authorization %+v", issued)
	}
	if !bytes.Equal(encoded(t, it.durable(integrationAuthorizationID)), priorBytes) {
		t.Fatal("reconciliation changed the prior authorization")
	}

	// A conclusive authorization is not reconcilable; an unrelated new one
	// is still allowed.
	conclusive := it.applyWith(completing, unrelatedAuthorizationID,
		&gitmanager.IssueRequest{AuthorizedBy: "operator", ReconcilesAuthorizationID: reconcilingAuthorizationID})
	if conclusive.Status != gitmanager.ApplyStatusRefused || conclusive.Reason != gitmanager.ReasonAuthorizationMismatch {
		t.Fatalf("reconciling a conclusive authorization %+v", conclusive)
	}
	if unrelated := it.applyWith(completing, unrelatedAuthorizationID, &gitmanager.IssueRequest{AuthorizedBy: "operator"}); unrelated.Status != gitmanager.ApplyStatusApplied {
		t.Fatalf("unrelated authorization %+v", unrelated)
	}
}

// T104/AC-005/AC-021: two attempts race to consume one issued
// authorization, repeatedly. Exactly one claims and runs the operations,
// once; the other is refused without executing anything; the sole closer
// leaves the authorization applied with one transition history.
func TestConcurrentConsumption(t *testing.T) {
	for round := 0; round < 10; round++ {
		it := newIntegration(t)
		it.planBasic()
		// Issue without consuming: issuance precedes the preflight.
		refusal := &gitmanager.PlanRejection{Reason: gitmanager.ReasonPlanStale, Detail: "hold"}
		if held := it.applyWith(&scriptedExecutor{preflight: refusal}, integrationAuthorizationID, &gitmanager.IssueRequest{AuthorizedBy: "operator"}); held.Reason != gitmanager.ReasonPlanStale {
			t.Fatalf("issuance %+v", held)
		}
		executor := &scriptedExecutor{outcome: gitmanager.OperationCompleted, evidence: gitmanager.ApplyEvidence{VerifiedComplete: true}, delay: time.Millisecond}
		results := make([]gitmanager.GitApplyResult, 2)
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := range results {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				results[i] = it.applyWith(executor, integrationAuthorizationID, nil)
			}(i)
		}
		close(start)
		wg.Wait()
		applied := 0
		for _, result := range results {
			switch {
			case result.Status == gitmanager.ApplyStatusApplied:
				applied++
			case result.Status == gitmanager.ApplyStatusRefused &&
				(result.Reason == gitmanager.ReasonLockConflict || result.Reason == gitmanager.ReasonAuthorizationConsumed):
			default:
				t.Fatalf("round %d: unexpected result %+v", round, result)
			}
		}
		final := it.durable(integrationAuthorizationID)
		if applied != 1 || executor.executed.Load() != int64(len(it.plan.DerivedOperations())) ||
			final.State != gitmanager.AuthorizationStateApplied || len(final.TransitionHistory()) != 3 {
			t.Fatalf("round %d: applied %d, executed %d, final %s with %d transitions", round, applied, executor.executed.Load(),
				final.State, len(final.TransitionHistory()))
		}
	}
}
