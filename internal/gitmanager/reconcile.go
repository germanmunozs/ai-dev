package gitmanager

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"
)

// errNotReconcilable reports a prior authorization whose outcome is not an
// unresolved consumption.
var errNotReconcilable = errors.New("authorization is not reconcilable")

// issueReconciliation is the only recovery path for an unresolved
// authorization (F008-FR-180, contract §8). After the caller's fresh
// inspection and new canonical plan, it issues a new supervised
// authorization in the same project partition, linked to the prior
// authorization, the apply attempt that consumed it and its plan digest.
// The prior authorization is never transitioned, reactivated or rewritten:
// it is only read, and is proven unchanged after issuance.
func issueReconciliation(ctx context.Context, store GitStore, journal *attemptJournal, priorID AuthorizationID, in GitAuthorizationInput) (GitAuthorization, error) {
	project := in.ProjectIdentity.Digest()
	prior, err := store.LoadAuthorization(ctx, project, priorID)
	if err != nil {
		return GitAuthorization{}, err
	}
	before, err := EncodeAuthorizationRecord(prior)
	if err != nil {
		return GitAuthorization{}, err
	}
	// Expiry does not limit reconciliation, so the assessment uses the
	// zero time.
	if !assessAuthorization(ctx, store, prior, time.Time{}).unresolved {
		return GitAuthorization{}, fmt.Errorf("%w: %s is %s", errNotReconcilable, priorID, prior.State)
	}
	in.PreviousAuthorizationID, in.PreviousApplyID, in.PreviousPlanDigest = prior.AuthorizationID, prior.OwnerApplyID, prior.PlanDigest
	authorization, err := NewGitAuthorization(in)
	if err != nil {
		return GitAuthorization{}, err
	}
	journal.bind(project, authorization.AuthorizationID)
	if err := store.IssueAuthorization(ctx, authorization); err != nil {
		return GitAuthorization{}, err
	}
	if _, err := journal.append(ctx, AuditEventAuthorizationIssued, "", map[string]any{
		"previous_authorization_id": string(prior.AuthorizationID), "previous_apply_id": string(prior.OwnerApplyID),
		"previous_plan_digest": string(prior.PlanDigest), "reason": string(ApplyStatusIndeterminate),
	}); err != nil {
		return authorization, err
	}
	after, err := store.LoadAuthorization(ctx, project, priorID)
	if err != nil {
		return authorization, err
	}
	if encoded, err := EncodeAuthorizationRecord(after); err != nil || !bytes.Equal(encoded, before) {
		return authorization, fmt.Errorf("prior authorization %s changed during reconciliation", priorID)
	}
	return authorization, nil
}
