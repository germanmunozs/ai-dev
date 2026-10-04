package report

import (
	"encoding/json"
	"fmt"
	"io"

	"ai-dev/internal/workflow"
)

// WriteWorkflowStateText writes one human-readable line reporting the given
// Workflow State Report's outcome.
func WriteWorkflowStateText(w io.Writer, r workflow.StateReport) error {
	if !r.Valid() {
		return fmt.Errorf("invalid workflow state report: %+v", r)
	}
	if r.Outcome == workflow.StateReported {
		_, err := fmt.Fprintf(w, "ai-dev: outcome=%s feature=%q state=%s terminal=%t\n", r.Outcome, r.FeatureID, r.State, r.Terminal)
		return err
	}
	_, err := fmt.Fprintf(w, "ai-dev: outcome=%s feature=%q\n", r.Outcome, r.FeatureID)
	return err
}

// WriteWorkflowStateJSON writes exactly one machine-readable Workflow State
// Report object.
func WriteWorkflowStateJSON(w io.Writer, r workflow.StateReport) error {
	if !r.Valid() {
		return fmt.Errorf("invalid workflow state report: %+v", r)
	}
	wire := struct {
		FeatureID string           `json:"feature_id"`
		State     string           `json:"state,omitempty"`
		Terminal  *bool            `json:"terminal,omitempty"`
		Outcome   workflow.Outcome `json:"outcome"`
	}{FeatureID: r.FeatureID, Outcome: r.Outcome}
	if r.Outcome == workflow.StateReported {
		terminal := r.Terminal
		wire.State = string(r.State)
		wire.Terminal = &terminal
	}
	return json.NewEncoder(w).Encode(wire)
}
