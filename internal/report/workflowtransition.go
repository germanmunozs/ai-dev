package report

import (
	"encoding/json"
	"fmt"
	"io"

	"ai-dev/internal/workflow"
)

// WriteWorkflowTransitionText writes one human-readable line reporting the
// given Transition Evaluation Result's outcome.
func WriteWorkflowTransitionText(w io.Writer, r workflow.TransitionReport) error {
	if !r.Valid() {
		return fmt.Errorf("invalid workflow transition report: %+v", r)
	}
	var err error
	switch r.Outcome {
	case workflow.TransitionValid, workflow.TransitionInvalid:
		_, err = fmt.Fprintf(w, "ai-dev: outcome=%s feature=%s current=%s target=%s\n", r.Outcome, shellInertQuote(r.FeatureID), r.CurrentState, r.Target)
	case workflow.TargetUnsupported:
		_, err = fmt.Fprintf(w, "ai-dev: outcome=%s feature=%s target=%s\n", r.Outcome, shellInertQuote(r.FeatureID), shellInertQuote(r.SuppliedTarget))
	default:
		_, err = fmt.Fprintf(w, "ai-dev: outcome=%s feature=%s\n", r.Outcome, shellInertQuote(r.FeatureID))
	}
	return err
}

// WriteWorkflowTransitionJSON writes exactly one machine-readable Transition
// Evaluation Result object.
func WriteWorkflowTransitionJSON(w io.Writer, r workflow.TransitionReport) error {
	if !r.Valid() {
		return fmt.Errorf("invalid workflow transition report: %+v", r)
	}
	wire := struct {
		FeatureID      string           `json:"feature_id"`
		CurrentState   string           `json:"current_state,omitempty"`
		Target         string           `json:"target,omitempty"`
		SuppliedTarget string           `json:"supplied_target,omitempty"`
		Outcome        workflow.Outcome `json:"outcome"`
	}{FeatureID: r.FeatureID, Outcome: r.Outcome}
	switch r.Outcome {
	case workflow.TransitionValid, workflow.TransitionInvalid:
		wire.CurrentState = string(r.CurrentState)
		wire.Target = string(r.Target)
	case workflow.TargetUnsupported:
		wire.SuppliedTarget = r.SuppliedTarget
	}
	return json.NewEncoder(w).Encode(wire)
}
