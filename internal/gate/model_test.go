package gate

import (
	"testing"

	"ai-dev/internal/workflow"
)

func TestModelTokens(t *testing.T) {
	checks := map[string]string{
		string(FeatureUnknown):    string(workflow.FeatureUnknown),
		string(StateInvalid):      string(workflow.StateInvalid),
		string(TargetUnsupported): string(workflow.TargetUnsupported),
		string(TransitionInvalid): string(workflow.TransitionInvalid),
		string(TransitionApplied): "transition_applied",
		string(ArtifactsMissing):  "artifacts_missing",
		string(StateConflict):     "state_conflict",
		string(PersistenceFailed): "persistence_failed",
		string(Approval):          "approval",
		string(Abandonment):       "abandonment",
	}
	for got, want := range checks {
		if got != want {
			t.Fatalf("token %q, want %q", got, want)
		}
	}
	if Committed != 1 || Conflict != 2 || Failed != 3 || CommitStatus(0) == Committed {
		t.Fatalf("commit statuses = %d/%d/%d", Committed, Conflict, Failed)
	}
}

func TestReportValid(t *testing.T) {
	valid := []Report{
		{FeatureID: "f", Decision: Approval, Approver: "alice", FromState: workflow.Draft, SuppliedTarget: "specified", Target: workflow.Specified, Outcome: TransitionApplied},
		{FeatureID: "f", Decision: Approval, Approver: "alice", FromState: workflow.Draft, SuppliedTarget: "specified", Target: workflow.Specified, Reason: "ok", ReasonSet: true, Outcome: TransitionApplied},
		{FeatureID: "f", Decision: Abandonment, Approver: "alice", FromState: workflow.Draft, SuppliedTarget: "abandoned", Target: workflow.Abandoned, Reason: "stop", ReasonSet: true, Outcome: TransitionApplied},
		{FeatureID: "f", Decision: Approval, Approver: "alice", FromState: workflow.Specified, SuppliedTarget: "planned", Target: workflow.Planned, Outcome: TransitionInvalid},
		{FeatureID: "f", Decision: Approval, Approver: "alice", FromState: workflow.Specified, SuppliedTarget: "planned", Target: workflow.Planned, Outcome: StateConflict},
		{FeatureID: "f", Decision: Approval, Approver: "alice", FromState: workflow.Specified, SuppliedTarget: "planned", Target: workflow.Planned, Outcome: PersistenceFailed},
		{FeatureID: "f", Decision: Approval, Approver: "alice", FromState: workflow.Specified, SuppliedTarget: "planned", Target: workflow.Planned, Missing: []string{"plan.md"}, Outcome: ArtifactsMissing},
		{FeatureID: "../x", Decision: Approval, Approver: "alice", SuppliedTarget: "Planned", Outcome: TargetUnsupported},
		{FeatureID: "../x", Decision: Approval, Approver: "alice", Outcome: FeatureUnknown},
		{FeatureID: "f", Decision: Approval, Approver: "alice", Outcome: StateInvalid},
	}
	for i, report := range valid {
		if !report.Valid() {
			t.Errorf("valid[%d] rejected: %+v", i, report)
		}
	}

	base := valid[0]
	invalid := []Report{
		{},
		withFeature(base, " "),
		withDecision(base, ""),
		withApprover(base, "\t"),
		withSuppliedTarget(base, "planned"),
		{FeatureID: "f", Decision: Abandonment, Approver: "a", FromState: workflow.Draft, SuppliedTarget: "specified", Target: workflow.Specified, Reason: "x", ReasonSet: true, Outcome: TransitionApplied},
		{FeatureID: "f", Decision: Abandonment, Approver: "a", FromState: workflow.Draft, SuppliedTarget: "abandoned", Target: workflow.Abandoned, Outcome: TransitionApplied},
		{FeatureID: "f", Decision: Approval, Approver: "a", FromState: workflow.Draft, SuppliedTarget: "abandoned", Target: workflow.Abandoned, Outcome: TransitionApplied},
		{FeatureID: "f", Decision: Approval, Approver: "a", FromState: workflow.Draft, SuppliedTarget: "specified", Target: workflow.Specified, Reason: "x", ReasonSet: true, Outcome: TransitionInvalid},
		{FeatureID: "f", Decision: Approval, Approver: "a", FromState: workflow.Draft, SuppliedTarget: "specified", Target: workflow.Specified, Outcome: ArtifactsMissing},
		{FeatureID: "f", Decision: Abandonment, Approver: "a", FromState: workflow.Draft, SuppliedTarget: "abandoned", Target: workflow.Abandoned, Missing: []string{"spec.md"}, Outcome: ArtifactsMissing},
		{FeatureID: "f", Decision: Approval, Approver: "a", SuppliedTarget: "planned", Outcome: TargetUnsupported},
		{FeatureID: "f", Decision: Approval, Approver: "a", SuppliedTarget: "Planned", FromState: workflow.Draft, Outcome: TargetUnsupported},
		{FeatureID: "f", Decision: Approval, Approver: "a", SuppliedTarget: "Planned", Target: workflow.Planned, Outcome: TargetUnsupported},
		{FeatureID: "f", Decision: Abandonment, Approver: "a", SuppliedTarget: "Planned", Outcome: TargetUnsupported},
		{FeatureID: "f", Decision: Approval, Approver: "a", FromState: workflow.Draft, Outcome: FeatureUnknown},
		{FeatureID: "../x", Decision: Approval, Approver: "a", Outcome: StateInvalid},
		{FeatureID: "f", Decision: Approval, Approver: "a", Outcome: Outcome("unknown")},
	}
	for i, report := range invalid {
		if report.Valid() {
			t.Errorf("invalid[%d] accepted: %+v", i, report)
		}
	}
}

func withFeature(r Report, value string) Report        { r.FeatureID = value; return r }
func withDecision(r Report, value Decision) Report     { r.Decision = value; return r }
func withApprover(r Report, value string) Report       { r.Approver = value; return r }
func withSuppliedTarget(r Report, value string) Report { r.SuppliedTarget = value; return r }
