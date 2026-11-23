package adapters

import (
	"reflect"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/execution"
)

// The two instruction forms of F013-FR-006 (data-model.md, Invocation).
func TestInvocationBuildsEitherInstructionForm(t *testing.T) {
	instruction := "review '$HOME' && echo nope\nthen explain = done ünïcode"
	tests := []struct {
		name       string
		invocation Invocation
		want       []string
	}{
		{"after end of options", Invocation{Executable: "codex", Mode: []string{"exec"}, Form: AfterEndOfOptions},
			[]string{"exec", "--", instruction}},
		{"after end of options, two mode arguments", Invocation{Executable: "kiro-cli", Mode: []string{"chat", "--no-interactive"}, Form: AfterEndOfOptions},
			[]string{"chat", "--no-interactive", "--", instruction}},
		{"attached option", Invocation{Executable: "gemini", Form: AttachedOption, Option: "--prompt"},
			[]string{"--prompt=" + instruction}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, args := tt.invocation.Command("/tmp/project with spaces", instruction)
			if name != tt.invocation.Executable {
				t.Fatalf("name = %q, want %q", name, tt.invocation.Executable)
			}
			if !reflect.DeepEqual(args, tt.want) {
				t.Fatalf("args = %#v, want %#v", args, tt.want)
			}
		})
	}
}

// Command returns a fresh slice, so a caller cannot change Mode through it.
func TestInvocationCommandDoesNotAliasMode(t *testing.T) {
	invocation := Invocation{Executable: "droid", Mode: []string{"exec"}, Form: AfterEndOfOptions}
	_, args := invocation.Command("/p", "x")
	args[0] = "changed"
	if invocation.Mode[0] != "exec" {
		t.Fatalf("mutating Command's result changed Mode to %q", invocation.Mode[0])
	}
}

func TestInvocationClassifiesExitCodes(t *testing.T) {
	invocation := Invocation{Executable: "qwen", Form: AttachedOption, Option: "--prompt"}
	if got := invocation.Classify(0); got != execution.AgentSucceeded {
		t.Fatalf("Classify(0) = %q", got)
	}
	for _, code := range []int{1, 2, 7, 130} {
		if got := invocation.Classify(code); got != execution.AgentFailed {
			t.Fatalf("Classify(%d) = %q", code, got)
		}
	}
}
