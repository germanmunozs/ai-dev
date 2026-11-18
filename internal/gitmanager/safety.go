package gitmanager

import (
	"context"
	"errors"
	"fmt"
)

var (
	// errMutationNotPermitted reports a permit that does not authorize the
	// requested operation now.
	errMutationNotPermitted = errors.New("mutation not permitted")
	// errSessionStopped reports a session that may perform no further
	// mutation; it must be closed.
	errSessionStopped = errors.New("mutation session stopped")
)

// MutationPermit is the only proof a mutating operation accepts. The safety
// kernel emits it after the apply lock is held, the authorization is
// claimed, mutation_intent is durable, the claimed->mutation_started CAS is
// durable and the operation's operation_started record is durable, in that
// order. It authorizes exactly one open operation of one authorization and
// lapses when that operation finishes or its session stops. Its fields are
// unexported, so no other package can build one, and the zero value
// authorizes nothing.
type MutationPermit struct {
	session         *mutationSession
	authorizationID AuthorizationID
	operationID     OperationID
}

// authorizes reports whether the permit allows operation of authorization
// to run now.
func (p MutationPermit) authorizes(authorization AuthorizationID, operation OperationID) error {
	s := p.session
	if s == nil || s.closed || s.stop != nil || !s.started || s.open == "" || s.open != p.operationID ||
		p.operationID != operation || p.authorizationID != authorization || s.authorization.AuthorizationID != authorization {
		return errMutationNotPermitted
	}
	return nil
}

// authorizesPlanned reports whether the permit allows operation of plan to
// run now: it is the open operation of a live session whose authorization
// was issued for this plan and lists this operation.
func (p MutationPermit) authorizesPlanned(plan GitInitializationPlan, operation DerivedOperation) error {
	if p.session == nil {
		return errMutationNotPermitted
	}
	authorization := p.session.authorization
	if err := p.authorizes(authorization.AuthorizationID, operation.OperationID); err != nil {
		return err
	}
	if authorization.PlanDigest != plan.PlanDigest || !containsValue(authorization.AuthorizedOperationIDs(), operation.OperationID) {
		return errMutationNotPermitted
	}
	return nil
}

// own registers a resource the permitted operation created and owns, for
// the planned, audited release at closure (F008-FR-169, 184).
func (p MutationPermit) own(operation DerivedOperation, role ResourceRole, release func(context.Context) error) error {
	if p.session == nil || p.authorizes(p.authorizationID, operation.OperationID) != nil {
		return errMutationNotPermitted
	}
	return p.session.addResource(ownedRelease{Role: role, Operation: operation.OperationID, Ordinal: operation.Ordinal, Release: release})
}

// safetyKernel is the durable safety kernel every Git mutation goes through
// (T024-T035). Later mutation code consumes it and never reimplements it.
type safetyKernel struct {
	store GitStore
	clock Clock
}

// claimRequest is one attempt's request to claim an authorization. The
// project identity was freshly verified, and the plan digest is the one
// the attempt validated before taking the lock.
type claimRequest struct {
	journal       *attemptJournal
	project       PhysicalProjectIdentity
	authorization AuthorizationID
	planDigest    PlanDigest
	planID        PlanID
	// proveNoEffects is a fresh read-only inspection proving that an
	// orphaned claim produced no effect. Without it an orphan is not
	// reassigned.
	proveNoEffects func(context.Context) error
	// verifyApplied freshly verifies the plan's postconditions. An applied
	// authorization whose postconditions still hold is already_applied
	// (F008-FR-127); without it, or when they do not hold, it is consumed.
	verifyApplied func(context.Context) bool
}

type operationOutcome string

// OperationOutcome is how an executed operation ended, as the operation
// executor reports it.
type OperationOutcome = operationOutcome

// Exported operation outcomes.
const (
	OperationCompleted     = operationCompleted
	OperationFailed        = operationFailed
	OperationIndeterminate = operationIndeterminate
)

const (
	operationCompleted     operationOutcome = "completed"
	operationFailed        operationOutcome = "failed"
	operationIndeterminate operationOutcome = "indeterminate"
	// operationOpen marks an operation that started and has no result.
	operationOpen operationOutcome = ""
)

