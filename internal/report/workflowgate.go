package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/germanmunozs/ai-dev/internal/gate"
)

const gateAppliedNote = "transition applied on the decision asserted by the named approver; approver identity is not verified"
const gateNotAppliedNote = "no transition was applied"

// WriteWorkflowGateText writes one human-readable Human Gate report line.
func WriteWorkflowGateText(w io.Writer, r gate.Report) error {
	if !r.Valid() {
		return fmt.Errorf("invalid workflow gate report: %+v", r)
	}
	switch r.Outcome {
	case gate.TransitionApplied:
		if r.ReasonSet {
			_, err := fmt.Fprintf(w, "ai-dev: outcome=%s feature=%s decision=%s approver=%s from=%s to=%s reason=%s note=%q\n", r.Outcome, shellInertQuote(r.FeatureID), r.Decision, shellInertQuote(r.Approver), r.FromState, r.Target, shellInertQuote(r.Reason), gateAppliedNote)
			return err
		}
		_, err := fmt.Fprintf(w, "ai-dev: outcome=%s feature=%s decision=%s approver=%s from=%s to=%s note=%q\n", r.Outcome, shellInertQuote(r.FeatureID), r.Decision, shellInertQuote(r.Approver), r.FromState, r.Target, gateAppliedNote)
		return err
	case gate.TransitionInvalid, gate.StateConflict, gate.PersistenceFailed:
		_, err := fmt.Fprintf(w, "ai-dev: outcome=%s feature=%s decision=%s approver=%s from=%s target=%s note=%q\n", r.Outcome, shellInertQuote(r.FeatureID), r.Decision, shellInertQuote(r.Approver), r.FromState, r.Target, gateNotAppliedNote)
		return err
	case gate.ArtifactsMissing:
		_, err := fmt.Fprintf(w, "ai-dev: outcome=%s feature=%s decision=%s approver=%s from=%s target=%s missing=%s note=%q\n", r.Outcome, shellInertQuote(r.FeatureID), r.Decision, shellInertQuote(r.Approver), r.FromState, r.Target, strings.Join(r.Missing, ","), gateNotAppliedNote)
		return err
	case gate.TargetUnsupported:
		_, err := fmt.Fprintf(w, "ai-dev: outcome=%s feature=%s decision=%s approver=%s target=%s note=%q\n", r.Outcome, shellInertQuote(r.FeatureID), r.Decision, shellInertQuote(r.Approver), shellInertQuote(r.SuppliedTarget), gateNotAppliedNote)
		return err
	case gate.FeatureUnknown, gate.StateInvalid:
		_, err := fmt.Fprintf(w, "ai-dev: outcome=%s feature=%s decision=%s approver=%s note=%q\n", r.Outcome, shellInertQuote(r.FeatureID), r.Decision, shellInertQuote(r.Approver), gateNotAppliedNote)
		return err
	default:
		return fmt.Errorf("workflow gate outcome %q cannot be rendered", r.Outcome)
	}
}

// WriteWorkflowGateJSON writes exactly one machine-readable Human Gate report.
func WriteWorkflowGateJSON(w io.Writer, r gate.Report) error {
	if !r.Valid() {
		return fmt.Errorf("invalid workflow gate report: %+v", r)
	}
	wire := struct {
		FeatureID      string        `json:"feature_id"`
		Outcome        gate.Outcome  `json:"outcome"`
		Decision       gate.Decision `json:"decision"`
		Approver       string        `json:"approver"`
		FromState      string        `json:"from_state,omitempty"`
		SuppliedTarget string        `json:"supplied_target,omitempty"`
		Target         string        `json:"target,omitempty"`
		Reason         *string       `json:"reason,omitempty"`
		Missing        []string      `json:"missing_artifacts,omitempty"`
	}{FeatureID: r.FeatureID, Outcome: r.Outcome, Decision: r.Decision, Approver: r.Approver}
	switch r.Outcome {
	case gate.TransitionApplied:
		wire.FromState = string(r.FromState)
		wire.SuppliedTarget = r.SuppliedTarget
		wire.Target = string(r.Target)
		if r.ReasonSet {
			reason := r.Reason
			wire.Reason = &reason
		}
	case gate.TransitionInvalid, gate.StateConflict, gate.PersistenceFailed:
		wire.FromState = string(r.FromState)
		wire.SuppliedTarget = r.SuppliedTarget
		wire.Target = string(r.Target)
	case gate.ArtifactsMissing:
		wire.FromState = string(r.FromState)
		wire.SuppliedTarget = r.SuppliedTarget
		wire.Target = string(r.Target)
		wire.Missing = r.Missing
	case gate.TargetUnsupported:
		wire.SuppliedTarget = r.SuppliedTarget
	}
	return json.NewEncoder(w).Encode(wire)
}
