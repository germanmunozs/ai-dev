package sdd

import (
	"io/fs"

	"github.com/germanmunozs/ai-dev/internal/role"
	"github.com/germanmunozs/ai-dev/internal/workflow"
)

// Evaluate resolves one read-only SDD next-step request.
func Evaluate(root fs.FS, featureID string) Report {
	return evaluate(root, featureID, nextStep)
}

func evaluate(root fs.FS, featureID string, lookup func(workflow.State) (Activity, role.Role, bool)) Report {
	stateReport := workflow.EvaluateState(root, featureID)
	switch stateReport.Outcome {
	case workflow.FeatureUnknown:
		return Report{FeatureID: featureID, Outcome: FeatureUnknown}
	case workflow.StateInvalid:
		return Report{FeatureID: featureID, Outcome: StateInvalid}
	case workflow.StateReported:
	default:
		return Report{FeatureID: featureID}
	}
	if missing := missingArtifacts(root, featureID, stateReport.State); len(missing) > 0 {
		return Report{FeatureID: featureID, State: stateReport.State, Missing: missing, Outcome: ArtifactsInconsistent}
	}
	if stateReport.Terminal {
		return Report{FeatureID: featureID, State: stateReport.State, Outcome: WorkflowTerminal}
	}
	activity, nextRole, ok := lookup(stateReport.State)
	if !ok {
		return Report{FeatureID: featureID}
	}
	return Report{FeatureID: featureID, State: stateReport.State, Activity: activity, Role: nextRole, Outcome: StepRecommended}
}
