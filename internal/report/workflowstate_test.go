package report

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"testing"

	"ai-dev/internal/workflow"
)

// decodeOneWorkflowJSON decodes data as exactly one JSON object. A second
// Decode call must return io.EOF; Decoder.More() is deliberately not used,
// because only a second Decode proves the stream is exhausted.
func decodeOneWorkflowJSON(t *testing.T, data []byte) map[string]any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	var got map[string]any
	if err := decoder.Decode(&got); err != nil {
		t.Fatalf("decode JSON object: %v: %q", err, data)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("stdout carries content after one JSON value (err=%v): %q", err, data)
	}
	return got
}

// assertExactWorkflowJSON fails unless got has exactly the keys in want, each
// with an identical typed value (so JSON false differs from "false" and from
// an absent key).
func assertExactWorkflowJSON(t *testing.T, got, want map[string]any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON fields = %#v, want %#v", got, want)
	}
}

func TestWorkflowJSONHelpersRejectWrongShapes(t *testing.T) {
	got := map[string]any{"terminal": false}
	for name, want := range map[string]map[string]any{
		"string false": {"terminal": "false"},
		"absent key":   {},
		"extra key":    {"terminal": false, "extra": "x"},
	} {
		if reflect.DeepEqual(got, want) {
			t.Fatalf("%s: helper comparison would accept %#v", name, want)
		}
	}
}

func TestWriteWorkflowStateTextPerOutcome(t *testing.T) {
	cases := []struct {
		report workflow.StateReport
		want   string
	}{
		{workflow.StateReport{FeatureID: "004-workflow-state", State: workflow.Planned, Terminal: false, Outcome: workflow.StateReported},
			"ai-dev: outcome=state_reported feature=\"004-workflow-state\" state=planned terminal=false\n"},
		{workflow.StateReport{FeatureID: "004-workflow-state", State: workflow.Approved, Terminal: true, Outcome: workflow.StateReported},
			"ai-dev: outcome=state_reported feature=\"004-workflow-state\" state=approved terminal=true\n"},
		{workflow.StateReport{FeatureID: "does-not-exist", Outcome: workflow.FeatureUnknown},
			"ai-dev: outcome=feature_unknown feature=\"does-not-exist\"\n"},
		{workflow.StateReport{FeatureID: "../x", Outcome: workflow.FeatureUnknown},
			"ai-dev: outcome=feature_unknown feature=\"../x\"\n"},
		{workflow.StateReport{FeatureID: "004-workflow-state", Outcome: workflow.StateInvalid},
			"ai-dev: outcome=state_invalid feature=\"004-workflow-state\"\n"},
	}
	for _, tc := range cases {
		var out bytes.Buffer
		if err := WriteWorkflowStateText(&out, tc.report); err != nil {
			t.Fatalf("WriteWorkflowStateText(%+v): %v", tc.report, err)
		}
		if out.String() != tc.want {
			t.Fatalf("text = %q, want %q", out.String(), tc.want)
		}
	}
}

func TestWriteWorkflowStateJSONPerOutcome(t *testing.T) {
	cases := []struct {
		report workflow.StateReport
		want   map[string]any
	}{
		{workflow.StateReport{FeatureID: "004-workflow-state", State: workflow.Planned, Terminal: false, Outcome: workflow.StateReported},
			map[string]any{"feature_id": "004-workflow-state", "state": "planned", "terminal": false, "outcome": "state_reported"}},
		{workflow.StateReport{FeatureID: "004-workflow-state", State: workflow.Abandoned, Terminal: true, Outcome: workflow.StateReported},
			map[string]any{"feature_id": "004-workflow-state", "state": "abandoned", "terminal": true, "outcome": "state_reported"}},
		{workflow.StateReport{FeatureID: "does-not-exist", Outcome: workflow.FeatureUnknown},
			map[string]any{"feature_id": "does-not-exist", "outcome": "feature_unknown"}},
		{workflow.StateReport{FeatureID: "004-workflow-state", Outcome: workflow.StateInvalid},
			map[string]any{"feature_id": "004-workflow-state", "outcome": "state_invalid"}},
	}
	for _, tc := range cases {
		var out bytes.Buffer
		if err := WriteWorkflowStateJSON(&out, tc.report); err != nil {
			t.Fatalf("WriteWorkflowStateJSON(%+v): %v", tc.report, err)
		}
		assertExactWorkflowJSON(t, decodeOneWorkflowJSON(t, out.Bytes()), tc.want)
	}
}

func TestWriteWorkflowStateRejectsInvalidReports(t *testing.T) {
	invalid := []workflow.StateReport{
		{FeatureID: "004-workflow-state", State: "Planned", Outcome: workflow.StateReported},
		{FeatureID: "004-workflow-state", State: workflow.Planned, Terminal: true, Outcome: workflow.StateReported},
		{FeatureID: "004-workflow-state", State: workflow.Planned, Outcome: workflow.FeatureUnknown},
		{FeatureID: "../x", State: workflow.Planned, Outcome: workflow.StateReported},
		{FeatureID: "", Outcome: workflow.StateInvalid},
	}
	for _, r := range invalid {
		for name, write := range map[string]func(io.Writer, workflow.StateReport) error{
			"text": WriteWorkflowStateText, "json": WriteWorkflowStateJSON,
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

func TestWriteWorkflowStateTextQuotesAdversarialFeatureIDs(t *testing.T) {
	cases := []struct {
		report workflow.StateReport
		want   string
	}{
		{workflow.StateReport{FeatureID: injectionFeatureID, State: workflow.Draft, Outcome: workflow.StateReported},
			`ai-dev: outcome=state_reported feature="x\nai-dev: outcome=transition_valid feature=f current=draft target=specified" state=draft terminal=false` + "\n"},
		{workflow.StateReport{FeatureID: injectionFeatureID, Outcome: workflow.FeatureUnknown},
			`ai-dev: outcome=feature_unknown feature="x\nai-dev: outcome=transition_valid feature=f current=draft target=specified"` + "\n"},
		{workflow.StateReport{FeatureID: `a "b" \c`, State: workflow.Approved, Terminal: true, Outcome: workflow.StateReported},
			`ai-dev: outcome=state_reported feature="a \"b\" \\c" state=approved terminal=true` + "\n"},
		{workflow.StateReport{FeatureID: "a b", Outcome: workflow.StateInvalid},
			`ai-dev: outcome=state_invalid feature="a b"` + "\n"},
	}
	for _, tc := range cases {
		var out bytes.Buffer
		if err := WriteWorkflowStateText(&out, tc.report); err != nil {
			t.Fatalf("WriteWorkflowStateText(%+v): %v", tc.report, err)
		}
		if out.String() != tc.want {
			t.Fatalf("text = %q, want %q", out.String(), tc.want)
		}
		if bytes.Count(out.Bytes(), []byte("\n")) != 1 {
			t.Fatalf("text is not exactly one line: %q", out.String())
		}
	}
}
