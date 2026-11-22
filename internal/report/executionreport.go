package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/germanmunozs/ai-dev/internal/execution"
)

// WriteExecutionText writes the final ai-dev-authored record to the caller's
// selected stream. The leading newline visibly separates it from agent output.
func WriteExecutionText(w io.Writer, report execution.Report) error {
	if err := validateExecutionReport(report); err != nil {
		return err
	}
	if report.Explanation == "" {
		_, err := fmt.Fprintf(w, "\nai-dev: outcome=%s agent=%s\n", report.Outcome, report.RequestedAgentID)
		return err
	}
	_, err := fmt.Fprintf(w, "\nai-dev: outcome=%s agent=%s reason=%q\n", report.Outcome, report.RequestedAgentID, report.Explanation)
	return err
}

// WriteExecutionJSON writes exactly one machine-readable final report.
func WriteExecutionJSON(w io.Writer, report execution.Report) error {
	if err := validateExecutionReport(report); err != nil {
		return err
	}
	wire := struct {
		AgentID     string            `json:"agent_id"`
		Outcome     execution.Outcome `json:"outcome"`
		Explanation string            `json:"explanation"`
	}{
		AgentID:     report.RequestedAgentID,
		Outcome:     report.Outcome,
		Explanation: report.Explanation,
	}
	return json.NewEncoder(w).Encode(wire)
}

func validateExecutionReport(report execution.Report) error {
	switch report.Outcome {
	case execution.AgentSucceeded:
		return nil
	case execution.AgentFailed, execution.ExecutionFailed:
		if strings.TrimSpace(report.Explanation) == "" {
			return fmt.Errorf("%s report requires a non-empty explanation", report.Outcome)
		}
		return nil
	default:
		return fmt.Errorf("invalid execution outcome %q", report.Outcome)
	}
}
