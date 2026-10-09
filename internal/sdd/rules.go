package sdd

import (
	"io/fs"

	"ai-dev/internal/role"
	"ai-dev/internal/workflow"
)

func nextStep(state workflow.State) (Activity, role.Role, bool) {
	switch state {
	case workflow.Draft:
		return ActivitySpecify, role.Specification, true
	case workflow.Specified:
		return ActivityPlan, role.Planning, true
	case workflow.Planned:
		return ActivityTasks, role.Planning, true
	case workflow.Tasked:
		return ActivityImplement, role.Implementation, true
	case workflow.Implemented:
		return ActivityReview, role.Review, true
	default:
		return "", "", false
	}
}

func requiredArtifacts(state workflow.State) []string {
	switch state {
	case workflow.Specified:
		return []string{SpecArtifact}
	case workflow.Planned:
		return []string{SpecArtifact, PlanArtifact}
	case workflow.Tasked, workflow.Implemented, workflow.Approved:
		return []string{SpecArtifact, PlanArtifact, TasksArtifact}
	default:
		return nil
	}
}

func missingArtifacts(root fs.FS, featureID string, state workflow.State) []string {
	required := requiredArtifacts(state)
	if len(required) == 0 {
		return nil
	}
	dir, ok := workflow.FeatureDir(featureID)
	if !ok {
		return required
	}
	entries, err := fs.ReadDir(root, dir)
	if err != nil {
		return required
	}
	present := make(map[string]bool, len(required))
	for _, entry := range entries {
		if entry.Type().IsRegular() {
			present[entry.Name()] = true
		}
	}
	missing := make([]string, 0, len(required))
	for _, name := range required {
		if !present[name] {
			missing = append(missing, name)
		}
	}
	return missing
}

// MissingArtifacts reports required artifacts absent from featureID's
// feature directory under Feature 005's exact presence rule and order.
func MissingArtifacts(root fs.FS, featureID string, state workflow.State) []string {
	return missingArtifacts(root, featureID, state)
}
