package selection

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"ai-dev/internal/agent"
	"ai-dev/internal/role"
)

type fakeExecutor struct {
	paths   map[string]string
	lookups []string
}

func (e *fakeExecutor) LookPath(name string) (string, error) {
	e.lookups = append(e.lookups, name)
	if path := e.paths[name]; path != "" {
		return path, nil
	}
	return "", errors.New("not found")
}

func (e *fakeExecutor) Run(context.Context, string, ...string) (string, error) {
	return "", errors.New("Select must never execute a process")
}

func bothAgentsAvailable() *fakeExecutor {
	return &fakeExecutor{paths: map[string]string{"claude": "/tools/claude", "codex": "/tools/codex"}}
}

func TestSelectorReportsRoleSelectedForEveryCanonicalRoleWithAnAvailableAgent(t *testing.T) {
	tests := []struct {
		suppliedRole string
		wantAgent    agent.ID
	}{
		{"specification", agent.ClaudeCode},
		{"planning", agent.ClaudeCode},
		{"implementation", agent.CodexCLI},
		{"review", agent.ClaudeCode},
		{"remediation", agent.CodexCLI},
	}
	for _, tt := range tests {
		t.Run(tt.suppliedRole, func(t *testing.T) {
			selector := NewSelector(agent.NewRegistry(bothAgentsAvailable()))
			got := selector.Select(tt.suppliedRole)
			if got.Outcome != RoleSelected {
				t.Fatalf("Outcome = %v, want %v", got.Outcome, RoleSelected)
			}
			if got.SelectedAgentID != tt.wantAgent {
				t.Fatalf("SelectedAgentID = %v, want %v", got.SelectedAgentID, tt.wantAgent)
			}
			if got.MappedAgentID != "" {
				t.Fatalf("MappedAgentID = %v, want empty", got.MappedAgentID)
			}
			if got.RequestedRole != role.Role(tt.suppliedRole) {
				t.Fatalf("RequestedRole = %v, want %v", got.RequestedRole, tt.suppliedRole)
			}
			if got.SuppliedRole != tt.suppliedRole {
				t.Fatalf("SuppliedRole = %v, want %v", got.SuppliedRole, tt.suppliedRole)
			}
		})
	}
}

func TestSelectorSelectIsDeterministic(t *testing.T) {
	selector := NewSelector(agent.NewRegistry(bothAgentsAvailable()))
	first := selector.Select("implementation")
	second := selector.Select("implementation")
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("first = %#v, second = %#v", first, second)
	}
}

func TestSelectorReportsRoleUnsupportedWithoutAnyRegistryAccess(t *testing.T) {
	tests := []string{"Implementation", "nonsense"}
	for _, suppliedRole := range tests {
		t.Run(suppliedRole, func(t *testing.T) {
			executor := &fakeExecutor{}
			selector := NewSelector(agent.NewRegistry(executor))
			got := selector.Select(suppliedRole)
			if got.Outcome != RoleUnsupported {
				t.Fatalf("Outcome = %v, want %v", got.Outcome, RoleUnsupported)
			}
			if got.SuppliedRole != suppliedRole {
				t.Fatalf("SuppliedRole = %q, want %q", got.SuppliedRole, suppliedRole)
			}
			if got.RequestedRole != "" || got.SelectedAgentID != "" || got.MappedAgentID != "" {
				t.Fatalf("unexpected non-empty fields: %#v", got)
			}
			if len(executor.lookups) != 0 {
				t.Fatalf("LookPath calls = %v, want none", executor.lookups)
			}
		})
	}
}

func TestResolveAgainstRegistryReportsAgentUnavailable(t *testing.T) {
	executor := &fakeExecutor{paths: map[string]string{"claude": "/tools/claude"}}
	got := resolveAgainstRegistry("implementation", role.Implementation, agent.CodexCLI, agent.NewRegistry(executor))
	if got.Outcome != AgentUnavailable {
		t.Fatalf("Outcome = %v, want %v", got.Outcome, AgentUnavailable)
	}
	if got.MappedAgentID != agent.CodexCLI {
		t.Fatalf("MappedAgentID = %v, want %v", got.MappedAgentID, agent.CodexCLI)
	}
	if got.SelectedAgentID != "" {
		t.Fatalf("SelectedAgentID = %v, want empty", got.SelectedAgentID)
	}
}

func TestResolveAgainstRegistryReportsSelectionInconsistentForAnUnregisteredTarget(t *testing.T) {
	got := resolveAgainstRegistry(
		"implementation",
		role.Implementation,
		agent.ID("not-a-real-agent"),
		agent.NewRegistry(bothAgentsAvailable()),
	)
	if got.Outcome != SelectionInconsistent {
		t.Fatalf("Outcome = %v, want %v", got.Outcome, SelectionInconsistent)
	}
	if got.MappedAgentID != agent.ID("not-a-real-agent") {
		t.Fatalf("MappedAgentID = %v, want not-a-real-agent", got.MappedAgentID)
	}
	if got.SelectedAgentID != "" {
		t.Fatalf("SelectedAgentID = %v, want empty", got.SelectedAgentID)
	}
}

func TestMappedAgentProductionEntriesAllResolveAgainstTheRealRegistry(t *testing.T) {
	roles := []role.Role{role.Specification, role.Planning, role.Implementation, role.Review, role.Remediation}
	statuses := agent.NewRegistry(&fakeExecutor{}).List()
	for _, r := range roles {
		t.Run(string(r), func(t *testing.T) {
			target, ok := mappedAgent(r)
			if !ok {
				t.Fatalf("mappedAgent(%v) reported no entry", r)
			}
			found := false
			for _, status := range statuses {
				if status.Agent.ID == target {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("mappedAgent(%v) = %v is not present in the real registry", r, target)
			}
		})
	}
}
