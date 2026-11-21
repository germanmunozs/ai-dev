package adapters

import (
	"reflect"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/execution"
)

func TestClaudeAdapterBuildsPermissionPreservingOneShotCommand(t *testing.T) {
	instruction := "review '$HOME' && echo nope\nthen explain"
	name, args := (Claude{}).Command("/tmp/project with spaces", instruction)
	if name != "claude" {
		t.Fatalf("name = %q, want claude", name)
	}
	if want := []string{"--print", "--", instruction}; !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
	for _, forbidden := range []string{
		"--dangerously-skip-permissions", "--allow-dangerously-skip-permissions",
		"--permission-mode", "--permission-prompts", "--fallback-model",
		"--mcp-config", "--bg", "--background",
	} {
		for _, arg := range args {
			if arg == forbidden {
				t.Fatalf("args contain forbidden flag %q", forbidden)
			}
		}
	}
}

func TestClaudeAdapterClassifiesNormalExitCodes(t *testing.T) {
	adapter := Claude{}
	if got := adapter.Classify(0); got != execution.AgentSucceeded {
		t.Fatalf("Classify(0) = %q", got)
	}
	if got := adapter.Classify(7); got != execution.AgentFailed {
		t.Fatalf("Classify(7) = %q", got)
	}
}
