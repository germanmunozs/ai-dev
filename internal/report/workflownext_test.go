package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/role"
	"github.com/germanmunozs/ai-dev/internal/sdd"
	"github.com/germanmunozs/ai-dev/internal/workflow"
)

func decodeExactlyOneWorkflowNextJSON(t *testing.T, data []byte) map[string]any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		t.Fatalf("decode first JSON object: %v; output=%q", err, data)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("second Decode = %v, want io.EOF; trailing=%v output=%q", err, trailing, data)
	}
	return object
}

func assertWorkflowNextNoForbiddenContent(t *testing.T, output string, allowDisclaimer bool) {
	t.Helper()
	check := strings.ToLower(output)
	if allowDisclaimer {
		check = strings.ReplaceAll(check, "recommendation only; does not authorize this activity or any transition", "")
	}
	for _, forbidden := range []string{"target", "next_state", "agent", "gate", "authorized", "approved_by", "approval", "authoriz"} {
		if strings.Contains(check, forbidden) {
			t.Errorf("output contains forbidden content %q: %s", forbidden, output)
		}
	}
}

func TestWorkflowNextWritersRejectInvalidReportsBeforeWriting(t *testing.T) {
	invalid := []sdd.Report{
		{},
		{FeatureID: "f"},
		{FeatureID: "f", Outcome: "other"},
		{FeatureID: " ", Outcome: sdd.FeatureUnknown},
		{FeatureID: "f", State: workflow.Planned, Outcome: sdd.StepRecommended},
		{FeatureID: "f", State: workflow.Planned, Activity: sdd.ActivityTasks, Role: role.Planning, Missing: []string{sdd.PlanArtifact}, Outcome: sdd.StepRecommended},
		{FeatureID: "f", State: workflow.Planned, Outcome: sdd.WorkflowTerminal},
		{FeatureID: "f", State: workflow.Planned, Missing: nil, Outcome: sdd.ArtifactsInconsistent},
		{FeatureID: "f", State: workflow.Planned, Outcome: sdd.FeatureUnknown},
		{FeatureID: "../x", Outcome: sdd.StateInvalid},
	}
	writers := map[string]func(io.Writer, sdd.Report) error{
		"text": WriteWorkflowNextText,
		"json": WriteWorkflowNextJSON,
	}
	for i, report := range invalid {
		for name, write := range writers {
			t.Run(name+" invalid "+string(rune('a'+i)), func(t *testing.T) {
				var output bytes.Buffer
				if err := write(&output, report); err == nil {
					t.Fatalf("writer accepted invalid report %+v", report)
				}
				if output.Len() != 0 {
					t.Fatalf("writer emitted %q before rejecting %+v", output.String(), report)
				}
			})
		}
	}
}

func TestWriteWorkflowNextStepRecommended(t *testing.T) {
	cases := []sdd.Report{
		{FeatureID: "f", State: workflow.Draft, Activity: sdd.ActivitySpecify, Role: role.Specification, Outcome: sdd.StepRecommended},
		{FeatureID: "f", State: workflow.Specified, Activity: sdd.ActivityPlan, Role: role.Planning, Outcome: sdd.StepRecommended},
		{FeatureID: "f", State: workflow.Planned, Activity: sdd.ActivityTasks, Role: role.Planning, Outcome: sdd.StepRecommended},
		{FeatureID: "f", State: workflow.Tasked, Activity: sdd.ActivityImplement, Role: role.Implementation, Outcome: sdd.StepRecommended},
		{FeatureID: "f", State: workflow.Implemented, Activity: sdd.ActivityReview, Role: role.Review, Outcome: sdd.StepRecommended},
	}
	for _, report := range cases {
		t.Run(string(report.State), func(t *testing.T) {
			var textOutput bytes.Buffer
			if err := WriteWorkflowNextText(&textOutput, report); err != nil {
				t.Fatal(err)
			}
			wantText := fmt.Sprintf("ai-dev: outcome=step_recommended feature=%q state=%s activity=%s role=%s note=\"recommendation only; does not authorize this activity or any transition\"\n", report.FeatureID, report.State, report.Activity, report.Role)
			if textOutput.String() != wantText || strings.Count(textOutput.String(), "\n") != 1 {
				t.Fatalf("text = %q, want %q", textOutput.String(), wantText)
			}
			assertWorkflowNextNoForbiddenContent(t, textOutput.String(), true)

			var jsonOutput bytes.Buffer
			if err := WriteWorkflowNextJSON(&jsonOutput, report); err != nil {
				t.Fatal(err)
			}
			wantJSON := map[string]any{
				"feature_id":    "f",
				"outcome":       "step_recommended",
				"current_state": string(report.State),
				"activity":      string(report.Activity),
				"role":          string(report.Role),
			}
			if got := decodeExactlyOneWorkflowNextJSON(t, jsonOutput.Bytes()); !reflect.DeepEqual(got, wantJSON) {
				t.Fatalf("JSON = %#v, want %#v", got, wantJSON)
			}
			assertWorkflowNextNoForbiddenContent(t, jsonOutput.String(), false)
		})
	}
}

