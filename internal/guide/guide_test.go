package guide

import (
	"context"
	"io/fs"
	"reflect"
	"testing"
	"testing/fstest"

	"ai-dev/internal/agent"
	"ai-dev/internal/role"
	"ai-dev/internal/sdd"
	"ai-dev/internal/selection"
	"ai-dev/internal/workflow"
)

const guideFeatureID = "f"

// guideFixture returns a tree with one feature directory and, when record is
// non-nil, its Workflow State Record and the named artifacts.
func guideFixture(featureID string, record *string, files ...string) fstest.MapFS {
	fsys := fstest.MapFS{"specs/" + featureID: &fstest.MapFile{Mode: fs.ModeDir | 0o755}}
	if record != nil {
		fsys["specs/"+featureID+"/"+workflow.RecordFileName] = &fstest.MapFile{Data: []byte(*record)}
	}
	for _, name := range files {
		fsys["specs/"+featureID+"/"+name] = &fstest.MapFile{Data: []byte("fixture")}
	}
	return fsys
}

func record(state string) *string {
	value := `{"state":"` + state + `"}`
	return &value
}

// guideCase is one fixture covering a Workflow State or an invalid case.
type guideCase struct {
	name string
	root fstest.MapFS
	id   string
}

func guideCases() []guideCase {
	all := []string{sdd.SpecArtifact, sdd.PlanArtifact, sdd.TasksArtifact}
	invalid := `{"state":"Planned"}`
	return []guideCase{
		{"draft", guideFixture(guideFeatureID, nil), guideFeatureID},
		{"specified", guideFixture(guideFeatureID, record("specified"), sdd.SpecArtifact), guideFeatureID},
		{"planned", guideFixture(guideFeatureID, record("planned"), sdd.SpecArtifact, sdd.PlanArtifact), guideFeatureID},
		{"tasked", guideFixture(guideFeatureID, record("tasked"), all...), guideFeatureID},
		{"implemented", guideFixture(guideFeatureID, record("implemented"), all...), guideFeatureID},
		{"approved", guideFixture(guideFeatureID, record("approved"), all...), guideFeatureID},
		{"abandoned", guideFixture(guideFeatureID, record("abandoned")), guideFeatureID},
		{"feature unknown", fstest.MapFS{}, guideFeatureID},
		{"structurally invalid id", fstest.MapFS{}, "../x"},
		{"state invalid", guideFixture(guideFeatureID, &invalid), guideFeatureID},
		{"artifacts inconsistent", guideFixture(guideFeatureID, record("implemented"), sdd.SpecArtifact), guideFeatureID},
		{"hostile semicolon", guideFixture("a;b", nil), "a;b"},
		{"hostile substitution", guideFixture("$(x)", nil), "$(x)"},
		{"hostile dash", guideFixture("-f", nil), "-f"},
		{"hostile quote", guideFixture("it's", record("specified"), sdd.SpecArtifact), "it's"},
		{"hostile space", guideFixture("a b", record("approved"), all...), "a b"},
		{"non-ASCII", guideFixture("ñandú", nil), "ñandú"},
	}
}

// failingExecutor allows executable lookup only; any process start fails.
type failingExecutor struct {
	t       *testing.T
	paths   map[string]bool
	lookups []string
}

func (e *failingExecutor) LookPath(file string) (string, error) {
	e.lookups = append(e.lookups, file)
	if e.paths[file] {
		return "/fake/" + file, nil
	}
	return "", fs.ErrNotExist
}

func (e *failingExecutor) Run(context.Context, string, ...string) (string, error) {
	e.t.Fatal("guidance started a process")
	return "", nil
}

// fakeSelector returns a fixed 003 report and records every requested role.
type fakeSelector struct {
	report selection.Report
	calls  []string
}

func (s *fakeSelector) Select(suppliedRole string) selection.Report {
	s.calls = append(s.calls, suppliedRole)
	report := s.report
	report.SuppliedRole = suppliedRole
	if requested, ok := role.Recognized(suppliedRole); ok && report.Outcome != selection.RoleUnsupported {
		report.RequestedRole = requested
	}
	return report
}

