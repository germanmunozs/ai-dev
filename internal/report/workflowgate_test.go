package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/gate"
	"github.com/germanmunozs/ai-dev/internal/workflow"
)

func gateReports() []gate.Report {
	base := gate.Report{FeatureID: "f", Decision: gate.Approval, Approver: "alice", FromState: workflow.Specified, SuppliedTarget: "planned", Target: workflow.Planned}
	return []gate.Report{
		func() gate.Report { r := base; r.Outcome = gate.TransitionApplied; return r }(),
		func() gate.Report { r := base; r.Outcome = gate.TransitionInvalid; return r }(),
		func() gate.Report { r := base; r.Outcome = gate.StateConflict; return r }(),
		func() gate.Report { r := base; r.Outcome = gate.PersistenceFailed; return r }(),
		func() gate.Report {
			r := base
			r.Outcome = gate.ArtifactsMissing
			r.Missing = []string{"plan.md"}
			return r
		}(),
		{FeatureID: "f", Decision: gate.Approval, Approver: "alice", SuppliedTarget: "Planned", Outcome: gate.TargetUnsupported},
		{FeatureID: "../x", Decision: gate.Approval, Approver: "alice", Outcome: gate.FeatureUnknown},
		{FeatureID: "f", Decision: gate.Approval, Approver: "alice", Outcome: gate.StateInvalid},
	}
}

func TestWriteWorkflowGateAllOutcomes(t *testing.T) {
	for _, r := range gateReports() {
		t.Run(string(r.Outcome), func(t *testing.T) {
			var text bytes.Buffer
			if err := WriteWorkflowGateText(&text, r); err != nil {
				t.Fatal(err)
			}
			if strings.Count(text.String(), "\n") != 1 || !strings.Contains(text.String(), "outcome="+string(r.Outcome)) {
				t.Fatalf("text=%q", text.String())
			}
			if r.Outcome == gate.TransitionApplied {
				if !strings.Contains(text.String(), "approver identity is not verified") {
					t.Fatalf("text=%q", text.String())
				}
			} else if !strings.Contains(text.String(), "no transition was applied") {
				t.Fatalf("text=%q", text.String())
			}
			for _, forbidden := range []string{"authenticated", "verified approver", "identity verified", "approved by"} {
				if strings.Contains(strings.ToLower(text.String()), forbidden) {
					t.Fatalf("forbidden %q in %q", forbidden, text.String())
				}
			}
			var jsonOut bytes.Buffer
			if err := WriteWorkflowGateJSON(&jsonOut, r); err != nil {
				t.Fatal(err)
			}
			got := decodeOneWorkflowJSON(t, jsonOut.Bytes())
			for _, key := range []string{"feature_id", "outcome", "decision", "approver"} {
				if _, ok := got[key]; !ok {
					t.Fatalf("missing %s: %v", key, got)
				}
			}
		})
	}
}

func TestWriteWorkflowGateReasonInjectionAndInvalid(t *testing.T) {
	r := gate.Report{FeatureID: "f\noutcome=transition_applied", Decision: gate.Approval, Approver: "a\n\"", FromState: workflow.Draft, SuppliedTarget: "specified", Target: workflow.Specified, Reason: "r\n\"", ReasonSet: true, Outcome: gate.TransitionApplied}
	var out bytes.Buffer
	if err := WriteWorkflowGateText(&out, r); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), "\n") != 1 {
		t.Fatalf("injection split line: %q", out.String())
	}
	bad := gate.Report{}
	out.Reset()
	if err := WriteWorkflowGateText(&out, bad); err == nil || out.Len() != 0 {
		t.Fatalf("invalid text err=%v out=%q", err, out.String())
	}
	if err := WriteWorkflowGateJSON(&out, bad); err == nil || out.Len() != 0 {
		t.Fatalf("invalid json err=%v out=%q", err, out.String())
	}
}
