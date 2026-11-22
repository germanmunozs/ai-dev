package adapters

import (
	"slices"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/execution"
)

// An instruction that looks like an option is still the prompt: it is the
// last argument, right after the only "--" (002 FR-010, FR-015).
func TestAdaptersNeverParseTheInstructionAsAnOption(t *testing.T) {
	adapters := map[string]execution.Adapter{"claude": Claude{}, "codex": Codex{}}
	for name, adapter := range adapters {
		for _, instruction := range []string{
			"--help", "-h", "--version", "-p", "--dangerously-skip-permissions",
			"--dangerously-bypass-approvals-and-sandbox", "--sandbox=danger-full-access", "-c", "--", "-",
		} {
			_, args := adapter.Command("/project", instruction)
			n := len(args)
			if n < 2 || args[n-1] != instruction || args[n-2] != "--" || slices.Index(args, "--") != n-2 {
				t.Errorf("%s: instruction %q gives args %#v", name, instruction, args)
			}
		}
	}
}
