package app

import (
	"context"
	"fmt"

	"ai-dev/internal/gitmanager"
)

// InspectGitReconciliation composes the read-only reconciliation
// inspection (T105): it opens the store at the canonical state root,
// reads the prior authorization and its attempt trail, and classifies the
// prior plan's postconditions on a fresh inspection. It writes nothing.
//
// Continuing is a new plan applied with an issuance whose
// ReconcilesAuthorizationID names the prior authorization; applyPlan runs
// it through the same engine. No CLI route exists yet: the options are a
// proposed amendment (REC-04).
func InspectGitReconciliation(ctx context.Context, deps GitManagementDependencies, planFile []byte, authorizationID string) (gitmanager.ReconciliationReport, *gitmanager.PlanRejection, error) {
	if deps.OpenStore == nil || deps.LookupEnv == nil {
		return gitmanager.ReconciliationReport{}, nil, fmt.Errorf("Git reconciliation is unavailable in this build")
	}
	stateRoot, err := gitmanager.ResolveStateRoot(deps.GOOS, deps.LookupEnv)
	if err != nil {
		return gitmanager.ReconciliationReport{}, nil, err
	}
	store, closeStore, err := deps.OpenStore(stateRoot)
	if err != nil {
		return gitmanager.ReconciliationReport{}, nil, fmt.Errorf("the Git state store is unavailable: %v", err)
	}
	defer closeStore()
	report, rejection := gitmanager.InspectReconciliation(ctx, gitmanager.ReconciliationDependencies{
		Store: store,
		Inspect: func(ctx context.Context, projectPath []byte, candidates [][]byte) (gitmanager.GitInspection, error) {
			inspection, _, err := inspectWithEnvironment(ctx, deps, string(projectPath), candidates)
			return inspection, err
		},
	}, gitmanager.ReconciliationRequest{PlanFile: planFile, AuthorizationID: authorizationID})
	return report, rejection, nil
}