// mutationSession is one attempt's hold on the apply lock and, once
// claimed, on one authorization. It is used by a single goroutine.
type mutationSession struct {
	store         GitStore
	journal       *attemptJournal
	lock          ApplyLock
	planID        PlanID
	authorization GitAuthorization // last verified durable record
	resolved      bool             // authorization was read under the lock
	prior         AuthorizationState
	claimed       bool // this attempt durably owns the claim
	intent        bool // mutation_intent is durable
	intentUnknown bool // mutation_intent may be durable
	started       bool // claimed->mutation_started is durable and verified
	stop          *stopOutcome
	open          OperationID
	outcomes      map[OperationID]operationOutcome
	resources     []ownedRelease
	closed        bool
	result        GitApplyResult
}

// claim takes the apply lock, rereads the authorization under it and
// claims it with a CAS: issued->claimed, or claimed->claimed for an orphan
// proven to have no intent and no effect. Every refusal releases the lock
// and is recorded before it is returned.
func (k safetyKernel) claim(ctx context.Context, req claimRequest) (*mutationSession, *GitApplyResult) {
	journal := req.journal
	journal.bind(req.project.Digest(), req.authorization)
	s := &mutationSession{store: k.store, journal: journal, planID: req.planID, outcomes: make(map[OperationID]operationOutcome)}
	refuse := func(reason Reason, recovery bool) (*mutationSession, *GitApplyResult) {
		result := s.refuse(ctx, reason, s.authorization.State, recovery)
		return nil, &result
	}
	lock, err := k.store.AcquireApplyLock(ctx, req.project, journal.applyID)
	if err != nil {
		return refuse(ReasonLockConflict, false)
	}
	s.lock = lock
	authorization, err := k.store.LoadAuthorization(ctx, req.project.Digest(), req.authorization)
	if err != nil {
		if errors.Is(err, ErrAuthorizationMissing) {
			return refuse(ReasonAuthorizationMissing, false)
		}
		return refuse(ReasonAuditUnavailable, false)
	}
	s.authorization, s.resolved, s.prior = authorization, true, authorization.State
	if req.planDigest != "" && authorization.PlanDigest != req.planDigest {
		return refuse(ReasonAuthorizationMismatch, false)
	}
	if authorization.State == AuthorizationStateApplied && req.verifyApplied != nil && req.verifyApplied(ctx) {
		result := s.alreadyApplied(ctx)
		return nil, &result
	}
	request := AuthorizationTransitionRequest{
		ProjectIdentityDigest: req.project.Digest(), AuthorizationID: req.authorization, ExpectedState: authorization.State,
		ExpectedVersion: authorization.StateVersion, ExpectedOwner: authorization.OwnerApplyID,
		To: AuthorizationStateClaimed, Owner: journal.applyID,
	}
	switch assessment := assessAuthorization(ctx, k.store, authorization, k.clock.Now()); {
	case assessment.usable:
	case assessment.orphan && req.proveNoEffects != nil:
		if req.proveNoEffects(ctx) != nil {
			return refuse(assessment.refusal, false)
		}
	default:
		return refuse(assessment.refusal, assessment.unresolved)
	}
	next, err := k.store.TransitionAuthorization(ctx, request)
	if err != nil {
		// An unverifiable claim is reread: a claim proven ours is released
		// cleanly, since no intent exists yet; otherwise the refusal
		// reports the state actually read.
		if current, readErr := k.store.LoadAuthorization(ctx, req.project.Digest(), req.authorization); readErr == nil {
			if current.State == AuthorizationStateClaimed && current.OwnerApplyID == journal.applyID &&
				current.StateVersion == authorization.StateVersion+1 {
				s.authorization, s.claimed = current, true
				result := s.abandon(ctx, ReasonAuditUnavailable)
				return nil, &result
			}
			s.authorization = current
		}
		return refuse(ReasonAuditUnavailable, false)
	}
	s.authorization, s.claimed = next, true
	if _, err := journal.append(ctx, AuditEventAuthorizationTransition, "", transitionDetails(authorization, next)); err != nil {
		result := s.abandon(ctx, ReasonAuditUnavailable)
		return nil, &result
	}
	return s, nil
}

