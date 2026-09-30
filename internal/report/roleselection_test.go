package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"ai-dev/internal/agent"
	"ai-dev/internal/role"
	"ai-dev/internal/selection"
)

func TestWriteRoleSelectionTextAndJSONForRoleSelected(t *testing.T) {
	r := selection.Report{
		SuppliedRole:    "implementation",
		RequestedRole:   role.Implementation,
		SelectedAgentID: agent.CodexCLI,
		Outcome:         selection.RoleSelected,
	}

	var text bytes.Buffer
	if err := WriteRoleSelectionText(&text, r); err != nil {
		t.Fatalf("WriteRoleSelectionText: %v", err)
	}
	if got, want := text.String(), "ai-dev: outcome=role_selected role=implementation agent=codex-cli\n"; got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}

	var jsonBuf bytes.Buffer
	if err := WriteRoleSelectionJSON(&jsonBuf, r); err != nil {
		t.Fatalf("WriteRoleSelectionJSON: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(jsonBuf.Bytes(), &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{"supplied_role", "requested_role", "selected_agent_id", "outcome"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("missing key %q in %v", key, decoded)
		}
	}
	if _, ok := decoded["mapped_agent_id"]; ok {
		t.Fatalf("unexpected mapped_agent_id key in %v", decoded)
	}
}

func TestWriteRoleSelectionTextAndJSONForRoleUnsupported(t *testing.T) {
	r := selection.Report{SuppliedRole: "Implementation", Outcome: selection.RoleUnsupported}

	var text bytes.Buffer
	if err := WriteRoleSelectionText(&text, r); err != nil {
		t.Fatalf("WriteRoleSelectionText: %v", err)
	}
	if got, want := text.String(), `ai-dev: outcome=role_unsupported role="Implementation"`+"\n"; got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
	if strings.Contains(text.String(), "agent=") {
		t.Fatalf("role_unsupported text contains agent=: %q", text.String())
	}

	var jsonBuf bytes.Buffer
	if err := WriteRoleSelectionJSON(&jsonBuf, r); err != nil {
		t.Fatalf("WriteRoleSelectionJSON: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(jsonBuf.Bytes(), &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{"supplied_role", "outcome"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("missing key %q in %v", key, decoded)
		}
	}
	for _, key := range []string{"requested_role", "selected_agent_id", "mapped_agent_id"} {
		if _, ok := decoded[key]; ok {
			t.Fatalf("unexpected key %q in %v", key, decoded)
		}
	}
}

func TestWriteRoleSelectionTextAndJSONForAgentUnavailableAndSelectionInconsistent(t *testing.T) {
	tests := []struct {
		name    string
		outcome selection.Outcome
	}{
		{"agent_unavailable", selection.AgentUnavailable},
		{"selection_inconsistent", selection.SelectionInconsistent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := selection.Report{
				SuppliedRole:  "implementation",
				RequestedRole: role.Implementation,
				MappedAgentID: agent.CodexCLI,
				Outcome:       tt.outcome,
			}

			var text bytes.Buffer
			if err := WriteRoleSelectionText(&text, r); err != nil {
				t.Fatalf("WriteRoleSelectionText: %v", err)
			}
			wantText := "ai-dev: outcome=" + string(tt.outcome) + " role=implementation agent=codex-cli\n"
			if text.String() != wantText {
				t.Fatalf("text = %q, want %q", text.String(), wantText)
			}

			var jsonBuf bytes.Buffer
			if err := WriteRoleSelectionJSON(&jsonBuf, r); err != nil {
				t.Fatalf("WriteRoleSelectionJSON: %v", err)
			}
			var decoded map[string]any
			if err := json.Unmarshal(jsonBuf.Bytes(), &decoded); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if decoded["mapped_agent_id"] != "codex-cli" {
				t.Fatalf("mapped_agent_id = %v, want codex-cli", decoded["mapped_agent_id"])
			}
			if _, ok := decoded["selected_agent_id"]; ok {
				t.Fatalf("unexpected selected_agent_id in %v", decoded)
			}
		})
	}
}

