package checks

import (
	"context"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/doctor"
)

// One check for every catalog agent: it passes when any is installed and
// names the available ones, in catalog order, in its name; a passing check
// keeps explanation and remediation empty (F013-FR-009, 001 contract).
func TestCodingAgentCheck(t *testing.T) {
	tests := []struct {
		name        string
		exec        *fakeExecutor
		status      doctor.Status
		checkName   string
		explanation string
		remediation string
	}{
		{"none installed", &fakeExecutor{}, doctor.Fail, "Coding agent",
			"No supported coding agent was found on PATH.", "Install a supported coding agent; run ai-dev agents to see the list."},
		{"only a new agent", &fakeExecutor{paths: map[string]string{"vibe": "/bin/vibe"}}, doctor.Pass,
			"Coding agent: mistral-vibe", "", ""},
		{"several, in catalog order", &fakeExecutor{paths: map[string]string{"qwen": "/q", "claude": "/c", "droid": "/d"}}, doctor.Pass,
			"Coding agent: claude-code, factory-droid, qwen-code", "", ""},
		{"lookup panic", &fakeExecutor{panicLookup: true}, doctor.Fail, "Coding agent",
			"Coding agent lookup failed unexpectedly.", "Install a supported coding agent; run ai-dev agents to see the list."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := NewCodingAgentCheck(tt.exec)
			got := check.Run(context.Background())
			want := doctor.CheckResult{ID: "coding-agent-tool", Name: tt.checkName, Area: "Environment",
				Status: tt.status, Explanation: tt.explanation, Remediation: tt.remediation}
			if check.ID() != "coding-agent-tool" || got != want {
				t.Fatalf("got %#v, want %#v", got, want)
			}
		})
	}
}