func TestEvaluateEquivalentToSDD(t *testing.T) {
	executor := &failingExecutor{t: t, paths: map[string]bool{"claude": true, "codex": true}}
	selector := selection.NewSelector(agent.NewRegistry(executor))
	for _, tc := range guideCases() {
		t.Run(tc.name, func(t *testing.T) {
			want := sdd.Evaluate(tc.root, tc.id)
			got := Evaluate(tc.root, tc.id, selector)
			if !reflect.DeepEqual(got.Report, want) {
				t.Fatalf("guidance 005 fields = %+v, want sdd.Evaluate %+v", got.Report, want)
			}
			if again := Evaluate(tc.root, tc.id, selector); !reflect.DeepEqual(again, got) {
				t.Fatalf("repeated evaluation differs (F007-FR-009):\n%+v\n%+v", got, again)
			}
			if got.Statement != Statement {
				t.Fatalf("statement = %q, want %q", got.Statement, Statement)
			}
			stepRecommended := want.Outcome == sdd.StepRecommended
			if stepRecommended != (got.Agent != "") || stepRecommended != (got.Availability != "") {
				t.Fatalf("agent/availability presence for %s: %+v", want.Outcome, got)
			}
			hasTemplates := want.Outcome == sdd.StepRecommended || want.Outcome == sdd.WorkflowTerminal
			if hasTemplates != (len(got.Templates) > 0) {
				t.Fatalf("templates presence for %s: %+v", want.Outcome, got.Templates)
			}
			if want.Valid() != got.Valid() {
				t.Fatalf("Valid() = %v, want %v as sdd for %+v", got.Valid(), want.Valid(), got)
			}
		})
	}
}

func TestEvaluateAgentEqualsMappingAndAvailability(t *testing.T) {
	mapped := map[role.Role]agent.ID{
		role.Specification:  agent.ClaudeCode,
		role.Planning:       agent.ClaudeCode,
		role.Implementation: agent.CodexCLI,
		role.Review:         agent.ClaudeCode,
	}
	binary := map[agent.ID]string{agent.ClaudeCode: "claude", agent.CodexCLI: "codex"}
	for _, installed := range []map[string]bool{{}, {"claude": true}, {"codex": true}, {"claude": true, "codex": true}} {
		for _, tc := range guideCases() {
			if sdd.Evaluate(tc.root, tc.id).Outcome != sdd.StepRecommended {
				continue
			}
			executor := &failingExecutor{t: t, paths: installed}
			got := Evaluate(tc.root, tc.id, selection.NewSelector(agent.NewRegistry(executor)))
			wantAgent := mapped[got.Role]
			wantAvailability := Unavailable
			if installed[binary[wantAgent]] {
				wantAvailability = Available
			}
			if got.Agent != wantAgent || got.Availability != wantAvailability || !got.Valid() {
				t.Fatalf("%s installed=%v: agent=%q availability=%q valid=%v, want %q %q", tc.name, installed, got.Agent, got.Availability, got.Valid(), wantAgent, wantAvailability)
			}
		}
	}
}

