package adapters

import "github.com/germanmunozs/ai-dev/internal/execution"

// Form is how an agent receives the instruction so that its command-line
// parser can never read it as one of its own options (002 FR-010,
// F013-FR-006).
type Form uint8

const (
	// AfterEndOfOptions passes the instruction as the last argument, right
	// after an end-of-options marker: <mode…> -- <instruction>.
	AfterEndOfOptions Form = iota
	// AttachedOption passes the instruction as the value of the agent's
	// prompt option within one argument: <mode…> --prompt=<instruction>.
	// Used only where the agent's parser was verified to take everything
	// after the "=" as the value, including text that starts with "-".
	AttachedOption
)

// Invocation is one agent's verified non-interactive command: the mode
// selector and the instruction, and nothing that relaxes the agent's
// approvals, permissions, trust or sandbox (F013-FR-007).
type Invocation struct {
	Executable string
	Mode       []string
	Form       Form
	Option     string
}

func (i Invocation) Command(_ string, instruction string) (string, []string) {
	args := append([]string(nil), i.Mode...)
	if i.Form == AttachedOption {
		return i.Executable, append(args, i.Option+"="+instruction)
	}
	return i.Executable, append(args, "--", instruction)
}

func (Invocation) Classify(exitCode int) execution.Outcome {
	if exitCode == 0 {
		return execution.AgentSucceeded
	}
	return execution.AgentFailed
}
