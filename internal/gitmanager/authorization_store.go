package gitmanager

import (
	"context"
	"errors"
)

// issueAuthorization makes a new issued authorization durable through the
// sole T029 store API and records its issuance through the attempt's
// journal (T065). It never overwrites, merges or widens an existing ID.
func issueAuthorization(ctx context.Context, store GitStore, journal *attemptJournal, authorization GitAuthorization) *PlanRejection {
	if err := store.IssueAuthorization(ctx, authorization); err != nil {
		if errors.Is(err, ErrAuthorizationExists) {
			return reject(ReasonAuthorizationIDExists, "authorization %s already exists", authorization.AuthorizationID)
		}
		return reject(ReasonAuditUnavailable, "the authorization could not be made durable")
	}
	journal.bind(authorization.ProjectIdentity.Digest(), authorization.AuthorizationID)
	if _, err := journal.append(ctx, AuditEventAuthorizationIssued, "", map[string]any{
		"plan_digest": string(authorization.PlanDigest), "declared_operator_present": true,
	}); err != nil {
		return reject(ReasonAuditUnavailable, "the issuance could not be audited")
	}
	return nil
}

// loadAuthorization rereads an authorization through the store API and
// classifies what a new attempt may do with it, without transitioning it.
func loadAuthorization(ctx context.Context, store GitStore, project ProjectIdentityDigest, id AuthorizationID, now TimeSource) (GitAuthorization, authorityAssessment, *PlanRejection) {
	authorization, err := store.LoadAuthorization(ctx, project, id)
	switch {
	case errors.Is(err, ErrAuthorizationMissing):
		return GitAuthorization{}, authorityAssessment{}, reject(ReasonAuthorizationMissing, "authorization %s does not exist for this project", id)
	case err != nil:
		return GitAuthorization{}, authorityAssessment{}, reject(ReasonAuditUnavailable, "the authorization cannot be read")
	}
	return authorization, assessAuthorization(ctx, store, authorization, now()), nil
}
