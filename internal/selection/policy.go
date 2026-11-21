package selection

import (
	"github.com/germanmunozs/ai-dev/internal/agent"
	"github.com/germanmunozs/ai-dev/internal/role"
)

// mappedAgent returns the fixed production mapping's target agent identity
// for a canonical role. This is the complete FR-003 table, expressed as code,
// not data — there is no map, no slice, no struct literal that could be
// mutated, and no constructor or accessor that exposes it for reading or
// writing from outside this file.
func mappedAgent(r role.Role) (agent.ID, bool) {
	switch r {
	case role.Specification:
		return agent.ClaudeCode, true
	case role.Planning:
		return agent.ClaudeCode, true
	case role.Implementation:
		return agent.CodexCLI, true
	case role.Review:
		return agent.ClaudeCode, true
	case role.Remediation:
		return agent.CodexCLI, true
	default:
		return "", false
	}
}
