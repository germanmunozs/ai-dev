package adapters

import (
	"reflect"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/execution"
)

func TestCodexAdapterBuildsPermissionPreservingOneShotCommand(t *testing.T) {
	instruction := "review '$HOME' && echo nope\nthen explain"
	name, args := (Codex{}).Command("/tmp/project with spaces", instruction)
	if name != "codex" {
		t.Fatalf("name = %q, want codex", name)
	}
	if want := []string{"exec", "--", instruction}; !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
	for _, forbidden := range []string{
		"--approve-for-me", "--dangerously-bypass-approvals-and-sandbox",
		"--dangerously-bypass-hook-trust", "--sandbox", "--ask-for-approval",
		"--config", "--enable", "--remote", "--worktree",
	} {
		for _, arg := range args {
			if arg == forbidden {
				t.Fatalf("args contain forbidden flag %q", forbidden)
			}
		}
	}
}

func TestCodexAdapterClassifiesNormalExitCodes(t *testing.T) {
	adapter := Codex{}
	if got := adapter.Classify(0); got != execution.AgentSucceeded {
		t.Fatalf("Classify(0) = %q", got)
	}
	if got := adapter.Classify(7); got != execution.AgentFailed {
		t.Fatalf("Classify(7) = %q", got)
	}
}
