// Package adapters contains vendor-specific coding-agent command adapters.
package adapters

import "ai-dev/internal/execution"

// Claude invokes Claude Code's documented non-interactive print mode while
// leaving its native permissions and approval behavior unchanged.
type Claude struct{}

func (Claude) Command(_ string, instruction string) (string, []string) {
	return "claude", []string{"--print", instruction}
}

func (Claude) Classify(exitCode int) execution.Outcome {
	if exitCode == 0 {
		return execution.AgentSucceeded
	}
	return execution.AgentFailed
}
