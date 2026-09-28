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
	wantLabels := []string{"Claude Code", "OpenAI Codex CLI"}
	for i, status := range statuses {
		if status.Agent.DisplayLabel != wantLabels[i] {
			t.Errorf("status %d label = %q, want %q", i, status.Agent.DisplayLabel, wantLabels[i])
		}
		if string(status.Agent.ID) == status.Agent.DisplayLabel {
			t.Errorf("status %d uses display label as ID", i)
		}
	}
}
