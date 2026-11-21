// Package adapters contains vendor-specific coding-agent command adapters.
package adapters

import "github.com/germanmunozs/ai-dev/internal/execution"

// Claude invokes Claude Code's documented non-interactive print mode while
// leaving its native permissions and approval behavior unchanged. The
// instruction follows "--", so text that starts with "-" is always the
// prompt and never one of Claude Code's own options.
type Claude struct{}

func (Claude) Command(_ string, instruction string) (string, []string) {
	return "claude", []string{"--print", "--", instruction}
}

func (Claude) Classify(exitCode int) execution.Outcome {
	if exitCode == 0 {
		return execution.AgentSucceeded
	}
	return execution.AgentFailed
}
