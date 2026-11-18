package gitmanager

import (
	"context"
	"errors"
	"time"
)

// OperationExecutor runs the regenerated operations of an authorized plan.
// Revalidate is read-only and runs before each operation; Execute runs one
// operation only under its MutationPermit; Verify freshly inspects the
// planned postconditions. The real executor is the operation dispatcher
// (T076); until it exists only test executors are supplied.
type OperationExecutor interface {
	Revalidate(ctx context.Context, operation DerivedOperation) *PlanRejection
	Execute(ctx context.Context, permit MutationPermit, operation DerivedOperation) OperationOutcome
	Verify(ctx context.Context, plan GitInitializationPlan) ApplyEvidence
}

// ApplyEvidence is what the fresh post-apply inspection established.
type ApplyEvidence struct {
	VerifiedComplete    bool
	ZeroEffectsProven   bool
	EffectsUnverifiable bool
	VerifiedEffects     []Condition
	AfterSnapshotDigest ContentDigest
}

// ApplyRequest is one apply invocation.
type ApplyRequest struct {
	PlanFile        []byte
	ConfirmPlan     string
	AuthorizationID string
	// Issue is set only with --issue-authorization.
	Issue *IssueRequest
}

// ApplyDependencies are the ports one apply uses.
type ApplyDependencies struct {
	Store       GitStore
	Clock       Clock
	Random      RandomSource
	Planner     PlannerDependencies
	Tool        GitToolIdentity
	Environment EnvironmentProfile
	// Inspect freshly inspects the project at the path the plan declares.
	Inspect  func(ctx context.Context, projectPath []byte) (GitInspection, error)
	Executor OperationExecutor
	// ProveNoEffects, when set, proves an orphaned claim produced nothing.
	ProveNoEffects func(context.Context) error
}

