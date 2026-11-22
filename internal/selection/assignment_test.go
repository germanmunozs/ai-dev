package selection

import (
	"testing"

	"github.com/germanmunozs/ai-dev/internal/agent"
	"github.com/germanmunozs/ai-dev/internal/projectconfig"
	"github.com/germanmunozs/ai-dev/internal/role"
)

func projectConfig(t *testing.T, implementation agent.ID) projectconfig.Config {
	t.Helper()
	config, reasons := projectconfig.New([]agent.ID{agent.ClaudeCode, agent.CodexCLI}, map[role.Role]agent.ID{
		role.Specification: agent.CodexCLI, role.Planning: agent.ClaudeCode, role.Implementation: implementation,
		role.Review: agent.CodexCLI, role.Remediation: agent.ClaudeCode,
	})
	if len(reasons) != 0 {
		t.Fatal(reasons)
	}
	return config
}

func onlyClaudeAvailable() *fakeExecutor {
	return &fakeExecutor{paths: map[string]string{"claude": "/tools/claude"}}
}

// The built-in assignment is 003 FR-003 with source built_in (A1).
func TestBuiltInAssignmentIsTheFixedMapping(t *testing.T) {
	selector := NewSelectorFor(agent.NewRegistry(bothAgentsAvailable()), BuiltIn())
	for _, r := range projectconfig.Roles() {
		got := selector.Select(string(r))
		want, _ := mappedAgent(r)
		if got.Outcome != RoleSelected || got.SelectedAgentID != want || got.Source != SourceBuiltIn || !got.Valid() {
			t.Errorf("%s: %+v", r, got)
		}
	}
	if got := NewSelector(agent.NewRegistry(bothAgentsAvailable())).Select("review"); got.Source != SourceBuiltIn {
		t.Fatalf("NewSelector source = %q", got.Source)
	}
}

// A valid project configuration replaces the built-in mapping, with the
// same availability rules (F011-FR-015).
func TestProjectAssignment(t *testing.T) {
	assignment := FromProject(projectConfig(t, agent.CodexCLI))
	selector := NewSelectorFor(agent.NewRegistry(onlyClaudeAvailable()), assignment)
	got := selector.Select("specification")
	if got.Outcome != AgentUnavailable || got.MappedAgentID != agent.CodexCLI || got.Source != SourceProject || !got.Valid() {
		t.Fatalf("specification: %+v", got)
	}
	got = selector.Select("planning")
	if got.Outcome != RoleSelected || got.SelectedAgentID != agent.ClaudeCode || got.Source != SourceProject || !got.Valid() {
		t.Fatalf("planning: %+v", got)
	}
}

// An invalid configuration never falls back to the built-in mapping
// (F011-FR-016).
func TestInvalidProjectAssignment(t *testing.T) {
	for _, assignment := range []Assignment{InvalidProject(), FromProject(projectconfig.Config{})} {
		selector := NewSelectorFor(agent.NewRegistry(bothAgentsAvailable()), assignment)
		for _, r := range projectconfig.Roles() {
			got := selector.Select(string(r))
			if got.Outcome != ConfigurationInvalid || got.RequestedRole != r || got.SelectedAgentID != "" || got.MappedAgentID != "" || got.Source != SourceProject || !got.Valid() {
				t.Errorf("%s: %+v", r, got)
			}
		}
	}
}

func TestUnrecognizedRoleCarriesTheSource(t *testing.T) {
	for _, test := range []struct {
		assignment Assignment
		source     Source
	}{{BuiltIn(), SourceBuiltIn}, {FromProject(projectConfig(t, agent.ClaudeCode)), SourceProject}, {InvalidProject(), SourceProject}} {
		got := NewSelectorFor(agent.NewRegistry(bothAgentsAvailable()), test.assignment).Select("Review")
		if got.Outcome != RoleUnsupported || got.Source != test.source || !got.Valid() {
			t.Errorf("%+v", got)
		}
	}
}

// Valid checks a project report for presence and catalog membership, and
// never accepts selection_inconsistent from a project (F011-FR-018).
func TestValidProjectReports(t *testing.T) {
	for _, test := range []struct {
		name   string
		report Report
		want   bool
	}{
		{"selected", Report{SuppliedRole: "review", RequestedRole: role.Review, SelectedAgentID: agent.CodexCLI, Outcome: RoleSelected, Source: SourceProject}, true},
		{"selected unknown agent", Report{SuppliedRole: "review", RequestedRole: role.Review, SelectedAgentID: "gemini", Outcome: RoleSelected, Source: SourceProject}, false},
		{"unavailable", Report{SuppliedRole: "review", RequestedRole: role.Review, MappedAgentID: agent.ClaudeCode, Outcome: AgentUnavailable, Source: SourceProject}, true},
		{"inconsistent from project", Report{SuppliedRole: "review", RequestedRole: role.Review, MappedAgentID: agent.ClaudeCode, Outcome: SelectionInconsistent, Source: SourceProject}, false},
		{"configuration invalid built in", Report{SuppliedRole: "review", RequestedRole: role.Review, Outcome: ConfigurationInvalid, Source: SourceBuiltIn}, false},
		{"configuration invalid with agent", Report{SuppliedRole: "review", RequestedRole: role.Review, MappedAgentID: agent.ClaudeCode, Outcome: ConfigurationInvalid, Source: SourceProject}, false},
		{"no source", Report{SuppliedRole: "review", RequestedRole: role.Review, SelectedAgentID: agent.ClaudeCode, Outcome: RoleSelected}, false},
		{"built in wrong agent", Report{SuppliedRole: "review", RequestedRole: role.Review, SelectedAgentID: agent.CodexCLI, Outcome: RoleSelected, Source: SourceBuiltIn}, false},
	} {
		if got := test.report.Valid(); got != test.want {
			t.Errorf("%s: Valid() = %t, want %t", test.name, got, test.want)
		}
	}
}

func TestAssignmentAgentFor(t *testing.T) {
	if id, ok := BuiltIn().AgentFor(role.Implementation); !ok || id != agent.CodexCLI {
		t.Fatalf("built-in implementation = %q %t", id, ok)
	}
	if id, ok := FromProject(projectConfig(t, agent.ClaudeCode)).AgentFor(role.Implementation); !ok || id != agent.ClaudeCode {
		t.Fatalf("project implementation = %q %t", id, ok)
	}
	if _, ok := InvalidProject().AgentFor(role.Review); ok {
		t.Fatal("an invalid assignment gave an agent")
	}
}
