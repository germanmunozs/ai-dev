package sdd

import (
	"strings"

	"ai-dev/internal/workflow"
)

// Valid reports whether r satisfies the approved outcome-specific field and
// cross-field invariants for a Next-Step Report.
func (r Report) Valid() bool {
	if strings.TrimSpace(r.FeatureID) == "" {
		return false
	}
	emptyOptional := func() bool {
		return r.State == "" && r.Activity == "" && r.Role == "" && len(r.Missing) == 0
	}
	switch r.Outcome {
	case StepRecommended:
		if _, ok := workflow.FeatureDir(r.FeatureID); !ok || workflow.Terminal(r.State) || len(r.Missing) != 0 {
			return false
		}
		activity, expectedRole, ok := nextStep(r.State)
		return ok && r.Activity == activity && r.Role == expectedRole
	case WorkflowTerminal:
		if _, ok := workflow.FeatureDir(r.FeatureID); !ok {
			return false
		}
		return r.State != "" && workflow.Terminal(r.State) && r.Activity == "" && r.Role == "" && len(r.Missing) == 0
	case ArtifactsInconsistent:
		if _, ok := workflow.FeatureDir(r.FeatureID); !ok || r.State == "" || r.Activity != "" || r.Role != "" || len(r.Missing) == 0 {
			return false
		}
		if _, canonical := workflow.Recognized(string(r.State)); !canonical {
			return false
		}
		required := requiredArtifacts(r.State)
		position := -1
		for _, missing := range r.Missing {
			found := -1
			for i := position + 1; i < len(required); i++ {
				if required[i] == missing {
					found = i
					break
				}
			}
			if found < 0 {
				return false
			}
			position = found
		}
		return true
	case FeatureUnknown:
		return emptyOptional()
	case StateInvalid:
		_, ok := workflow.FeatureDir(r.FeatureID)
		return ok && emptyOptional()
	default:
		return false
	}
}
