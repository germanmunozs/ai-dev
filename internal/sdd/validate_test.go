package sdd

import (
	"reflect"
	"testing"

	"ai-dev/internal/role"
	"ai-dev/internal/workflow"
)

func TestReportValidAcceptsEveryOutcome(t *testing.T) {
	valid := []Report{
		{FeatureID: "f", State: workflow.Draft, Activity: ActivitySpecify, Role: role.Specification, Outcome: StepRecommended},
		{FeatureID: "f", State: workflow.Specified, Activity: ActivityPlan, Role: role.Planning, Outcome: StepRecommended},
		{FeatureID: "f", State: workflow.Planned, Activity: ActivityTasks, Role: role.Planning, Outcome: StepRecommended},
		{FeatureID: "f", State: workflow.Tasked, Activity: ActivityImplement, Role: role.Implementation, Outcome: StepRecommended},
		{FeatureID: "f", State: workflow.Implemented, Activity: ActivityReview, Role: role.Review, Outcome: StepRecommended},
		{FeatureID: "f", State: workflow.Approved, Outcome: WorkflowTerminal},
		{FeatureID: "f", State: workflow.Abandoned, Outcome: WorkflowTerminal},
		{FeatureID: "f", State: workflow.Specified, Missing: []string{SpecArtifact}, Outcome: ArtifactsInconsistent},
		{FeatureID: "f", State: workflow.Planned, Missing: []string{PlanArtifact}, Outcome: ArtifactsInconsistent},
		{FeatureID: "f", State: workflow.Implemented, Missing: []string{SpecArtifact, TasksArtifact}, Outcome: ArtifactsInconsistent},
		{FeatureID: "../x", Outcome: FeatureUnknown},
		{FeatureID: "f", Outcome: StateInvalid},
	}
	for _, report := range valid {
		if !report.Valid() {
			t.Errorf("Valid() = false for %+v", report)
		}
	}
}

func TestReportValidRejectsInvalidCombinations(t *testing.T) {
	step := Report{FeatureID: "f", State: workflow.Planned, Activity: ActivityTasks, Role: role.Planning, Outcome: StepRecommended}
	terminal := Report{FeatureID: "f", State: workflow.Approved, Outcome: WorkflowTerminal}
	inconsistent := Report{FeatureID: "f", State: workflow.Tasked, Missing: []string{SpecArtifact, TasksArtifact}, Outcome: ArtifactsInconsistent}
	unknown := Report{FeatureID: "../x", Outcome: FeatureUnknown}
	invalidState := Report{FeatureID: "f", Outcome: StateInvalid}

	cases := map[string]Report{
		"blank feature":                    {FeatureID: " \t", Outcome: FeatureUnknown},
		"empty outcome":                    {FeatureID: "f"},
		"unknown outcome":                  {FeatureID: "f", Outcome: "other"},
		"step missing state":               func() Report { r := step; r.State = ""; return r }(),
		"step missing activity":            func() Report { r := step; r.Activity = ""; return r }(),
		"step missing role":                func() Report { r := step; r.Role = ""; return r }(),
		"step prohibited missing":          func() Report { r := step; r.Missing = []string{PlanArtifact}; return r }(),
		"step wrong activity":              func() Report { r := step; r.Activity = ActivityPlan; return r }(),
		"step wrong role":                  func() Report { r := step; r.Role = role.Review; return r }(),
		"step terminal state":              func() Report { r := step; r.State = workflow.Approved; return r }(),
		"step invalid feature ID":          func() Report { r := step; r.FeatureID = "../x"; return r }(),
		"terminal missing state":           func() Report { r := terminal; r.State = ""; return r }(),
		"terminal nonterminal state":       func() Report { r := terminal; r.State = workflow.Planned; return r }(),
		"terminal prohibited activity":     func() Report { r := terminal; r.Activity = ActivityReview; return r }(),
		"terminal prohibited role":         func() Report { r := terminal; r.Role = role.Review; return r }(),
		"terminal prohibited missing":      func() Report { r := terminal; r.Missing = []string{TasksArtifact}; return r }(),
		"terminal invalid feature ID":      func() Report { r := terminal; r.FeatureID = "a/b"; return r }(),
		"inconsistent missing state":       func() Report { r := inconsistent; r.State = ""; return r }(),
		"inconsistent prohibited activity": func() Report { r := inconsistent; r.Activity = ActivityTasks; return r }(),
		"inconsistent prohibited role":     func() Report { r := inconsistent; r.Role = role.Planning; return r }(),
		"inconsistent empty missing":       func() Report { r := inconsistent; r.Missing = nil; return r }(),
		"inconsistent unordered":           func() Report { r := inconsistent; r.Missing = []string{TasksArtifact, SpecArtifact}; return r }(),
		"inconsistent duplicate":           func() Report { r := inconsistent; r.Missing = []string{SpecArtifact, SpecArtifact}; return r }(),
		"inconsistent nonrequired": func() Report {
			r := inconsistent
			r.State = workflow.Planned
			r.Missing = []string{TasksArtifact}
			return r
		}(),
		"inconsistent no requirements": func() Report {
			r := inconsistent
			r.State = workflow.Draft
			r.Missing = []string{SpecArtifact}
			return r
		}(),
		"inconsistent invalid feature ID": func() Report { r := inconsistent; r.FeatureID = "../x"; return r }(),
		"unknown prohibited state":        func() Report { r := unknown; r.State = workflow.Draft; return r }(),
		"unknown prohibited activity":     func() Report { r := unknown; r.Activity = ActivitySpecify; return r }(),
		"unknown prohibited role":         func() Report { r := unknown; r.Role = role.Specification; return r }(),
		"unknown prohibited missing":      func() Report { r := unknown; r.Missing = []string{SpecArtifact}; return r }(),
		"state invalid bad feature ID":    func() Report { r := invalidState; r.FeatureID = "../x"; return r }(),
		"state invalid prohibited state":  func() Report { r := invalidState; r.State = workflow.Planned; return r }(),
		"state invalid activity":          func() Report { r := invalidState; r.Activity = ActivityPlan; return r }(),
		"state invalid role":              func() Report { r := invalidState; r.Role = role.Planning; return r }(),
		"state invalid missing":           func() Report { r := invalidState; r.Missing = []string{SpecArtifact}; return r }(),
	}
	for name, report := range cases {
		t.Run(name, func(t *testing.T) {
			if report.Valid() {
				t.Fatalf("Valid() = true for invalid report %+v", report)
			}
		})
	}
}

func TestTableMissRepresentation(t *testing.T) {
	got := Report{FeatureID: "f"}
	want := Report{FeatureID: "f"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("table miss = %+v, want %+v", got, want)
	}
	if got.Valid() {
		t.Fatal("table-miss report unexpectedly valid")
	}
}

func TestStateWithoutOutcomeIsAdditionalInvalidInput(t *testing.T) {
	report := Report{FeatureID: "f", State: workflow.Planned}
	if report.Valid() {
		t.Fatal("state-bearing report without outcome unexpectedly valid")
	}
}
