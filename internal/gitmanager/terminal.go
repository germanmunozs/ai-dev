package gitmanager

import "context"

// terminalClosure is what the apply flow established about the attempt's
// effects through its fresh controlled inspection.
type terminalClosure struct {
	// PrimaryReason is the causal reason, retained whatever the status.
	PrimaryReason Reason
	// VerifiedComplete: every planned postcondition was verified.
	VerifiedComplete bool
	// ZeroEffectsProven: a fresh controlled proof of zero effects across
	// the whole attempt, the only basis for status failed (F008-FR-189).
	ZeroEffectsProven bool
	// EffectsUnverifiable: some applied, absent or planned effect could not
	// be established.
	EffectsUnverifiable      bool
	VerifiedEffects          []Condition
	BeforeSnapshotDigest     ContentDigest
	AfterSnapshotDigest      ContentDigest
	CommitOID                ObjectOID
	IndexTreeOID             ObjectOID
	DevelopmentBranchCreated bool
	Warnings                 []string
	Findings                 []SecurityFinding
}

// operationSummary partitions the authorized operations by outcome.
type operationSummary struct {
	completed     []OperationID
	notStarted    []OperationID
	indeterminate []OperationID
	failed        OperationID
}

func (s *mutationSession) summary() operationSummary {
	var summary operationSummary
	for _, op := range s.authorization.AuthorizedOperationIDs() {
		outcome, seen := s.outcomes[op]
		switch {
		case !seen:
			summary.notStarted = append(summary.notStarted, op)
		case outcome == operationCompleted:
			summary.completed = append(summary.completed, op)
		case outcome == operationFailed && summary.failed == "":
			summary.failed = op
		default:
			summary.indeterminate = append(summary.indeterminate, op)
		}
	}
	return summary
}

// classifyEffects derives the status from all effects (F008-FR-155, 189):
// applied only when every planned operation completed and every
// postcondition was verified; failed only with a proof of zero effects;
// partially_applied when some effect is verified and the result is
// incomplete; state_indeterminate whenever an effect cannot be established,
// an audit record failed after mutation, or the evidence contradicts
// itself. The primary reason is retained.
func classifyEffects(c terminalClosure, operations operationSummary, auditFailed bool) (ApplyStatus, Reason) {
	successReason := c.PrimaryReason == "" || c.PrimaryReason == ReasonDevelopmentBranchPendingInitialHistory
	cause := ReasonOperationFailed
	if !successReason {
		cause = c.PrimaryReason
	}
	effects := len(operations.completed) > 0 || len(c.VerifiedEffects) > 0
	switch {
	case auditFailed:
		return ApplyStatusIndeterminate, ReasonAuditFailedAfterMutation
	case c.EffectsUnverifiable || len(operations.indeterminate) > 0:
		return ApplyStatusIndeterminate, cause
	case c.ZeroEffectsProven && effects:
		return ApplyStatusIndeterminate, cause
	case c.ZeroEffectsProven:
		return ApplyStatusFailed, cause
	case c.VerifiedComplete && operations.failed == "" && len(operations.notStarted) == 0:
		if successReason {
			return ApplyStatusApplied, c.PrimaryReason
		}
		return ApplyStatusIndeterminate, c.PrimaryReason
	case effects:
		return ApplyStatusPartiallyApplied, cause
	default:
		return ApplyStatusIndeterminate, cause
	}
}

// terminalState is the durable authorization state of a closed status.
func terminalState(status ApplyStatus) AuthorizationState {
	switch status {
	case ApplyStatusApplied, ApplyStatusAlreadyApplied:
		return AuthorizationStateApplied
	case ApplyStatusFailed:
		return AuthorizationStateFailed
	case ApplyStatusPartiallyApplied:
		return AuthorizationStatePartiallyApplied
	default:
		return AuthorizationStateIndeterminate
	}
}

