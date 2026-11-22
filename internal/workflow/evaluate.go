package workflow

import "io/fs"

// EvaluateState resolves one state-reporting request. It is unaffected by
// FR-029 (no candidate target): feature existence, then record resolution.
func EvaluateState(root fs.FS, featureID string) StateReport {
	if !FeatureExists(root, featureID) {
		return StateReport{FeatureID: featureID, Outcome: FeatureUnknown}
	}
	state, invalid := ResolveRecord(root, featureID)
	if invalid {
		return StateReport{FeatureID: featureID, Outcome: StateInvalid}
	}
	return StateReport{FeatureID: featureID, State: state, Terminal: Terminal(state), Outcome: StateReported}
}

// EvaluateTransition resolves one Transition Evaluation Request under the
// fixed precedence order (FR-029). Candidate-target canonicality (step 2) is
// checked and can return before FeatureExists/ResolveRecord (steps 3-4) —
// the only filesystem I/O in this feature — are ever called; usage
// validation (step 1) happens in the caller (RunWorkflowTransition) before
// this function is invoked at all.
func EvaluateTransition(root fs.FS, featureID, suppliedTarget string) TransitionReport {
	target, ok := Recognized(suppliedTarget)
	if !ok {
		return TransitionReport{FeatureID: featureID, SuppliedTarget: suppliedTarget, Outcome: TargetUnsupported}
	}
	// FeatureExists performs raw feature-ID validation before constructing a path.
	if !FeatureExists(root, featureID) {
		return TransitionReport{FeatureID: featureID, Outcome: FeatureUnknown}
	}
	state, invalid := ResolveRecord(root, featureID)
	if invalid {
		return TransitionReport{FeatureID: featureID, Outcome: StateInvalid}
	}
	if Allowed(state, target) {
		return TransitionReport{FeatureID: featureID, CurrentState: state, SuppliedTarget: suppliedTarget, Target: target, Outcome: TransitionValid}
	}
	return TransitionReport{FeatureID: featureID, CurrentState: state, SuppliedTarget: suppliedTarget, Target: target, Outcome: TransitionInvalid}
}
