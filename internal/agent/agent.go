// Package agent defines the vendor-neutral supported-agent catalog.
package agent

// ID is the stable canonical identifier used for selection and reporting.
type ID string

const (
	ClaudeCode ID = "claude-code"
	CodexCLI   ID = "codex-cli"
)

// Agent describes one supported coding agent without execution semantics.
type Agent struct {
	ID           ID
	DisplayLabel string
	binary       string
}

// Availability is a point-in-time local executable discovery result.
type Availability uint8

const (
	Unavailable Availability = iota
	Available
)

func (a Availability) String() string {
	if a == Available {
		return "available"
	}
	return "unavailable"
}

// AgentStatus pairs a supported agent with its current availability.
type AgentStatus struct {
	Agent        Agent
	Availability Availability
}
