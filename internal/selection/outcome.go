// Package selection is the policy boundary connecting development roles to
// canonical supported-agent identities. It deliberately knows about both
// role.Role and agent.ID, because resolving one into the other is its entire
// purpose.
package selection

import (
	"github.com/germanmunozs/ai-dev/internal/agent"
	"github.com/germanmunozs/ai-dev/internal/role"
)

// Outcome is one of the canonical results of a Role Selection Request: the
// four of 003, plus configuration_invalid (F011 Amendment A1).
type Outcome string

const (
	RoleSelected          Outcome = "role_selected"
	RoleUnsupported       Outcome = "role_unsupported"
	AgentUnavailable      Outcome = "agent_unavailable"
	SelectionInconsistent Outcome = "selection_inconsistent"
	ConfigurationInvalid  Outcome = "configuration_invalid"
)

// Report is the result of one recognized Role Selection Request, in both
// human- and machine-readable presentations. Per-outcome field presence:
//
//	role_selected:           SuppliedRole, RequestedRole, SelectedAgentID
//	role_unsupported:        SuppliedRole
//	agent_unavailable:       SuppliedRole, RequestedRole, MappedAgentID
//	selection_inconsistent:  SuppliedRole, RequestedRole, MappedAgentID
//	configuration_invalid:   SuppliedRole, RequestedRole
//
// Source is always present. selection_inconsistent only arises under the
// built-in assignment, configuration_invalid only under a project one.
//
// SelectedAgentID and MappedAgentID are never both non-empty on the same
// Report.
type Report struct {
	SuppliedRole    string
	RequestedRole   role.Role
	SelectedAgentID agent.ID
	MappedAgentID   agent.ID
	Outcome         Outcome
	Source          Source
}
