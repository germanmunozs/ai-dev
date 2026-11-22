package gitmanager

import "context"

// refusalSession is a session that never held the lock or a claim. It
// renders and records a pre-claim refusal through the same journal and
// result path as every other outcome (T066), so there is one writer.
func refusalSession(journal *attemptJournal, planID PlanID, authorization *GitAuthorization) *mutationSession {
	s := &mutationSession{journal: journal, planID: planID, outcomes: map[OperationID]operationOutcome{}}
	if authorization != nil {
		s.authorization, s.resolved, s.prior = *authorization, true, authorization.State
	}
	return s
}

// recordRefusal records a refusal decided before the claim and returns
// the rendered result. A consumed prior outcome that remains unresolved
// requires recovery.
func recordRefusal(ctx context.Context, journal *attemptJournal, planID PlanID, authorization *GitAuthorization, rejection *PlanRejection, recovery bool) GitApplyResult {
	s := refusalSession(journal, planID, authorization)
	durable := AuthorizationState("")
	if authorization != nil {
		durable = authorization.State
	}
	return s.refuse(ctx, rejection.Reason, durable, recovery)
}
