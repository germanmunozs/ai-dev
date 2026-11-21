package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/execution"
)

func TestWriteExecutionTextProducesDelimitedCanonicalRecord(t *testing.T) {
	var output bytes.Buffer
	if err := WriteExecutionText(&output, execution.Report{
		RequestedAgentID: "claude-code",
		Outcome:          execution.AgentSucceeded,
	}); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	if !strings.HasPrefix(got, "\n") || !strings.Contains(got, "ai-dev: outcome=agent_succeeded agent=claude-code") {
		t.Fatalf("output = %q", got)
	}
	if strings.Contains(got, "reason=") {
		t.Fatalf("success output unexpectedly contains reason: %q", got)
	}
}

func TestWriteExecutionJSONProducesOneIsolatedObject(t *testing.T) {
	var output bytes.Buffer
	if err := WriteExecutionJSON(&output, execution.Report{
		RequestedAgentID: "codex-cli",
		Outcome:          execution.ExecutionFailed,
		Explanation:      "canceled",
	}); err != nil {
		t.Fatal(err)
	}
	var got struct {
		AgentID     string `json:"agent_id"`
		Outcome     string `json:"outcome"`
		Explanation string `json:"explanation"`
	}
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON %q: %v", output.String(), err)
	}
	if got.AgentID != "codex-cli" || got.Outcome != "execution_failed" || got.Explanation != "canceled" {
		t.Fatalf("JSON report = %#v", got)
	}
}

func TestWriteExecutionFailureReportsRequireExplanationAndPreserveCanonicalValues(t *testing.T) {
	for _, outcome := range []execution.Outcome{execution.AgentFailed, execution.ExecutionFailed} {
		t.Run(string(outcome), func(t *testing.T) {
			report := execution.Report{
				RequestedAgentID: "unsupported-as-supplied",
				Outcome:          outcome,
				Explanation:      "deterministic failure explanation",
			}
			var textOutput bytes.Buffer
			if err := WriteExecutionText(&textOutput, report); err != nil {
				t.Fatal(err)
			}
			text := textOutput.String()
			if !strings.Contains(text, "outcome="+string(outcome)) ||
				!strings.Contains(text, "agent=unsupported-as-supplied") ||
				!strings.Contains(text, `reason="deterministic failure explanation"`) {
				t.Fatalf("text report = %q", text)
			}
			if strings.Contains(text, string(execution.AgentSucceeded)) {
				t.Fatalf("failure text contains success-like outcome: %q", text)
			}

			var jsonOutput bytes.Buffer
			if err := WriteExecutionJSON(&jsonOutput, report); err != nil {
				t.Fatal(err)
			}
			var decoded struct {
				AgentID     string `json:"agent_id"`
				Outcome     string `json:"outcome"`
				Explanation string `json:"explanation"`
			}
			if err := json.Unmarshal(jsonOutput.Bytes(), &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded.AgentID != "unsupported-as-supplied" || decoded.Outcome != string(outcome) || decoded.Explanation == "" {
				t.Fatalf("JSON report = %#v", decoded)
			}
			if strings.Contains(jsonOutput.String(), string(execution.AgentSucceeded)) {
				t.Fatalf("failure JSON contains success-like outcome: %q", jsonOutput.String())
			}
		})
	}
}

func TestWriteExecutionReportRejectsNonCanonicalOrUnexplainedFailure(t *testing.T) {
	tests := []execution.Report{
		{RequestedAgentID: "claude-code", Outcome: execution.AgentFailed},
		{RequestedAgentID: "claude-code", Outcome: execution.ExecutionFailed},
		{RequestedAgentID: "claude-code", Outcome: execution.Outcome("success"), Explanation: "not canonical"},
	}
	for _, report := range tests {
		t.Run(string(report.Outcome), func(t *testing.T) {
			var textOutput bytes.Buffer
			if err := WriteExecutionText(&textOutput, report); err == nil {
				t.Fatalf("WriteExecutionText(%#v) succeeded with output %q", report, textOutput.String())
			}
			if textOutput.Len() != 0 {
				t.Fatalf("invalid text report wrote partial output %q", textOutput.String())
			}

			var jsonOutput bytes.Buffer
			if err := WriteExecutionJSON(&jsonOutput, report); err == nil {
				t.Fatalf("WriteExecutionJSON(%#v) succeeded with output %q", report, jsonOutput.String())
			}
			if jsonOutput.Len() != 0 {
				t.Fatalf("invalid JSON report wrote partial output %q", jsonOutput.String())
			}
		})
	}
}