func transitionDetails(from, to GitAuthorization) map[string]any {
	return map[string]any{
		"from_state": string(from.State), "to_state": string(to.State), "state_version": canonicalInteger(to.StateVersion),
	}
}

// begin returns the permit for one authorized operation. Before the first
// operation it makes mutation_intent and the claimed->mutation_started CAS
// durable; before every operation it makes operation_started durable. Any
// failure stops the session: no permit, no further operation.
func (s *mutationSession) begin(ctx context.Context, op OperationID) (MutationPermit, error) {
	if s.closed || s.stop != nil || !s.claimed {
		return MutationPermit{}, errSessionStopped
	}
	if s.open != "" || !containsValue(s.authorization.AuthorizedOperationIDs(), op) {
		return MutationPermit{}, fmt.Errorf("%w: operation %s", errMutationNotPermitted, op)
	}
	if _, seen := s.outcomes[op]; seen {
		return MutationPermit{}, fmt.Errorf("%w: operation %s already ran", errMutationNotPermitted, op)
	}
	if !s.intent {
		if err := s.startMutation(ctx); err != nil {
			return MutationPermit{}, err
		}
	}
	if _, err := s.journal.append(ctx, AuditEventOperationStarted, op, nil); err != nil {
		s.stop = &stopOutcome{status: ApplyStatusIndeterminate, reason: ReasonAuditFailedAfterMutation}
		return MutationPermit{}, fmt.Errorf("%w: operation_started: %v", errSessionStopped, err)
	}
	s.outcomes[op], s.open = operationOpen, op
	return MutationPermit{session: s, authorizationID: s.authorization.AuthorizationID, operationID: op}, nil
}

// startMutation publishes mutation_intent and then performs the
// claimed->mutation_started CAS (F008-FR-062, F008-FR-179).
func (s *mutationSession) startMutation(ctx context.Context) error {
	details := map[string]any{
		"plan_digest":              string(s.authorization.PlanDigest),
		"authorized_operation_ids": canonicalStrings(s.authorization.AuthorizedOperationIDs()),
	}
	if _, err := s.journal.append(ctx, AuditEventMutationIntent, "", details); err != nil {
		s.intentUnknown = errors.Is(err, ErrStoreOutcomeUnverifiable)
		s.stop = &stopOutcome{status: ApplyStatusRefused, reason: ReasonAuditUnavailable}
		return fmt.Errorf("%w: mutation_intent: %v", errSessionStopped, err)
	}
	s.intent = true
	request := AuthorizationTransitionRequest{
		ProjectIdentityDigest: s.authorization.ProjectIdentity.Digest(), AuthorizationID: s.authorization.AuthorizationID,
		ExpectedState: AuthorizationStateClaimed, ExpectedVersion: s.authorization.StateVersion, ExpectedOwner: s.journal.applyID,
		To: AuthorizationStateMutationStarted, Owner: s.journal.applyID,
	}
	next, err := s.store.TransitionAuthorization(ctx, request)
	if err != nil {
		outcome := startFailure(ctx, s.store, request, err)
		s.stop = &outcome
		return fmt.Errorf("%w: mutation start: %v", errSessionStopped, err)
	}
	previous := s.authorization
	s.authorization, s.started = next, true
	if _, err := s.journal.append(ctx, AuditEventAuthorizationTransition, "", transitionDetails(previous, next)); err != nil {
		s.stop = &stopOutcome{status: ApplyStatusIndeterminate, reason: ReasonAuditFailedAfterMutation}
		return fmt.Errorf("%w: mutation start record: %v", errSessionStopped, err)
	}
	return nil
}