// close is the sole terminal-closure owner (contract §7, plan step 9-13).
// For a session whose mutation never durably started it releases resources
// and records the stop outcome, releasing the claim only when no intent can
// be durable. Otherwise it classifies all effects, releases owned resources
// with the apply lock last, publishes one immutable prepared terminal
// result, and performs the owner-only CAS from mutation_started that binds
// the prepared record's digest. Only a verified CAS makes the result
// conclusive; any failure returns state_indeterminate/
// audit_failed_after_mutation with no retry. It never rolls back.
func (s *mutationSession) close(ctx context.Context, c terminalClosure) GitApplyResult {
	if s.closed {
		return s.result
	}
	if !s.started {
		if s.stop == nil {
			reason := c.PrimaryReason
			if reason == "" {
				reason = ReasonAuditUnavailable
			}
			return s.abandon(ctx, reason)
		}
		durable, consumed := s.authorization.State, s.intent || s.intentUnknown
		if !consumed {
			durable = s.releaseClaim(ctx)
		}
		report := releaseOwned(ctx, s.journal, s.ownedResources())
		in := GitApplyResultInput{
			Status: s.stop.status, Reason: s.stop.reason, DurableAuthorizationState: durable,
			RecoveryRequired: consumed, SecondaryFailures: report.Failures,
		}
		if in.Status == ApplyStatusIndeterminate {
			in.AuditStatus = auditStatusFailed
		}
		return s.finishResult(ctx, in)
	}

	operations := s.summary()
	status, reason := classifyEffects(c, operations, s.stop != nil)
	report := releaseOwned(ctx, s.journal, s.ownedResources())
	if report.AuditFailed {
		status, reason = ApplyStatusIndeterminate, ReasonAuditFailedAfterMutation
	}
	in := GitApplyResultInput{
		Status: status, Reason: reason, DurableAuthorizationState: terminalState(status),
		SecondaryFailures: report.Failures, BeforeSnapshotDigest: c.BeforeSnapshotDigest, AfterSnapshotDigest: c.AfterSnapshotDigest,
		CompletedOperationIDs: operations.completed, FailedOperationID: operations.failed, NotStartedOperationIDs: operations.notStarted,
		IndeterminateOperationIDs: operations.indeterminate, VerifiedEffects: c.VerifiedEffects, CommitOID: c.CommitOID,
		IndexTreeOID: c.IndexTreeOID, DevelopmentBranchCreated: c.DevelopmentBranchCreated, Warnings: c.Warnings, Findings: c.Findings,
		RecoveryRequired: status == ApplyStatusIndeterminate || status == ApplyStatusPartiallyApplied,
	}
	prepared := s.render(in)
	value, err := applyResultValue(prepared)
	if err != nil {
		return s.nonConclusive(ctx, in)
	}
	digest, err := s.journal.append(ctx, AuditEventPreparedTerminalResult, "", map[string]any{"result": value})
	if err != nil {
		return s.nonConclusive(ctx, in)
	}
	next, err := s.store.TransitionAuthorization(ctx, AuthorizationTransitionRequest{
		ProjectIdentityDigest: s.authorization.ProjectIdentity.Digest(), AuthorizationID: s.authorization.AuthorizationID,
		ExpectedState: AuthorizationStateMutationStarted, ExpectedVersion: s.authorization.StateVersion, ExpectedOwner: s.journal.applyID,
		To: prepared.DurableAuthorizationState, Owner: s.journal.applyID, TerminalResultDigest: digest,
	})
	if err != nil {
		return s.nonConclusive(ctx, in)
	}
	s.authorization = next
	// The verified CAS already made the prepared result conclusive and the
	// authorization record carries the bound digest; this record only
	// indexes it, so its failure cannot change the rendered result.
	_, _ = s.journal.append(ctx, AuditEventTerminalAuthorizationCAS, "", map[string]any{
		"terminal_result_digest": string(digest), "to_state": string(next.State), "state_version": canonicalInteger(next.StateVersion),
	})
	return s.finishResult(ctx, in)
}

// nonConclusive renders the outcome when the prepared result or its
// terminal CAS could not be verified: the authorization's last verified
// state is mutation_started and recovery is supervised.
func (s *mutationSession) nonConclusive(ctx context.Context, in GitApplyResultInput) GitApplyResult {
	in.Status, in.Reason = ApplyStatusIndeterminate, ReasonAuditFailedAfterMutation
	in.DurableAuthorizationState, in.RecoveryRequired, in.AuditStatus = AuthorizationStateMutationStarted, true, auditStatusFailed
	return s.finishResult(ctx, in)
}
