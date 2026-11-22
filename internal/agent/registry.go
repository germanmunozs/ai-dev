package agent

import "github.com/germanmunozs/ai-dev/internal/system"

var supportedAgents = []Agent{
	{ID: ClaudeCode, DisplayLabel: "Claude Code", binary: "claude"},
	{ID: CodexCLI, DisplayLabel: "OpenAI Codex CLI", binary: "codex"},
}

// Registry enumerates the fixed supported-agent set in stable report order.
type Registry struct {
	agents   []Agent
	executor system.Executor
}

func NewRegistry(executor system.Executor) Registry {
	return Registry{agents: supportedAgents, executor: executor}
}

// List discovers each fixed agent through executable lookup only.
func (r Registry) List() []AgentStatus {
	statuses := make([]AgentStatus, 0, len(r.agents))
	for _, supported := range r.agents {
		availability := Unavailable
		if _, err := r.executor.LookPath(supported.binary); err == nil {
			availability = Available
		}
		statuses = append(statuses, AgentStatus{Agent: supported, Availability: availability})
	}
	return statuses
}

// Resolve finds a supported agent by canonical ID without probing availability.
func (r Registry) Resolve(id ID) (Agent, bool) {
	for _, supported := range r.agents {
		if supported.ID == id {
			return supported, true
		}
	}
	return Agent{}, false
}

// Catalog returns a copy of the fixed supported-agent set in report order.
// It probes nothing, so callers can validate identifiers without an executor.
func Catalog() []Agent {
	return append([]Agent(nil), supportedAgents...)
}

// Known reports whether id is exactly, case-sensitively, a catalog identifier.
func Known(id ID) bool {
	for _, supported := range supportedAgents {
		if supported.ID == id {
			return true
		}
	}
	return false
}