func TestEvaluateAvailabilityMapping(t *testing.T) {
	cases := []struct {
		name             string
		report           selection.Report
		wantAgent        agent.ID
		wantAvailability Availability
		wantValid        bool
	}{
		{"role selected", selection.Report{SelectedAgentID: agent.ClaudeCode, Outcome: selection.RoleSelected}, agent.ClaudeCode, Available, true},
		{"agent unavailable", selection.Report{MappedAgentID: agent.ClaudeCode, Outcome: selection.AgentUnavailable}, agent.ClaudeCode, Unavailable, true},
		{"selection inconsistent", selection.Report{MappedAgentID: agent.ClaudeCode, Outcome: selection.SelectionInconsistent}, agent.ClaudeCode, Inconsistent, true},
		{"role unsupported", selection.Report{Outcome: selection.RoleUnsupported}, "", "", false},
		{"unknown outcome", selection.Report{SelectedAgentID: agent.ClaudeCode, Outcome: "bogus"}, "", "", false},
	}
	root := guideFixture(guideFeatureID, record("specified"), sdd.SpecArtifact)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			selector := &fakeSelector{report: tc.report}
			got := Evaluate(root, guideFeatureID, selector)
			if !reflect.DeepEqual(selector.calls, []string{string(role.Planning)}) {
				t.Fatalf("selector calls = %v, want exactly the 005 role %q", selector.calls, role.Planning)
			}
			if got.Agent != tc.wantAgent || got.Availability != tc.wantAvailability || got.Valid() != tc.wantValid {
				t.Fatalf("got agent=%q availability=%q valid=%v, want %q %q %v", got.Agent, got.Availability, got.Valid(), tc.wantAgent, tc.wantAvailability, tc.wantValid)
			}
		})
	}

	t.Run("selector not consulted without a recommended step", func(t *testing.T) {
		for _, tc := range guideCases() {
			if sdd.Evaluate(tc.root, tc.id).Outcome == sdd.StepRecommended {
				continue
			}
			selector := &fakeSelector{report: selection.Report{SelectedAgentID: agent.ClaudeCode, Outcome: selection.RoleSelected}}
			_ = Evaluate(tc.root, tc.id, selector)
			if len(selector.calls) != 0 {
				t.Fatalf("%s: selector called with %v", tc.name, selector.calls)
			}
		}
	})
}

func TestEvaluateUnsafeIdentifierHasNoTemplates(t *testing.T) {
	for _, id := range []string{"a" + string(rune(0x202e)) + "b", "a" + string(rune(0x200b)) + "b", "a\nb", "a" + string(rune(0x2028)) + "b"} {
		for _, rec := range []*string{nil, record("approved")} {
			var root fstest.MapFS
			if rec == nil {
				root = guideFixture(id, nil)
			} else {
				root = guideFixture(id, rec, sdd.SpecArtifact, sdd.PlanArtifact, sdd.TasksArtifact)
			}
			selector := &fakeSelector{report: selection.Report{SelectedAgentID: agent.ClaudeCode, Outcome: selection.RoleSelected}}
			got := Evaluate(root, id, selector)
			if got.Outcome != sdd.StepRecommended && got.Outcome != sdd.WorkflowTerminal {
				t.Fatalf("%q: outcome = %s, want a templated outcome", id, got.Outcome)
			}
			if len(got.Templates) != 0 || !got.Valid() || got.Statement != Statement {
				t.Fatalf("%q: templates=%v valid=%v statement=%q, want none, valid, statement", id, got.Templates, got.Valid(), got.Statement)
			}
		}
	}
}

func TestValidRejectsAlteredReports(t *testing.T) {
	root := guideFixture(guideFeatureID, record("specified"), sdd.SpecArtifact)
	base := Evaluate(root, guideFeatureID, &fakeSelector{report: selection.Report{SelectedAgentID: agent.ClaudeCode, Outcome: selection.RoleSelected}})
	if !base.Valid() {
		t.Fatalf("base report invalid: %+v", base)
	}
	mutations := map[string]func(*Report){
		"statement changed":    func(r *Report) { r.Statement = "approved" },
		"agent missing":        func(r *Report) { r.Agent = "" },
		"availability unknown": func(r *Report) { r.Availability = "maybe" },
		"template dropped":     func(r *Report) { r.Templates = r.Templates[1:] },
		"slot filled":          func(r *Report) { r.Templates[2].Words[5] = Word{Kind: WordFixed, Text: "planned"} },
		"non-canonical agent token": func(r *Report) {
			r.Agent = "claude code"
			r.Templates = Templates(r.Outcome, r.Availability, r.Role, r.Agent)
		},
		"templates on unsafe feature": func(r *Report) { r.FeatureID = "f" + string(rune(0x202e)) },
		"005 report invalid":          func(r *Report) { r.Role = role.Review },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := Evaluate(root, guideFeatureID, &fakeSelector{report: selection.Report{SelectedAgentID: agent.ClaudeCode, Outcome: selection.RoleSelected}})
			mutate(&changed)
			if changed.Valid() {
				t.Fatalf("Valid accepted %+v", changed)
			}
		})
	}
}