func TestWriteRoleSelectionRejectsInvalidReportsBeforeWriting(t *testing.T) {
	tests := []struct {
		name string
		r    selection.Report
	}{
		{
			name: "role_selected missing SelectedAgentID",
			r:    selection.Report{SuppliedRole: "implementation", RequestedRole: role.Implementation, Outcome: selection.RoleSelected},
		},
		{
			name: "role_unsupported carries RequestedRole",
			r:    selection.Report{SuppliedRole: "nonsense", RequestedRole: role.Implementation, Outcome: selection.RoleUnsupported},
		},
		{
			name: "role_unsupported carries SelectedAgentID",
			r:    selection.Report{SuppliedRole: "nonsense", SelectedAgentID: agent.CodexCLI, Outcome: selection.RoleUnsupported},
		},
		{
			name: "agent_unavailable carries SelectedAgentID",
			r: selection.Report{
				SuppliedRole: "implementation", RequestedRole: role.Implementation,
				SelectedAgentID: agent.CodexCLI, Outcome: selection.AgentUnavailable,
			},
		},
		{
			name: "selection_inconsistent carries SelectedAgentID",
			r: selection.Report{
				SuppliedRole: "implementation", RequestedRole: role.Implementation,
				SelectedAgentID: agent.CodexCLI, Outcome: selection.SelectionInconsistent,
			},
		},
		{
			name: "both SelectedAgentID and MappedAgentID non-empty",
			r: selection.Report{
				SuppliedRole: "implementation", RequestedRole: role.Implementation,
				SelectedAgentID: agent.CodexCLI, MappedAgentID: agent.ClaudeCode, Outcome: selection.RoleSelected,
			},
		},
		{
			name: "unknown outcome",
			r:    selection.Report{SuppliedRole: "implementation", Outcome: selection.Outcome("bogus")},
		},
		{
			name: "whitespace-only supplied role on role_unsupported",
			r:    selection.Report{SuppliedRole: "   ", Outcome: selection.RoleUnsupported},
		},
		{
			name: "empty supplied role on an otherwise valid role_selected report",
			r:    selection.Report{RequestedRole: role.Implementation, SelectedAgentID: agent.CodexCLI, Outcome: selection.RoleSelected},
		},
		{
			name: "empty supplied role on role_unsupported",
			r:    selection.Report{Outcome: selection.RoleUnsupported},
		},
		{
			name: "noncanonical requested role",
			r: selection.Report{
				SuppliedRole: "bogus-role", RequestedRole: role.Role("bogus-role"),
				SelectedAgentID: agent.ClaudeCode, Outcome: selection.RoleSelected,
			},
		},
		{
			name: "inconsistent supplied and requested roles",
			r: selection.Report{
				SuppliedRole: "implementation", RequestedRole: role.Specification,
				SelectedAgentID: agent.ClaudeCode, Outcome: selection.RoleSelected,
			},
		},
		{
			name: "incorrect selected agent for a canonical role",
			r: selection.Report{
				SuppliedRole: "implementation", RequestedRole: role.Implementation,
				SelectedAgentID: agent.ClaudeCode, Outcome: selection.RoleSelected,
			},
		},
		{
			name: "incorrect mapped agent for agent_unavailable",
			r: selection.Report{
				SuppliedRole: "implementation", RequestedRole: role.Implementation,
				MappedAgentID: agent.ClaudeCode, Outcome: selection.AgentUnavailable,
			},
		},
		{
			name: "incorrect mapped agent for selection_inconsistent",
			r: selection.Report{
				SuppliedRole: "implementation", RequestedRole: role.Implementation,
				MappedAgentID: agent.ClaudeCode, Outcome: selection.SelectionInconsistent,
			},
		},
		{
			name: "canonical supplied role represented as role_unsupported",
			r:    selection.Report{SuppliedRole: "implementation", Outcome: selection.RoleUnsupported},
		},
		{
			name: "agent_unavailable missing MappedAgentID",
			r:    selection.Report{SuppliedRole: "implementation", RequestedRole: role.Implementation, Outcome: selection.AgentUnavailable},
		},
		{
			name: "selection_inconsistent missing MappedAgentID",
			r:    selection.Report{SuppliedRole: "implementation", RequestedRole: role.Implementation, Outcome: selection.SelectionInconsistent},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/text", func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteRoleSelectionText(&buf, tt.r); err == nil {
				t.Fatalf("WriteRoleSelectionText returned nil error for invalid report %+v", tt.r)
			}
			if buf.Len() != 0 {
				t.Fatalf("WriteRoleSelectionText wrote %d bytes for invalid report %+v", buf.Len(), tt.r)
			}
		})
		t.Run(tt.name+"/json", func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteRoleSelectionJSON(&buf, tt.r); err == nil {
				t.Fatalf("WriteRoleSelectionJSON returned nil error for invalid report %+v", tt.r)
			}
			if buf.Len() != 0 {
				t.Fatalf("WriteRoleSelectionJSON wrote %d bytes for invalid report %+v", buf.Len(), tt.r)
			}
		})
	}
}
