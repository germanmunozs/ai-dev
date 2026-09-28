// Package execution owns the vendor-neutral coding-agent execution model.
package execution

import "ai-dev/internal/agent"

// Request identifies one agent and carries one already-resolved instruction.
type Request struct {
	AgentID     agent.ID
	Instruction string
}
