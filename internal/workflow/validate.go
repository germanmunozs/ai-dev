package workflow

import "strings"

// Valid reports whether r is a StateReport the approved state-reporting
// model could actually have produced, enforcing the per-outcome field
// presence and cross-field invariants from data-model.md. Report writers call
// it to reject a malformed report at render time rather than emit it.
func (r StateReport) Valid() bool {
	if strings.TrimSpace(r.FeatureID) == "" {
		return false
	}
	switch r.Outcome {
	case StateReported:
		_, canonical := Recognized(string(r.State))
		return validFeatureID(r.FeatureID) && canonical && r.Terminal == Terminal(r.State)
	case FeatureUnknown:
		// The identifier may be structurally invalid (deliberately unresolved)
		// or structurally valid but absent from the filesystem.
		return r.State == "" && !r.Terminal
	case StateInvalid:
		// Feature existence was established first, so the identifier is
		// structurally valid.
		return validFeatureID(r.FeatureID) && r.State == "" && !r.Terminal
	default:
		return false
	}
}

// Valid reports whether r is a TransitionReport the approved
// transition-evaluation model could actually have produced, enforcing the
// per-outcome field presence and cross-field invariants from data-model.md.
func (r TransitionReport) Valid() bool {
	if strings.TrimSpace(r.FeatureID) == "" {
		return false
	}
	switch r.Outcome {
	case TransitionValid, TransitionInvalid:
		_, currentCanonical := Recognized(string(r.CurrentState))
		_, targetCanonical := Recognized(string(r.Target))
		if !validFeatureID(r.FeatureID) || !currentCanonical || !targetCanonical || r.SuppliedTarget != string(r.Target) {
			return false
		}
		return Allowed(r.CurrentState, r.Target) == (r.Outcome == TransitionValid)
	case TargetUnsupported:
		// FR-029 returns this outcome before feature-ID validation, so the
		// identifier need not be structurally valid.
		_, canonical := Recognized(r.SuppliedTarget)
		return strings.TrimSpace(r.SuppliedTarget) != "" && !canonical && r.CurrentState == "" && r.Target == ""
	case FeatureUnknown:
		return r.CurrentState == "" && r.SuppliedTarget == "" && r.Target == ""
	case StateInvalid:
		return validFeatureID(r.FeatureID) && r.CurrentState == "" && r.SuppliedTarget == "" && r.Target == ""
	default:
		return false
	}
}
