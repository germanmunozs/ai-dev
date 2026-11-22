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

// reconcileAuthorization issues the linked authorization of a supervised
// reconciliation and maps its outcome to a closed reason.
func reconcileAuthorization(ctx context.Context, store GitStore, journal *attemptJournal, priorID AuthorizationID, in GitAuthorizationInput) *PlanRejection {
	if priorID.Validate() != nil || priorID == in.AuthorizationID {
		return reject(ReasonAuthorizationMismatch, "the reconciled authorization must be another valid authorization ID")
	}
	_, err := issueReconciliation(ctx, store, journal, priorID, in)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrAuthorizationMissing):
		return reject(ReasonAuthorizationMissing, "the reconciled authorization %s does not exist for this project", priorID)
	case errors.Is(err, errNotReconcilable):
		return reject(ReasonAuthorizationMismatch, "%v", err)
	case errors.Is(err, ErrAuthorizationExists):
		return reject(ReasonAuthorizationIDExists, "authorization %s already exists", in.AuthorizationID)
	}
	return reject(ReasonAuditUnavailable, "the reconciliation could not be made durable")
}

// ReconciliationRequest names a prior authorization and the plan file it
// was issued for.
type ReconciliationRequest struct {
	PlanFile        []byte
	AuthorizationID string
}

// ReconciliationDependencies are the read-only ports reconciliation
// inspection uses.
type ReconciliationDependencies struct {
	Store   GitStore
	Inspect func(ctx context.Context, projectPath []byte, candidates [][]byte) (GitInspection, error)
}

// ReconciliationReport is steps 1–2 of the only recovery procedure
// (F008-FR-180): the prior authorization as durably recorded, the audit
// trail of the attempt that consumed it, and every prior planned
// postcondition classified against a fresh inspection. It writes nothing
// and grants nothing; continuing needs a new plan and a new linked
// authorization.
type ReconciliationReport struct {
	Prior         GitAuthorization
	Unresolved    bool
	TrailReadable bool
	Trail         AuditTrail
	Applied       []Condition
	Absent        []Condition
	Unverifiable  []Condition
}

// GrantsMutationAuthority is false for every report.
func (ReconciliationReport) GrantsMutationAuthority() bool { return false }

// InspectReconciliation builds the report. The plan file is authentic
// only because the prior authorization durably names its digest.
func InspectReconciliation(ctx context.Context, deps ReconciliationDependencies, req ReconciliationRequest) (ReconciliationReport, *PlanRejection) {
	if deps.Store == nil || deps.Inspect == nil {
		return ReconciliationReport{}, reject(ReasonAuditUnavailable, "reconciliation inspection is not available")
	}
	file, err := DecodePlanFile(req.PlanFile)
	if err != nil {
		return ReconciliationReport{}, reject(ReasonPlanInvalid, "%v", err)
	}
	id := AuthorizationID(req.AuthorizationID)
	if id.Validate() != nil {
		return ReconciliationReport{}, reject(ReasonAuthorizationMissing, "no valid authorization ID")
	}
	inspection, err := deps.Inspect(ctx, file.ProjectPath, intentCandidates(file.Intent))
	if err != nil {
		var inspectErr *InspectError
		if errors.As(err, &inspectErr) {
			return ReconciliationReport{}, reject(inspectErr.Reason, "%s", inspectErr.Detail)
		}
		return ReconciliationReport{}, reject(ReasonProjectIdentityUnavailable, "the project could not be inspected")
	}
	prior, err := deps.Store.LoadAuthorization(ctx, inspection.ProjectIdentity.Digest(), id)
	switch {
	case errors.Is(err, ErrAuthorizationMissing):
		return ReconciliationReport{}, reject(ReasonAuthorizationMissing, "authorization %s does not exist for this project", id)
	case err != nil:
		return ReconciliationReport{}, reject(ReasonAuditUnavailable, "the authorization cannot be read")
	case prior.PlanDigest != file.PlanDigest:
		return ReconciliationReport{}, reject(ReasonAuthorizationMismatch, "the plan file is not the one the authorization was issued for")
	}
	report := ReconciliationReport{Prior: prior, Unresolved: assessAuthorization(ctx, deps.Store, prior, time.Time{}).unresolved}
	if prior.OwnerApplyID != "" {
		if trail, err := deps.Store.ListAuditEvents(ctx, prior.OwnerApplyID); err == nil {
			report.Trail, report.TrailReadable = trail, true
		}
	}
	for _, condition := range file.ExpectedPostconditions {
		switch evaluateCondition(condition, inspection) {
		case conditionHolds:
			report.Applied = append(report.Applied, condition)
		case conditionFails:
			report.Absent = append(report.Absent, condition)
		default:
			report.Unverifiable = append(report.Unverifiable, condition)
		}
	}
	return report, nil
}

// GitReconciliationReportSchema is the schema of a reconciliation report.
const GitReconciliationReportSchema = "ai-dev.git-reconciliation-report/v1"

// EncodeReconciliationReport renders the report as one canonical object.
// It carries identifiers, states, event kinds and conditions only: no
// operator declaration, path content, token or record bytes.
func EncodeReconciliationReport(r ReconciliationReport) ([]byte, error) {
	e := &canonicalEncoder{}
	prior := map[string]any{
		"authorization_id": string(r.Prior.AuthorizationID), "state": string(r.Prior.State),
		"state_version": canonicalInteger(r.Prior.StateVersion), "plan_digest": string(r.Prior.PlanDigest),
	}
	optional(prior, "owner_apply_id", string(r.Prior.OwnerApplyID), r.Prior.OwnerApplyID != "")
	report := map[string]any{
		"schema_version":            GitReconciliationReportSchema,
		"project_identity_digest":   string(r.Prior.ProjectIdentity.Digest()),
		"prior_authorization":       prior,
		"unresolved":                r.Unresolved,
		"trail_readable":            r.TrailReadable,
		"trail_events":              canonicalStrings(r.Trail.Events),
		"crash_evidence":            r.Trail.CrashEvidence,
		"applied":                   canonicalList(r.Applied, e.condition),
		"absent":                    canonicalList(r.Absent, e.condition),
		"unverifiable":              canonicalList(r.Unverifiable, e.condition),
		"grants_mutation_authority": r.GrantsMutationAuthority(),
	}
	if e.err != nil {
		return nil, e.err
	}
	return MarshalCanonical(report)
}
