package selection

import (
	"testing"

	"github.com/germanmunozs/ai-dev/internal/agent"
	"github.com/germanmunozs/ai-dev/internal/role"
)

// Every Report the selector can produce is Valid, for each outcome it
// reaches through the real registry.
func TestValidAcceptsEveryProducibleReport(t *testing.T) {
	onlyClaude := &fakeExecutor{paths: map[string]string{"claude": "/tools/claude"}}
	var reports []Report
	for _, supplied := range []string{"specification", "planning", "implementation", "review", "remediation"} {
		reports = append(reports, NewSelector(agent.NewRegistry(bothAgentsAvailable())).Select(supplied))
	}
	reports = append(reports,
		NewSelector(agent.NewRegistry(onlyClaude)).Select("implementation"),
		NewSelector(agent.NewRegistry(bothAgentsAvailable())).Select("architect"),
		resolveAgainstRegistry("review", role.Review, agent.ClaudeCode, agent.NewRegistry(&fakeExecutor{})),
	)
	outcomes := map[Outcome]bool{}
	for _, report := range reports {
		if !report.Valid() {
			t.Errorf("Valid() = false for producible report %+v", report)
		}
		outcomes[report.Outcome] = true
	}
	for _, outcome := range []Outcome{RoleSelected, RoleUnsupported, AgentUnavailable} {
		if !outcomes[outcome] {
			t.Errorf("no producible report exercised outcome %s", outcome)
		}
	}
}

// Each field-presence, recognition and mapping rule rejects the Report
// that breaks it.
func TestValidRejectsInconsistentReports(t *testing.T) {
	cases := []struct {
		name   string
		report Report
	}{
		{"empty supplied role", Report{Outcome: RoleUnsupported}},
		{"whitespace supplied role", Report{SuppliedRole: " \t", Outcome: RoleUnsupported}},
		{"both agent identities", Report{SuppliedRole: "review", RequestedRole: role.Review, SelectedAgentID: agent.ClaudeCode, MappedAgentID: agent.ClaudeCode, Outcome: RoleSelected}},
		{"unknown outcome", Report{SuppliedRole: "review", RequestedRole: role.Review, SelectedAgentID: agent.ClaudeCode, Outcome: "approved"}},

		{"unsupported but recognized", Report{SuppliedRole: "review", Outcome: RoleUnsupported}},
		{"unsupported with requested role", Report{SuppliedRole: "architect", RequestedRole: role.Review, Outcome: RoleUnsupported}},
		{"unsupported with selected agent", Report{SuppliedRole: "architect", SelectedAgentID: agent.ClaudeCode, Outcome: RoleUnsupported}},
		{"unsupported with mapped agent", Report{SuppliedRole: "architect", MappedAgentID: agent.ClaudeCode, Outcome: RoleUnsupported}},

		{"selected but unrecognized", Report{SuppliedRole: "architect", RequestedRole: role.Review, SelectedAgentID: agent.ClaudeCode, Outcome: RoleSelected}},
		{"selected with another requested role", Report{SuppliedRole: "review", RequestedRole: role.Planning, SelectedAgentID: agent.ClaudeCode, Outcome: RoleSelected}},
		{"selected with the wrong agent", Report{SuppliedRole: "implementation", RequestedRole: role.Implementation, SelectedAgentID: agent.ClaudeCode, Outcome: RoleSelected}},
		{"selected without an agent", Report{SuppliedRole: "review", RequestedRole: role.Review, Outcome: RoleSelected}},
		{"selected with only a mapped agent", Report{SuppliedRole: "review", RequestedRole: role.Review, MappedAgentID: agent.ClaudeCode, Outcome: RoleSelected}},

		{"unavailable but unrecognized", Report{SuppliedRole: "architect", RequestedRole: role.Review, MappedAgentID: agent.ClaudeCode, Outcome: AgentUnavailable}},
		{"unavailable with the wrong agent", Report{SuppliedRole: "review", RequestedRole: role.Review, MappedAgentID: agent.CodexCLI, Outcome: AgentUnavailable}},
		{"unavailable with a selected agent", Report{SuppliedRole: "review", RequestedRole: role.Review, SelectedAgentID: agent.ClaudeCode, Outcome: AgentUnavailable}},
		{"inconsistent with an unmapped target", Report{SuppliedRole: "implementation", RequestedRole: role.Implementation, MappedAgentID: "not-a-real-agent", Outcome: SelectionInconsistent}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.report.Valid() {
				t.Fatalf("Valid() = true for %+v", tc.report)
			}
		})
	}
	// The same shapes with the mapped agent are accepted, so each
	// rejection above is caused by the rule it names.
	for _, report := range []Report{
		{SuppliedRole: "review", RequestedRole: role.Review, MappedAgentID: agent.ClaudeCode, Outcome: AgentUnavailable},
		{SuppliedRole: "implementation", RequestedRole: role.Implementation, MappedAgentID: agent.CodexCLI, Outcome: SelectionInconsistent},
	} {
		if !report.Valid() {
			t.Errorf("Valid() = false for consistent %+v", report)
		}
	}
}
