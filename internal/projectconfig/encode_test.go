package projectconfig

import (
	"reflect"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/agent"
	"github.com/germanmunozs/ai-dev/internal/role"
)

func builtInLike() map[role.Role]agent.ID {
	return map[role.Role]agent.ID{
		role.Specification:  agent.ClaudeCode,
		role.Planning:       agent.ClaudeCode,
		role.Implementation: agent.CodexCLI,
		role.Review:         agent.ClaudeCode,
		role.Remediation:    agent.CodexCLI,
	}
}

// Encode is the canonical form of contracts/config-file.md: fixed field
// order, agents in catalog order, roles in contract order, trailing newline
// (F011-FR-007, SC-006).
func TestEncodeIsCanonical(t *testing.T) {
	config, reasons := New([]agent.ID{agent.CodexCLI, agent.ClaudeCode}, builtInLike())
	if len(reasons) != 0 {
		t.Fatalf("New reasons: %v", reasons)
	}
	if got := string(config.Encode()); got != canonical {
		t.Fatalf("Encode:\n%s\nwant:\n%s", got, canonical)
	}
	loaded, state, reasons := Load(mapFS(string(config.Encode())))
	if state != Valid || len(reasons) != 0 || !loaded.Equal(config) {
		t.Fatalf("round trip: state=%v reasons=%v equal=%t", state, reasons, loaded.Equal(config))
	}
}

func TestEqual(t *testing.T) {
	a, _ := New([]agent.ID{agent.ClaudeCode, agent.CodexCLI}, builtInLike())
	b, _ := New([]agent.ID{agent.CodexCLI, agent.ClaudeCode}, builtInLike())
	if !a.Equal(b) {
		t.Fatal("same content in a different order is not Equal")
	}
	roles := builtInLike()
	roles[role.Review] = agent.CodexCLI
	c, _ := New([]agent.ID{agent.ClaudeCode, agent.CodexCLI}, roles)
	d, _ := New([]agent.ID{agent.ClaudeCode, agent.CodexCLI, agent.CodexCLI}, builtInLike())
	if a.Equal(c) {
		t.Fatal("different roles are Equal")
	}
	if len(d.Agents()) != 0 {
		t.Fatal("New returned a usable Config despite reasons")
	}
}

// New applies the same content rules as Load (F011-FR-005).
func TestNewRejectsWithTheSameReasons(t *testing.T) {
	roles := builtInLike()
	roles["deploy"] = agent.ClaudeCode
	delete(roles, role.Remediation)
	roles[role.Specification] = "gemini"
	roles[role.Review] = agent.CodexCLI
	_, reasons := New([]agent.ID{agent.ClaudeCode, agent.ClaudeCode}, roles)
	want := []Reason{
		{Code: "duplicate_agent", Value: "claude-code"},
		{Code: "unknown_agent", Value: "gemini"},
		{Code: "unknown_role", Value: "deploy"},
		{Code: "missing_role", Value: "remediation"},
		{Code: "agent_not_chosen", Value: "implementation=codex-cli"},
		{Code: "agent_not_chosen", Value: "review=codex-cli"},
	}
	if !reflect.DeepEqual(reasons, want) {
		t.Fatalf("reasons = %#v\nwant %#v", reasons, want)
	}
	if _, reasons := New(nil, builtInLike()); len(reasons) == 0 || reasons[0].Code != "no_agents" {
		t.Fatalf("no agents: %v", reasons)
	}
}

func TestAccessors(t *testing.T) {
	config, _ := New([]agent.ID{agent.ClaudeCode}, map[role.Role]agent.ID{
		role.Specification: agent.ClaudeCode, role.Planning: agent.ClaudeCode, role.Implementation: agent.ClaudeCode,
		role.Review: agent.ClaudeCode, role.Remediation: agent.ClaudeCode,
	})
	if !config.Chosen(agent.ClaudeCode) || config.Chosen(agent.CodexCLI) {
		t.Fatal("Chosen")
	}
	if _, ok := config.AgentFor("deploy"); ok {
		t.Fatal("AgentFor accepted an unknown role")
	}
	agents := config.Agents()
	agents[0] = "changed"
	if config.Agents()[0] != agent.ClaudeCode {
		t.Fatal("Agents() exposes internal state")
	}
	if got := Roles(); !reflect.DeepEqual(got, []role.Role{role.Specification, role.Planning, role.Implementation, role.Review, role.Remediation}) {
		t.Fatalf("Roles() = %v", got)
	}
}
