// Package selection is the policy boundary connecting development roles to
// canonical supported-agent identities. It deliberately knows about both
// role.Role and agent.ID, because resolving one into the other is its entire
// purpose.
package selection

import (
	"ai-dev/internal/agent"
	"ai-dev/internal/role"
)

// Outcome is one of the four canonical results of a Role Selection Request.
type Outcome string

const (
	RoleSelected          Outcome = "role_selected"
	RoleUnsupported       Outcome = "role_unsupported"
	AgentUnavailable      Outcome = "agent_unavailable"
	SelectionInconsistent Outcome = "selection_inconsistent"
)

// Report is the result of one recognized Role Selection Request, in both
// human- and machine-readable presentations. Per-outcome field presence:
//
//	role_selected:           SuppliedRole, RequestedRole, SelectedAgentID
//	role_unsupported:        SuppliedRole
//	agent_unavailable:       SuppliedRole, RequestedRole, MappedAgentID
//	selection_inconsistent:  SuppliedRole, RequestedRole, MappedAgentID
//
// SelectedAgentID and MappedAgentID are never both non-empty on the same
// Report.
type Report struct {
	SuppliedRole    string
	RequestedRole   role.Role
	SelectedAgentID agent.ID
	MappedAgentID   agent.ID
	Outcome         Outcome
}
