package report

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/workflow"
)

func TestWriteWorkflowTransitionTextAndJSONPerOutcome(t *testing.T) {
	cases := []struct {
		report   workflow.TransitionReport
		wantText string
		wantJSON map[string]any
	}{
		{workflow.TransitionReport{FeatureID: "004-workflow-state", CurrentState: workflow.Planned, SuppliedTarget: "tasked", Target: workflow.Tasked, Outcome: workflow.TransitionValid},
			"ai-dev: outcome=transition_valid feature=\"004-workflow-state\" current=planned target=tasked\n",
			map[string]any{"feature_id": "004-workflow-state", "current_state": "planned", "target": "tasked", "outcome": "transition_valid"}},
		{workflow.TransitionReport{FeatureID: "004-workflow-state", CurrentState: workflow.Planned, SuppliedTarget: "approved", Target: workflow.Approved, Outcome: workflow.TransitionInvalid},
			"ai-dev: outcome=transition_invalid feature=\"004-workflow-state\" current=planned target=approved\n",
			map[string]any{"feature_id": "004-workflow-state", "current_state": "planned", "target": "approved", "outcome": "transition_invalid"}},
		{workflow.TransitionReport{FeatureID: "does-not-exist", SuppliedTarget: "reviewing", Outcome: workflow.TargetUnsupported},
			"ai-dev: outcome=target_unsupported feature=\"does-not-exist\" target=\"reviewing\"\n",
			map[string]any{"feature_id": "does-not-exist", "supplied_target": "reviewing", "outcome": "target_unsupported"}},
		{workflow.TransitionReport{FeatureID: "004-workflow-state", SuppliedTarget: " Planned ", Outcome: workflow.TargetUnsupported},
			"ai-dev: outcome=target_unsupported feature=\"004-workflow-state\" target=\" Planned \"\n",
			map[string]any{"feature_id": "004-workflow-state", "supplied_target": " Planned ", "outcome": "target_unsupported"}},
		{workflow.TransitionReport{FeatureID: "does-not-exist", Outcome: workflow.FeatureUnknown},
			"ai-dev: outcome=feature_unknown feature=\"does-not-exist\"\n",
			map[string]any{"feature_id": "does-not-exist", "outcome": "feature_unknown"}},
		{workflow.TransitionReport{FeatureID: "004-workflow-state", Outcome: workflow.StateInvalid},
			"ai-dev: outcome=state_invalid feature=\"004-workflow-state\"\n",
			map[string]any{"feature_id": "004-workflow-state", "outcome": "state_invalid"}},
	}
	for _, tc := range cases {
		var text, js bytes.Buffer
		if err := WriteWorkflowTransitionText(&text, tc.report); err != nil {
			t.Fatalf("text %+v: %v", tc.report, err)
		}
		if text.String() != tc.wantText {
			t.Fatalf("text = %q, want %q", text.String(), tc.wantText)
		}
		if err := WriteWorkflowTransitionJSON(&js, tc.report); err != nil {
			t.Fatalf("json %+v: %v", tc.report, err)
		}
		assertExactWorkflowJSON(t, decodeOneWorkflowJSON(t, js.Bytes()), tc.wantJSON)
	}
}

func TestWriteWorkflowTransitionRejectsInvalidReports(t *testing.T) {
	invalid := []workflow.TransitionReport{
		{FeatureID: "004-workflow-state", SuppliedTarget: "reviewing", Outcome: workflow.FeatureUnknown},
		{FeatureID: "004-workflow-state", SuppliedTarget: "reviewing", Outcome: workflow.StateInvalid},
		{FeatureID: "004-workflow-state", CurrentState: workflow.Planned, Outcome: workflow.StateInvalid},
		{FeatureID: "004-workflow-state", Target: workflow.Planned, Outcome: workflow.FeatureUnknown},
		{FeatureID: "004-workflow-state", CurrentState: workflow.Planned, SuppliedTarget: "approved", Target: workflow.Approved, Outcome: workflow.TransitionValid},
		{FeatureID: "004-workflow-state", SuppliedTarget: "planned", Outcome: workflow.TargetUnsupported},
		{FeatureID: "a/b", CurrentState: workflow.Planned, SuppliedTarget: "tasked", Target: workflow.Tasked, Outcome: workflow.TransitionValid},
	}
	for _, r := range invalid {
		for name, write := range map[string]func(io.Writer, workflow.TransitionReport) error{
			"text": WriteWorkflowTransitionText, "json": WriteWorkflowTransitionJSON,
		} {
			var out bytes.Buffer
			if err := write(&out, r); err == nil {
				t.Fatalf("%s writer accepted invalid report %+v", name, r)
			}
			if out.Len() != 0 {
				t.Fatalf("%s writer rendered output for invalid report %+v: %q", name, r, out.String())
			}
		}
	}
}

// injectionFeatureID tries to smuggle a forged success line into text output.
const injectionFeatureID = "x\nai-dev: outcome=transition_valid feature=f current=draft target=specified"

func TestWriteWorkflowTransitionTextQuotesAdversarialFeatureIDs(t *testing.T) {
	cases := []struct {
		report workflow.TransitionReport
		want   string
	}{
		{workflow.TransitionReport{FeatureID: injectionFeatureID, SuppliedTarget: "reviewing", Outcome: workflow.TargetUnsupported},
			`ai-dev: outcome=target_unsupported feature="x\nai-dev: outcome=transition_valid feature=f current=draft target=specified" target="reviewing"` + "\n"},
		{workflow.TransitionReport{FeatureID: injectionFeatureID, Outcome: workflow.FeatureUnknown},
			`ai-dev: outcome=feature_unknown feature="x\nai-dev: outcome=transition_valid feature=f current=draft target=specified"` + "\n"},
		{workflow.TransitionReport{FeatureID: injectionFeatureID, Outcome: workflow.StateInvalid},
			`ai-dev: outcome=state_invalid feature="x\nai-dev: outcome=transition_valid feature=f current=draft target=specified"` + "\n"},
		{workflow.TransitionReport{FeatureID: injectionFeatureID, CurrentState: workflow.Planned, SuppliedTarget: "approved", Target: workflow.Approved, Outcome: workflow.TransitionInvalid},
			`ai-dev: outcome=transition_invalid feature="x\nai-dev: outcome=transition_valid feature=f current=draft target=specified" current=planned target=approved` + "\n"},
		{workflow.TransitionReport{FeatureID: `a "b" \c`, SuppliedTarget: "x\ry", Outcome: workflow.TargetUnsupported},
			`ai-dev: outcome=target_unsupported feature="a \"b\" \\c" target="x\ry"` + "\n"},
		{workflow.TransitionReport{FeatureID: "a b\tc", CurrentState: workflow.Draft, SuppliedTarget: "specified", Target: workflow.Specified, Outcome: workflow.TransitionValid},
			`ai-dev: outcome=transition_valid feature="a b\tc" current=draft target=specified` + "\n"},
	}
	for _, tc := range cases {
		var out bytes.Buffer
		if err := WriteWorkflowTransitionText(&out, tc.report); err != nil {
			t.Fatalf("WriteWorkflowTransitionText(%+v): %v", tc.report, err)
		}
		if out.String() != tc.want {
			t.Fatalf("text = %q, want %q", out.String(), tc.want)
		}
		if strings.Count(out.String(), "\n") != 1 {
			t.Fatalf("text is not exactly one line: %q", out.String())
		}
	}
}
