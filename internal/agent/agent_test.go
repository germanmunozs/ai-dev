package agent

import "testing"

func TestCanonicalIDsAreStable(t *testing.T) {
	if ClaudeCode != ID("claude-code") {
		t.Fatalf("ClaudeCode = %q", ClaudeCode)
	}
	if CodexCLI != ID("codex-cli") {
		t.Fatalf("CodexCLI = %q", CodexCLI)
	}
}

func TestCanonicalIDsRemainSeparateFromDisplayLabels(t *testing.T) {
	registry := NewRegistry(&recordingExecutor{paths: map[string]string{}})
	statuses := registry.List()
	if len(statuses) != len(wantCatalog) {
		t.Fatalf("List() has %d agents, want %d", len(statuses), len(wantCatalog))
	}
	for i, status := range statuses {
		if status.Agent.DisplayLabel != wantCatalog[i].DisplayLabel {
			t.Errorf("status %d label = %q, want %q", i, status.Agent.DisplayLabel, wantCatalog[i].DisplayLabel)
		}
		if string(status.Agent.ID) == status.Agent.DisplayLabel {
			t.Errorf("status %d uses display label as ID", i)
		}
	}
}