func TestWriteWorkflowNextStepRecommendedValidUTF8Fidelity(t *testing.T) {
	id := "feature-<>&-español"
	report := sdd.Report{FeatureID: id, State: workflow.Draft, Activity: sdd.ActivitySpecify, Role: role.Specification, Outcome: sdd.StepRecommended}
	var textOutput bytes.Buffer
	if err := WriteWorkflowNextText(&textOutput, report); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(textOutput.String(), "feature="+fmt.Sprintf("%q", id)) || strings.Count(textOutput.String(), "\n") != 1 {
		t.Fatalf("text did not quote ID on exactly one line: %q", textOutput.String())
	}
	var jsonOutput bytes.Buffer
	if err := WriteWorkflowNextJSON(&jsonOutput, report); err != nil {
		t.Fatal(err)
	}
	got := decodeExactlyOneWorkflowNextJSON(t, jsonOutput.Bytes())
	if got["feature_id"] != id {
		t.Fatalf("JSON feature_id = %q, want %q", got["feature_id"], id)
	}
}

func TestWriteWorkflowNextTerminal(t *testing.T) {
	for _, state := range []workflow.State{workflow.Approved, workflow.Abandoned} {
		t.Run(string(state), func(t *testing.T) {
			report := sdd.Report{FeatureID: "f", State: state, Outcome: sdd.WorkflowTerminal}
			var textOutput bytes.Buffer
			if err := WriteWorkflowNextText(&textOutput, report); err != nil {
				t.Fatal(err)
			}
			wantText := fmt.Sprintf("ai-dev: outcome=workflow_terminal feature=%q state=%s\n", "f", state)
			if textOutput.String() != wantText {
				t.Fatalf("text = %q, want %q", textOutput.String(), wantText)
			}
			assertWorkflowNextNoForbiddenContent(t, textOutput.String(), false)

			var jsonOutput bytes.Buffer
			if err := WriteWorkflowNextJSON(&jsonOutput, report); err != nil {
				t.Fatal(err)
			}
			wantJSON := map[string]any{"feature_id": "f", "outcome": "workflow_terminal", "current_state": string(state)}
			if got := decodeExactlyOneWorkflowNextJSON(t, jsonOutput.Bytes()); !reflect.DeepEqual(got, wantJSON) {
				t.Fatalf("JSON = %#v, want %#v", got, wantJSON)
			}
			assertWorkflowNextNoForbiddenContent(t, jsonOutput.String(), false)
		})
	}
}