// finish records the result of the permitted operation and closes the
// permit. A result that cannot be recorded stops the session.
func (s *mutationSession) finish(ctx context.Context, permit MutationPermit, outcome operationOutcome) error {
	if permit.session != s || permit.authorizes(s.authorization.AuthorizationID, permit.operationID) != nil {
		return errMutationNotPermitted
	}
	kinds := map[operationOutcome]AuditEventKind{
		operationCompleted: AuditEventOperationCompleted, operationFailed: AuditEventOperationFailed,
		operationIndeterminate: AuditEventOperationIndeterminate,
	}
	kind, ok := kinds[outcome]
	if !ok {
		return fmt.Errorf("%w: operation outcome %q", errInvalidModel, outcome)
	}
	op := permit.operationID
	s.outcomes[op], s.open = outcome, ""
	if _, err := s.journal.append(ctx, kind, op, map[string]any{"outcome": string(outcome)}); err != nil {
		s.stop = &stopOutcome{status: ApplyStatusIndeterminate, reason: ReasonAuditFailedAfterMutation}
		return fmt.Errorf("%w: operation result: %v", errSessionStopped, err)
	}
	return nil
}

// addResource registers a resource this attempt created and owns, for the
// planned release at closure. The apply lock is registered by the kernel.
func (s *mutationSession) addResource(resource ownedRelease) error {
	if s.closed || resource.Role == ResourceRoleApplyLock || !resource.Role.Valid() || resource.Release == nil {
		return fmt.Errorf("%w: owned resource %q", errInvalidModel, resource.Role)
	}
	s.resources = append(s.resources, resource)
	return nil
}

// abandon refuses before any mutation intent: the claim is released to
// issued by the same owner, owned resources and the lock are released, and
// the refusal is recorded. After intent it is a closure instead.
func (s *mutationSession) abandon(ctx context.Context, reason Reason) GitApplyResult {
	if s.closed {
		return s.result
	}
	if s.intent || s.intentUnknown || s.started {
		return s.close(ctx, terminalClosure{PrimaryReason: reason})
	}
	return s.refuse(ctx, reason, s.releaseClaim(ctx), false)
}

// releaseClaim performs the clean claimed->issued release that is allowed
// only while no mutation intent can be durable, and returns the durable
// state it leaves.
func (s *mutationSession) releaseClaim(ctx context.Context) AuthorizationState {
	if !s.claimed {
		return s.authorization.State
	}
	request := AuthorizationTransitionRequest{
		ProjectIdentityDigest: s.authorization.ProjectIdentity.Digest(), AuthorizationID: s.authorization.AuthorizationID,
		ExpectedState: AuthorizationStateClaimed, ExpectedVersion: s.authorization.StateVersion, ExpectedOwner: s.journal.applyID,
		To: AuthorizationStateIssued, Owner: s.journal.applyID,
	}
	next, err := s.store.TransitionAuthorization(ctx, request)
	if err != nil {
		return AuthorizationStateClaimed
	}
	previous := s.authorization
	s.authorization, s.claimed = next, false
	_, _ = s.journal.append(ctx, AuditEventAuthorizationTransition, "", transitionDetails(previous, next))
	return next.State
}

// refuse releases every owned resource, the lock last, and records the
// refusal with the given last verified durable state.
func (s *mutationSession) refuse(ctx context.Context, reason Reason, durable AuthorizationState, recovery bool) GitApplyResult {
	report := releaseOwned(ctx, s.journal, s.ownedResources())
	return s.finishResult(ctx, GitApplyResultInput{
		Status: ApplyStatusRefused, Reason: reason, DurableAuthorizationState: durable, RecoveryRequired: recovery,
		SecondaryFailures: report.Failures,
	})
}

// alreadyApplied closes an attempt that found its applied authorization's
// postconditions still holding: it releases the lock, transitions nothing,
// mutates nothing and records already_applied.
func (s *mutationSession) alreadyApplied(ctx context.Context) GitApplyResult {
	report := releaseOwned(ctx, s.journal, s.ownedResources())
	return s.finishResult(ctx, GitApplyResultInput{
		Status: ApplyStatusAlreadyApplied, DurableAuthorizationState: AuthorizationStateApplied, SecondaryFailures: report.Failures,
	})
}

// ownedResources lists the registered resources and the apply lock.
func (s *mutationSession) ownedResources() []ownedRelease {
	resources := append([]ownedRelease(nil), s.resources...)
	if s.lock != nil {
		resources = append(resources, ownedRelease{Role: ResourceRoleApplyLock, Release: s.lock.Release})
	}
	return resources
}

