package gitmanager

import "context"

// RepositoryPhase is when a repository condition is first established
// (inspection contract §6, c-01).
type RepositoryPhase int

const (
	// PhasePlanning is plan generation or initial plan validation.
	PhasePlanning RepositoryPhase = iota
	// PhaseApplyDrift is apply's fresh inspection and regeneration against
	// the authorized snapshot, before the first mutation.
	PhaseApplyDrift
	// PhaseRevalidated is after an operation's immediate revalidation
	// succeeded.
	PhaseRevalidated
)

// ClassifyRepositoryReason gives the reason for a condition by the phase
// in which it was first observed: the planning reason stands during
// planning, any difference first seen against the authorized snapshot is
// plan_stale, and a change after successful immediate revalidation is
// concurrent_change. Final-state similarity never overrides the phase.
func ClassifyRepositoryReason(phase RepositoryPhase, planning Reason) Reason {
	switch phase {
	case PhaseApplyDrift:
		return ReasonPlanStale
	case PhaseRevalidated:
		return ReasonConcurrentChange
	default:
		return planning
	}
}

// RegenerateAuthorized regenerates the plan for apply from the authorized
// immutable intent and a fresh inspection. Any refusal, and any covered
// difference from the authorized digest, is plan_stale (F008-FR-055–058):
// the original planning reason is kept only as the detail.
func RegenerateAuthorized(ctx context.Context, deps PlannerDependencies, intent ImmutableIntent, authorized PlanDigest,
	inspection GitInspection, environment EnvironmentProfile, meta PlanMetadata) (GitInitializationPlan, *PlanRejection) {
	plan, rejection := Regenerate(ctx, deps, intent, inspection, environment, meta)
	if rejection != nil {
		return GitInitializationPlan{}, reject(ClassifyRepositoryReason(PhaseApplyDrift, rejection.Reason), "%s: %s", rejection.Reason, rejection.Detail)
	}
	if plan.PlanDigest != authorized {
		return GitInitializationPlan{}, reject(ReasonPlanStale, "the regenerated plan differs from the authorized plan")
	}
	return plan, nil
}