func TestWriteWorkflowNextFailureOutcomes(t *testing.T) {
	cases := []struct {
		report   sdd.Report
		wantText string
		wantJSON map[string]any
	}{
		{
			sdd.Report{FeatureID: "f", State: workflow.Planned, Missing: []string{sdd.SpecArtifact, sdd.PlanArtifact}, Outcome: sdd.ArtifactsInconsistent},
			"ai-dev: outcome=artifacts_inconsistent feature=\"f\" state=planned missing=spec.md,plan.md\n",
			map[string]any{"feature_id": "f", "outcome": "artifacts_inconsistent", "current_state": "planned", "missing_artifacts": []any{"spec.md", "plan.md"}},
		},
		{
			sdd.Report{FeatureID: "missing", Outcome: sdd.FeatureUnknown},
			"ai-dev: outcome=feature_unknown feature=\"missing\"\n",
			map[string]any{"feature_id": "missing", "outcome": "feature_unknown"},
		},
		{
			sdd.Report{FeatureID: "f", Outcome: sdd.StateInvalid},
			"ai-dev: outcome=state_invalid feature=\"f\"\n",
			map[string]any{"feature_id": "f", "outcome": "state_invalid"},
		},
	}
	for _, tc := range cases {
		t.Run(string(tc.report.Outcome), func(t *testing.T) {
			var textOutput bytes.Buffer
			if err := WriteWorkflowNextText(&textOutput, tc.report); err != nil {
				t.Fatal(err)
			}
			if textOutput.String() != tc.wantText {
				t.Fatalf("text = %q, want %q", textOutput.String(), tc.wantText)
			}
			var jsonOutput bytes.Buffer
			if err := WriteWorkflowNextJSON(&jsonOutput, tc.report); err != nil {
				t.Fatal(err)
			}
			if got := decodeExactlyOneWorkflowNextJSON(t, jsonOutput.Bytes()); !reflect.DeepEqual(got, tc.wantJSON) {
				t.Fatalf("JSON = %#v, want %#v", got, tc.wantJSON)
			}
		})
	}
}

func TestWriteWorkflowNextFeatureUnknownIsInjectionSafe(t *testing.T) {
	ids := []string{"line\nbreak", "tab\tvalue", "carriage\rreturn", `quote"value`, `backslash\value`, "space value", "outcome=step_recommended"}
	for _, id := range ids {
		t.Run(fmt.Sprintf("%q", id), func(t *testing.T) {
			report := sdd.Report{FeatureID: id, Outcome: sdd.FeatureUnknown}
			var output bytes.Buffer
			if err := WriteWorkflowNextText(&output, report); err != nil {
				t.Fatal(err)
			}
			quoted := fmt.Sprintf("%q", id)
			want := "ai-dev: outcome=feature_unknown feature=" + quoted + "\n"
			if output.String() != want || strings.Count(output.String(), "\n") != 1 {
				t.Fatalf("text = %q, want exactly %q", output.String(), want)
			}
			if strings.ContainsRune(id, '\t') && strings.ContainsRune(output.String(), '\t') {
				t.Fatalf("raw tab present in output %q", output.String())
			}
			if strings.ContainsRune(id, '\r') && strings.ContainsRune(output.String(), '\r') {
				t.Fatalf("raw carriage return present in output %q", output.String())
			}
			remainder := strings.Replace(output.String(), quoted, "", 1)
			if strings.Count(remainder, "outcome=") != 1 {
				t.Fatalf("outcome injection in %q", output.String())
			}
		})
	}
}

func TestWriteWorkflowNextInvalidUTF8InheritedBehavior(t *testing.T) {
	id := string([]byte{'f', 0xff})
	report := sdd.Report{FeatureID: id, Outcome: sdd.FeatureUnknown}
	var textOutput bytes.Buffer
	if err := WriteWorkflowNextText(&textOutput, report); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(textOutput.String(), `feature="f\xff"`) {
		t.Fatalf("text = %q, want invalid byte escaped as \\xff", textOutput.String())
	}
	var jsonOutput bytes.Buffer
	if err := WriteWorkflowNextJSON(&jsonOutput, report); err != nil {
		t.Fatal(err)
	}
	got := decodeExactlyOneWorkflowNextJSON(t, jsonOutput.Bytes())
	if got["feature_id"] != "f\uFFFD" {
		t.Fatalf("JSON feature_id = %q, want U+FFFD replacement", got["feature_id"])
	}
}
