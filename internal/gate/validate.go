package gate

import (
	"strings"

	"ai-dev/internal/workflow"
)

// Valid reports whether r satisfies the Human Gate report field table.
func (r Report) Valid() bool {
	if strings.TrimSpace(r.FeatureID) == "" || strings.TrimSpace(string(r.Decision)) == "" || strings.TrimSpace(r.Approver) == "" {
		return false
	}
	if r.Decision != Approval && r.Decision != Abandonment {
		return false
	}
	canonical := func(state workflow.State) bool {
		matched, ok := workflow.Recognized(string(state))
		return ok && matched == state
	}
	validID := func() bool { _, ok := workflow.FeatureDir(r.FeatureID); return ok }
	baseTarget := func() bool {
		return validID() && canonical(r.FromState) && canonical(r.Target) && r.SuppliedTarget == string(r.Target) && !r.ReasonSet && r.Reason == "" && len(r.Missing) == 0
	}

	switch r.Outcome {
	case TransitionApplied:
		if !validID() || !canonical(r.FromState) || !canonical(r.Target) || r.SuppliedTarget != string(r.Target) || len(r.Missing) != 0 {
			return false
		}
		if r.ReasonSet != (r.Reason != "") || (r.ReasonSet && strings.TrimSpace(r.Reason) == "") {
			return false
		}
		if r.Decision == Abandonment {
			return r.Target == workflow.Abandoned && r.ReasonSet
		}
		return r.Target != workflow.Abandoned
	case TransitionInvalid, StateConflict, PersistenceFailed:
		return baseTarget()
	case ArtifactsMissing:
		return r.Decision == Approval && validID() && canonical(r.FromState) && canonical(r.Target) && r.SuppliedTarget == string(r.Target) && !r.ReasonSet && r.Reason == "" && len(r.Missing) > 0
	case TargetUnsupported:
		_, targetCanonical := workflow.Recognized(r.SuppliedTarget)
		return r.Decision == Approval && strings.TrimSpace(r.SuppliedTarget) != "" && !targetCanonical && r.FromState == "" && r.Target == "" && !r.ReasonSet && r.Reason == "" && len(r.Missing) == 0
	case FeatureUnknown:
		return r.FromState == "" && r.SuppliedTarget == "" && r.Target == "" && !r.ReasonSet && r.Reason == "" && len(r.Missing) == 0
	case StateInvalid:
		return validID() && r.FromState == "" && r.SuppliedTarget == "" && r.Target == "" && !r.ReasonSet && r.Reason == "" && len(r.Missing) == 0
	default:
		return false
	}
}
