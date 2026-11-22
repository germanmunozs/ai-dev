package gitmanager

import (
	"context"
	"errors"
	"time"
)

// OperationExecutor runs the regenerated operations of an authorized plan.
// Revalidate is read-only and runs before each operation; Execute runs one
// operation only under its MutationPermit; Verify freshly inspects the
// planned postconditions. The production executor is the
// OperationDispatcher (T076).
type OperationExecutor interface {
	// Preflight is the initial-commit global preflight (F008-FR-191 Phase
	// 1), run before the claim and the first mutation; it is a no-op for a
	// plan without the group.
	Preflight(ctx context.Context, plan GitInitializationPlan) *PlanRejection
	Revalidate(ctx context.Context, plan GitInitializationPlan, operation DerivedOperation) *PlanRejection
	Execute(ctx context.Context, permit MutationPermit, plan GitInitializationPlan, operation DerivedOperation) OperationOutcome
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
	// Inspect freshly inspects the project at the path the plan declares,
	// observing the initial-commit candidates the intent names.
	Inspect  func(ctx context.Context, projectPath []byte, candidates [][]byte) (GitInspection, error)
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
		// INT-01: every refusal before the claim happens with no effect.
		// When the caller was interrupted, the interrupt is the cause: a
		// cancelled read or inspection must not read as drift.
		if ctx.Err() != nil && rejection.Reason != ReasonInterrupted {
			rejection = reject(ReasonInterrupted, "interrupted: %s: %s", rejection.Reason, rejection.Detail)
		}
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
	inspection, err := deps.Inspect(ctx, file.ProjectPath, intentCandidates(file.Intent))
	if err != nil {
		var inspectErr *InspectError
		if errors.As(err, &inspectErr) {
			return refuse(file.PlanID, nil, reject(ClassifyRepositoryReason(PhaseApplyDrift, inspectErr.Reason), "%s", inspectErr.Detail), false)
		}
		return refuse(file.PlanID, nil, reject(ReasonPlanStale, "the project could not be inspected"), false)
	}
	// An applied authorization of this plan is not regenerated: its effects
	// make the snapshot stale by construction. Its authentic
	// postconditions are verified freshly under the apply lock instead.
	kernel := safetyKernel{store: deps.Store, clock: clockOf(now)}
	verifyApplied := func(postconditions []Condition) func(context.Context) bool {
		return func(ctx context.Context) bool {
			// A read-only verification an interrupt must not turn into
			// authorization_consumed for an applied plan.
			ctx = context.WithoutCancel(ctx)
			fresh, err := deps.Inspect(ctx, file.ProjectPath, intentCandidates(file.Intent))
			if err != nil || fresh.ProjectIdentity.Digest() != inspection.ProjectIdentity.Digest() {
				return false
			}
			result, _ := evaluateConditions(postconditions, fresh)
			return len(postconditions) > 0 && result == conditionHolds
		}
	}
	if req.Issue == nil && AuthorizationID(req.AuthorizationID).Validate() == nil {
		prior, err := deps.Store.LoadAuthorization(ctx, inspection.ProjectIdentity.Digest(), AuthorizationID(req.AuthorizationID))
		if err == nil && prior.State == AuthorizationStateApplied && prior.PlanDigest == file.PlanDigest {
			_, result := kernel.claim(ctx, claimRequest{
				journal: journal, project: inspection.ProjectIdentity, authorization: prior.AuthorizationID, planDigest: file.PlanDigest,
				planID: file.PlanID, verifyApplied: verifyApplied(file.ExpectedPostconditions),
			})
			return *result
		}
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
		if issue.ReconcilesAuthorizationID != "" {
			in, rejection := newAuthorizationInput(issue, plan, now())
			if rejection != nil {
				return refuse(plan.PlanID, nil, rejection, false)
			}
			if rejection := reconcileAuthorization(ctx, deps.Store, journal, AuthorizationID(issue.ReconcilesAuthorizationID), in); rejection != nil {
				return refuse(plan.PlanID, nil, rejection, false)
			}
		} else {
			authorization, rejection := newAuthorization(issue, plan, now())
			if rejection != nil {
				return refuse(plan.PlanID, nil, rejection, false)
			}
			if rejection := issueAuthorization(ctx, deps.Store, journal, authorization); rejection != nil {
				return refuse(plan.PlanID, nil, rejection, false)
			}
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
	if rejection := deps.Executor.Preflight(ctx, plan); rejection != nil {
		return refuse(plan.PlanID, &authorization, rejection, false)
	}
	operations := plan.DerivedOperations()
	if len(operations) > 0 {
		if rejection := deps.Executor.Revalidate(ctx, plan, operations[0]); rejection != nil {
			return refuse(plan.PlanID, &authorization, reject(ClassifyRepositoryReason(PhaseApplyDrift, rejection.Reason), "%s", rejection.Detail), false)
		}
	}

	if ctx.Err() != nil {
		return refuse(plan.PlanID, &authorization, reject(ReasonInterrupted, "interrupted before the claim"), false)
	}
	// 9. Lock, reread and claim through the kernel.
	session, refusal := kernel.claim(ctx, claimRequest{
		journal: journal, project: project, authorization: authorization.AuthorizationID, planDigest: plan.PlanDigest,
		planID: plan.PlanID, proveNoEffects: deps.ProveNoEffects, verifyApplied: verifyApplied(plan.ExpectedPostconditions()),
	})
	if refusal != nil {
		return *refusal
	}

	// 10–12. Each operation only under its permit, stopping at the first
	// failure; the first operation was revalidated before the claim.
	//
	// INT-01: from the claim on, the work runs under a context the caller
	// cannot cancel, so an interrupt never kills Git in the middle of an
	// operation or skips verification, release and the terminal closure.
	// The interrupt is observed only between operations: before the first
	// one it is a clean refusal that leaves the authorization usable, and
	// after it no further operation starts and the effects are classified.
	work := context.WithoutCancel(ctx)
	var primary Reason
	for i, operation := range operations {
		if ctx.Err() != nil {
			primary = ReasonInterrupted
			break
		}
		if i > 0 {
			if rejection := deps.Executor.Revalidate(work, plan, operation); rejection != nil {
				primary = ClassifyRepositoryReason(PhaseRevalidated, rejection.Reason)
				break
			}
		}
		permit, err := session.begin(work, operation.OperationID)
		if err != nil {
			break
		}
		outcome := deps.Executor.Execute(work, permit, plan, operation)
		if err := session.finish(work, permit, outcome); err != nil {
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
		return session.abandon(work, primary)
	}

	// 13–15. Verify, release, prepare, terminal CAS, return.
	evidence := deps.Executor.Verify(work, plan)
	closure := terminalClosure{
		PrimaryReason: primary, VerifiedComplete: evidence.VerifiedComplete, ZeroEffectsProven: evidence.ZeroEffectsProven,
		EffectsUnverifiable: evidence.EffectsUnverifiable, VerifiedEffects: evidence.VerifiedEffects,
		AfterSnapshotDigest: evidence.AfterSnapshotDigest,
	}
	// The result names the commit, index tree and development branch only
	// as far as their effects were verified.
	verified := map[Condition]bool{}
	for _, effect := range evidence.VerifiedEffects {
		verified[effect] = true
	}
	for _, operation := range operations {
		for _, effect := range operation.ExpectedEffects() {
			if !verified[effect] {
				continue
			}
			switch {
			case operation.Kind == OperationKindInitialRefCAS:
				closure.CommitOID = ObjectOID(effect.Expected)
			case effect.Kind == ConditionIndexTree:
				closure.IndexTreeOID = ObjectOID(effect.Expected)
			case operation.Kind == OperationKindDevelopmentRefCAS:
				closure.DevelopmentBranchCreated = true
			}
		}
	}
	if primary == "" && evidence.VerifiedComplete && developmentPending(plan) {
		closure.PrimaryReason = ReasonDevelopmentBranchPendingInitialHistory
	}
	return session.close(work, closure)
}

// developmentPending reports a Git Flow plan whose development branch is
// deferred for lack of history (F008-FR-044–045).
func developmentPending(plan GitInitializationPlan) bool {
	intent := plan.ImmutableIntent
	if intent.Strategy != StrategyGitFlow || !intent.DevelopmentBase.IsZero() || intent.InitialCommit {
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