// Apply runs the definitive apply sequence (spec, Definitive Apply
// Sequence) as a consumer of the safety kernel and the sole terminal
// closer: it records the attempt before any business result, extracts only
// the intent from the untrusted plan, regenerates and binds the
// authorization, claims it under the apply lock, runs each operation only
// under its permit, and closes through terminal.go. It never rolls back.
func Apply(ctx context.Context, deps ApplyDependencies, req ApplyRequest) GitApplyResult {
	now := time.Now
	if deps.Clock != nil {
		now = deps.Clock.Now
	}
	id, err := newUUIDv4(deps.Random)
	applyID := ApplyID(id)
	if err != nil || deps.Store == nil || deps.Executor == nil || deps.Inspect == nil {
		return unrecordedRefusal(applyID, "apply is not available")
	}
	// 1–2. Minimal input bounds, then the durable attempt record.
	if err := CheckLimit(LimitPlanBytes, int64(len(req.PlanFile))); err != nil {
		return unrecordedRefusal(applyID, "the plan exceeds its bound")
	}
	attempt := GitApplyAttempt{SchemaVersion: GitApplyAttemptSchema, ApplyID: applyID, StartedAt: now().UTC().Truncate(time.Second),
		InvocationMode: deps.Tool.InvocationMode}
	if AuthorizationID(req.AuthorizationID).Validate() == nil {
		attempt.AuthorizationID = AuthorizationID(req.AuthorizationID)
	}
	if PlanDigest(req.ConfirmPlan).Validate() == nil {
		attempt.DeclaredPlanDigest = PlanDigest(req.ConfirmPlan)
	}
	journal, err := openAttemptJournal(ctx, deps.Store, attempt)
	if err != nil {
		return unrecordedRefusal(applyID, "the attempt record could not be made durable")
	}
	refuse := func(planID PlanID, authorization *GitAuthorization, rejection *PlanRejection, recovery bool) GitApplyResult {
		return recordRefusal(ctx, journal, planID, authorization, rejection, recovery)
	}

	// 3, 7. Extract only the intent, inspect freshly and regenerate.
	file, err := DecodePlanFile(req.PlanFile)
	if err != nil {
		reason := ReasonPlanInvalid
		var limit *LimitError
		if errors.As(err, &limit) {
			reason = limit.Reason
		}
		return refuse("", nil, reject(reason, "%v", err), false)
	}
	if req.ConfirmPlan != string(file.PlanDigest) {
		return refuse(file.PlanID, nil, reject(ReasonAuthorizationMismatch, "--confirm-plan does not name this plan"), false)
	}
	inspection, err := deps.Inspect(ctx, file.ProjectPath)
	if err != nil {
		var inspectErr *InspectError
		if errors.As(err, &inspectErr) {
			return refuse(file.PlanID, nil, reject(ClassifyRepositoryReason(PhaseApplyDrift, inspectErr.Reason), "%s", inspectErr.Detail), false)
		}
		return refuse(file.PlanID, nil, reject(ReasonPlanStale, "the project could not be inspected"), false)
	}
	plan, rejection := RegenerateAuthorized(ctx, deps.Planner, file.Intent, file.PlanDigest, inspection, deps.Environment,
		PlanMetadata{PlanID: file.PlanID, GeneratedAt: now().UTC().Truncate(time.Second)})
	if rejection != nil {
		return refuse(file.PlanID, nil, rejection, false)
	}
	project := plan.PreconditionSnapshot.ProjectIdentity
	journal.bind(project.Digest(), AuthorizationID(req.AuthorizationID))

	// 4. Exclusive issuance when explicitly requested.
	if req.Issue != nil {
		issue := *req.Issue
		issue.AuthorizationID = req.AuthorizationID
		authorization, rejection := newAuthorization(issue, plan, now())
		if rejection != nil {
			return refuse(plan.PlanID, nil, rejection, false)
		}
		if rejection := issueAuthorization(ctx, deps.Store, journal, authorization); rejection != nil {
			return refuse(plan.PlanID, nil, rejection, false)
		}
	} else if AuthorizationID(req.AuthorizationID).Validate() != nil {
		return refuse(plan.PlanID, nil, reject(ReasonAuthorizationMissing, "no valid --authorization-id"), false)
	}

	// 5–6. Bind the authorization to plan, project, scope and approvals.
	authorization, assessment, rejection := loadAuthorization(ctx, deps.Store, project.Digest(), AuthorizationID(req.AuthorizationID), now)
	if rejection != nil {
		return refuse(plan.PlanID, nil, rejection, false)
	}
	if !assessment.usable && !assessment.orphan {
		return refuse(plan.PlanID, &authorization, reject(assessment.refusal, "the authorization is %s", authorization.State), assessment.unresolved)
	}
	if rejection := checkScope(authorization, plan); rejection != nil {
		return refuse(plan.PlanID, &authorization, rejection, false)
	}
	operations := plan.DerivedOperations()
	if len(operations) > 0 {
		if rejection := deps.Executor.Revalidate(ctx, operations[0]); rejection != nil {
			return refuse(plan.PlanID, &authorization, reject(ClassifyRepositoryReason(PhaseApplyDrift, rejection.Reason), "%s", rejection.Detail), false)
		}
	}

	// 9. Lock, reread and claim through the kernel.
	kernel := safetyKernel{store: deps.Store, clock: clockOf(now)}
	session, refusal := kernel.claim(ctx, claimRequest{
		journal: journal, project: project, authorization: authorization.AuthorizationID, planDigest: plan.PlanDigest,
		planID: plan.PlanID, proveNoEffects: deps.ProveNoEffects,
	})
	if refusal != nil {
		return *refusal
	}

	// 10–12. Each operation only under its permit, stopping at the first
	// failure; the first operation was revalidated before the claim.
	var primary Reason
	for i, operation := range operations {
		if i > 0 {
			if rejection := deps.Executor.Revalidate(ctx, operation); rejection != nil {
				primary = ClassifyRepositoryReason(PhaseRevalidated, rejection.Reason)
				break
			}
		}
		permit, err := session.begin(ctx, operation.OperationID)
		if err != nil {
			break
		}
		outcome := deps.Executor.Execute(ctx, permit, operation)
		if err := session.finish(ctx, permit, outcome); err != nil {
			break
		}
		if outcome != OperationCompleted {
			primary = ReasonOperationFailed
			break
		}
	}
	if !session.started && session.stop == nil {
		// Nothing started: a clean refusal that leaves the authorization
		// usable.
		if primary == "" {
			primary = ReasonConcurrentChange
		}
		return session.abandon(ctx, primary)
	}

	// 13–15. Verify, release, prepare, terminal CAS, return.
	evidence := deps.Executor.Verify(ctx, plan)
	closure := terminalClosure{
		PrimaryReason: primary, VerifiedComplete: evidence.VerifiedComplete, ZeroEffectsProven: evidence.ZeroEffectsProven,
		EffectsUnverifiable: evidence.EffectsUnverifiable, VerifiedEffects: evidence.VerifiedEffects,
		AfterSnapshotDigest: evidence.AfterSnapshotDigest,
	}
	if primary == "" && evidence.VerifiedComplete && developmentPending(plan) {
		closure.PrimaryReason = ReasonDevelopmentBranchPendingInitialHistory
	}
	return session.close(ctx, closure)
}

// developmentPending reports a Git Flow plan whose development branch is
// deferred for lack of history (F008-FR-044–045).
func developmentPending(plan GitInitializationPlan) bool {
	intent := plan.ImmutableIntent
	if intent.Strategy != StrategyGitFlow || !intent.DevelopmentBase.IsZero() {
		return false
	}
	development := "refs/heads/" + intent.BranchParameters()[BranchDevelopment]
	for _, ref := range plan.PreconditionSnapshot.Observation.Refs() {
		if string(ref.Name.Bytes()) == development {
			return false
		}
	}
	return true
}

type clockFunc func() time.Time

func (f clockFunc) Now() time.Time { return f() }

func clockOf(now func() time.Time) Clock { return clockFunc(now) }

// unrecordedRefusal is the only result returned without an attempt record:
// the attempt itself could not be made durable, so nothing ran.
func unrecordedRefusal(applyID ApplyID, detail string) GitApplyResult {
	if applyID.Validate() != nil {
		applyID = "00000000-0000-4000-8000-000000000000"
	}
	result, _ := NewGitApplyResult(GitApplyResultInput{
		SchemaVersion: GitApplyResultSchema, ApplyID: applyID, Status: ApplyStatusRefused, Reason: ReasonAuditUnavailable,
		ExitCode: 1, AuditStatus: auditStatusUnavailable,
	})
	_ = detail
	return result
}
