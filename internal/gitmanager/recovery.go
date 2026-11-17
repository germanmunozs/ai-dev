package gitmanager

import (
	"context"
	"errors"
	"time"
)

// authorityAssessment is what a new attempt may do with an authorization it
// found under the apply lock.
type authorityAssessment struct {
	// usable: issued and unexpired; the attempt may claim it.
	usable bool
	// orphan: claimed by an attempt whose trail proves no mutation intent;
	// it may be reassigned only after a fresh proof of no effects.
	orphan bool
	// refusal is the reason the attempt must return when not usable.
	refusal Reason
	// unresolved: the prior outcome is not conclusive and needs supervised
	// reconciliation (F008-FR-168, F008-FR-180).
	unresolved bool
}

// assessAuthorization applies the consumption rules of F008-FR-168/179. A
// claimed authorization is consumed once its owner's trail holds a durable
// mutation_intent; a trail that cannot be read, or that carries crash
// evidence, cannot prove the intent absent and is treated as consumed.
// mutation_started and every terminal state are consumed; only
// mutation_started and state_indeterminate leave an unresolved outcome.
func assessAuthorization(ctx context.Context, store GitStore, authorization GitAuthorization, now time.Time) authorityAssessment {
	expired := !authorization.ExpiresAt.IsZero() && !now.Before(authorization.ExpiresAt)
	switch authorization.State {
	case AuthorizationStateIssued:
		if expired {
			return authorityAssessment{refusal: ReasonAuthorizationExpired}
		}
		return authorityAssessment{usable: true}
	case AuthorizationStateClaimed:
		trail, err := store.ListAuditEvents(ctx, authorization.OwnerApplyID)
		if err != nil || trail.CrashEvidence || trail.Contains(AuditEventMutationIntent) {
			return authorityAssessment{refusal: ReasonAuthorizationConsumed, unresolved: true}
		}
		if expired {
			return authorityAssessment{refusal: ReasonAuthorizationExpired}
		}
		// Without a proof of no effects the orphan is unusable by this
		// attempt; it is reported as consumed but needs no reconciliation.
		return authorityAssessment{orphan: true, refusal: ReasonAuthorizationConsumed}
	case AuthorizationStateMutationStarted, AuthorizationStateIndeterminate:
		return authorityAssessment{refusal: ReasonAuthorizationConsumed, unresolved: true}
	default:
		return authorityAssessment{refusal: ReasonAuthorizationConsumed}
	}
}

// startFailure classifies a failed claimed->mutation_started CAS made after
// a durable mutation_intent (F008-FR-179). An unverifiable outcome is
// state_indeterminate/audit_failed_after_mutation. Any other failure is
// proven not to have happened only when a fresh read still shows the
// expected claimed tuple: then the result is refused/audit_unavailable and
// the claim stays consumed. Anything else is indeterminate.
func startFailure(ctx context.Context, store GitStore, request AuthorizationTransitionRequest, err error) stopOutcome {
	indeterminate := stopOutcome{status: ApplyStatusIndeterminate, reason: ReasonAuditFailedAfterMutation}
	if errors.Is(err, ErrStoreOutcomeUnverifiable) {
		return indeterminate
	}
	current, readErr := store.LoadAuthorization(ctx, request.ProjectIdentityDigest, request.AuthorizationID)
	if readErr != nil || current.State != request.ExpectedState || current.StateVersion != request.ExpectedVersion ||
		current.OwnerApplyID != request.ExpectedOwner {
		return indeterminate
	}
	return stopOutcome{status: ApplyStatusRefused, reason: ReasonAuditUnavailable}
}

// stopOutcome is why a session may perform no further mutation.
type stopOutcome struct {
	status ApplyStatus
	reason Reason
}
