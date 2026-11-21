package adapters

import "github.com/germanmunozs/ai-dev/internal/execution"

// Codex invokes Codex CLI's documented non-interactive exec mode while
// leaving its native sandbox and approval behavior unchanged. The instruction
// follows "--", so text that starts with "-" is always the prompt and
// never one of Codex CLI's own options.
type Codex struct{}

func (Codex) Command(_ string, instruction string) (string, []string) {
	return "codex", []string{"exec", "--", instruction}
}

func (Codex) Classify(exitCode int) execution.Outcome {
	if exitCode == 0 {
		return execution.AgentSucceeded
	}
	return execution.AgentFailed
}