// render completes and validates a result of this attempt. An input the
// model rejects is rendered conservatively instead of being returned
// invalid.
func (s *mutationSession) render(in GitApplyResultInput) GitApplyResult {
	in.SchemaVersion, in.ApplyID, in.PlanID = GitApplyResultSchema, s.journal.applyID, s.planID
	if s.resolved {
		in.AuthorizationID, in.PlanDigest, in.PriorAuthorizationState = s.authorization.AuthorizationID, s.authorization.PlanDigest, s.prior
	} else {
		in.DurableAuthorizationState = ""
	}
	if in.AuditStatus == "" {
		in.AuditStatus = auditStatusRecorded
	}
	SortSecondaryFailures(in.SecondaryFailures)
	if in.Reason == "" && len(in.SecondaryFailures) > 0 {
		in.Reason = ReasonResourceReleaseFailed
	}
	in.ExitCode = exitCodeFor(in.Status, in.Reason, len(in.SecondaryFailures) > 0)
	if result, err := NewGitApplyResult(in); err == nil {
		return result
	}
	in.Reason = ReasonOperationFailed
	if in.Status == ApplyStatusRefused {
		in.Reason = ReasonAuditUnavailable
	}
	in.ExitCode = exitCodeFor(in.Status, in.Reason, len(in.SecondaryFailures) > 0)
	if result, err := NewGitApplyResult(in); err == nil {
		return result
	}
	// Once a mutation may have happened, the last resort must not claim that
	// nothing changed: it is indeterminate and needs reconciliation.
	if s.resolved && (s.intent || s.intentUnknown || s.started) {
		durable := s.authorization.State
		if !resultStateCompatible(ApplyStatusIndeterminate, durable) {
			durable = AuthorizationStateMutationStarted
		}
		if result, err := NewGitApplyResult(GitApplyResultInput{
			SchemaVersion: GitApplyResultSchema, ApplyID: s.journal.applyID, PlanID: s.planID,
			AuthorizationID: s.authorization.AuthorizationID, PlanDigest: s.authorization.PlanDigest, PriorAuthorizationState: s.prior,
			Status: ApplyStatusIndeterminate, Reason: ReasonOperationFailed, DurableAuthorizationState: durable,
			ExitCode: 1, AuditStatus: in.AuditStatus, RecoveryRequired: true,
		}); err == nil {
			return result
		}
	}
	result, _ := NewGitApplyResult(GitApplyResultInput{
		SchemaVersion: GitApplyResultSchema, ApplyID: s.journal.applyID, Status: ApplyStatusRefused,
		Reason: ReasonAuditUnavailable, ExitCode: 1, AuditStatus: auditStatusUnavailable,
	})
	return result
}

// finishResult renders, records and closes the session with its result. A
// refusal that cannot be recorded becomes audit_unavailable, because a
// refusal must be audited before it is returned.
func (s *mutationSession) finishResult(ctx context.Context, in GitApplyResultInput) GitApplyResult {
	result := s.render(in)
	if _, err := s.journal.recordResult(ctx, result); err != nil && result.Status == ApplyStatusRefused {
		in.Reason, in.AuditStatus = ReasonAuditUnavailable, auditStatusUnavailable
		result = s.render(in)
	}
	s.closed, s.result, s.open = true, result, ""
	return result
}

// Audit status labels of a result.
const (
	auditStatusRecorded    = "recorded"
	auditStatusUnavailable = "unavailable"
	auditStatusFailed      = "audit_failed"
)

// exitCodeFor applies the exit rules of validateResultExit.
func exitCodeFor(status ApplyStatus, reason Reason, secondary bool) int {
	switch {
	case secondary:
		return 1
	case reason == ReasonOutputFailed:
		return 2
	case status == ApplyStatusApplied || status == ApplyStatusAlreadyApplied:
		return 0
	case reason == ReasonInteractiveTerminalRequired || reason == ReasonInvalidStrategy || reason == ReasonMissingRequiredBranch ||
		reason == ReasonOutputExists || reason == ReasonUnsafeOutputPath:
		return 2
	default:
		return 1
	}
}
