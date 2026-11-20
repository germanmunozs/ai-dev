package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"ai-dev/internal/sdd"
)

// WriteWorkflowNextText writes one human-readable Next-Step Report line.
func WriteWorkflowNextText(w io.Writer, report sdd.Report) error {
	if !report.Valid() {
		return fmt.Errorf("invalid workflow next report: %+v", report)
	}
	if report.Outcome == sdd.StepRecommended {
		_, err := fmt.Fprintf(w, "ai-dev: outcome=%s feature=%s state=%s activity=%s role=%s note=\"recommendation only; does not authorize this activity or any transition\"\n", report.Outcome, shellInertQuote(report.FeatureID), report.State, report.Activity, report.Role)
		return err
	}
	if report.Outcome == sdd.WorkflowTerminal {
		_, err := fmt.Fprintf(w, "ai-dev: outcome=%s feature=%s state=%s\n", report.Outcome, shellInertQuote(report.FeatureID), report.State)
		return err
	}
	if report.Outcome == sdd.ArtifactsInconsistent {
		_, err := fmt.Fprintf(w, "ai-dev: outcome=%s feature=%s state=%s missing=%s\n", report.Outcome, shellInertQuote(report.FeatureID), report.State, strings.Join(report.Missing, ","))
		return err
	}
	if report.Outcome == sdd.FeatureUnknown || report.Outcome == sdd.StateInvalid {
		_, err := fmt.Fprintf(w, "ai-dev: outcome=%s feature=%s\n", report.Outcome, shellInertQuote(report.FeatureID))
		return err
	}
	return fmt.Errorf("workflow next outcome %q cannot be rendered", report.Outcome)
}

// WriteWorkflowNextJSON writes exactly one machine-readable Next-Step Report.
func WriteWorkflowNextJSON(w io.Writer, report sdd.Report) error {
	if !report.Valid() {
		return fmt.Errorf("invalid workflow next report: %+v", report)
	}
	wire := struct {
		FeatureID        string      `json:"feature_id"`
		Outcome          sdd.Outcome `json:"outcome"`
		CurrentState     string      `json:"current_state,omitempty"`
		Activity         string      `json:"activity,omitempty"`
		Role             string      `json:"role,omitempty"`
		MissingArtifacts []string    `json:"missing_artifacts,omitempty"`
	}{FeatureID: report.FeatureID, Outcome: report.Outcome}
	if report.Outcome == sdd.StepRecommended {
		wire.CurrentState = string(report.State)
		wire.Activity = string(report.Activity)
		wire.Role = string(report.Role)
		return json.NewEncoder(w).Encode(wire)
	}
	if report.Outcome == sdd.WorkflowTerminal {
		wire.CurrentState = string(report.State)
		return json.NewEncoder(w).Encode(wire)
	}
	if report.Outcome == sdd.ArtifactsInconsistent {
		wire.CurrentState = string(report.State)
		wire.MissingArtifacts = report.Missing
		return json.NewEncoder(w).Encode(wire)
	}
	if report.Outcome == sdd.FeatureUnknown || report.Outcome == sdd.StateInvalid {
		return json.NewEncoder(w).Encode(wire)
	}
	return fmt.Errorf("workflow next outcome %q cannot be rendered", report.Outcome)
}
