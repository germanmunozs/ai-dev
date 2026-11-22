package checks

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/doctor"
)

func TestToolCheckPass(t *testing.T) {
	tests := []struct{ id, name, binary string }{
		{"git-tool", "Git", "git"},
		{"speckit-tool", "Spec Kit", "specify"},
		{"claude-code-tool", "Claude Code", "claude"},
		{"codex-cli-tool", "Codex CLI", "codex"},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			exec := &fakeExecutor{paths: map[string]string{tt.binary: "/bin/" + tt.binary}}
			check := NewToolCheck(tt.id, tt.name, tt.binary, exec)
			got := check.Run(context.Background())
			if check.ID() != tt.id || got.ID != tt.id || got.Name != tt.name || got.Area != "Environment" || got.Status != doctor.Pass {
				t.Fatalf("unexpected result: %#v", got)
			}
			if got.Explanation != "" || got.Remediation != "" {
				t.Fatalf("Pass diagnostics must be empty: %#v", got)
			}
		})
	}
}

func TestToolCheckFailuresAreActionableAndContainPanics(t *testing.T) {
	tests := []struct {
		name string
		exec *fakeExecutor
	}{
		{"not found", &fakeExecutor{}},
		{"lookup error", &fakeExecutor{lookErrs: map[string]error{"codex": errors.New("permission denied")}}},
		{"panic", &fakeExecutor{panicLookup: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewToolCheck("codex-cli-tool", "Codex CLI", "codex", tt.exec).Run(context.Background())
			if got.Status != doctor.Fail || got.Explanation == "" || got.Remediation == "" {
				t.Fatalf("unexpected result: %#v", got)
			}
			if !strings.Contains(got.Explanation, "Codex CLI") || !strings.Contains(got.Remediation, "codex") {
				t.Fatalf("diagnostics are not tool-specific: %#v", got)
			}
		})
	}
}
