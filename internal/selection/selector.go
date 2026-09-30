package selection

import (
	"ai-dev/internal/agent"
	"ai-dev/internal/role"
)

// Selector resolves one Role Selection Request against the fixed production
// mapping and a supported-agent registry. It never launches, authenticates,
// probes the network, or otherwise invokes any coding agent — it holds no
// system.Executor, no system.Launcher, and no execution.Runner, so it has no
// path to do so.
type Selector struct {
	registry agent.Registry
}

// NewSelector builds a Selector against the given registry. Select always
// resolves through the fixed, unexported mappedAgent function — there is no
// way to supply a substitute policy through this or any other exported
// constructor.
func NewSelector(registry agent.Registry) Selector {
	return Selector{registry: registry}
}

// Select resolves exactly one supplied role value (verbatim, as the caller
// supplied it) to a Role Selection Report.
func (s Selector) Select(suppliedRole string) Report {
	requested, ok := role.Recognized(suppliedRole)
	if !ok {
		return Report{SuppliedRole: suppliedRole, Outcome: RoleUnsupported}
	}
	target, _ := mappedAgent(requested)
	return resolveAgainstRegistry(suppliedRole, requested, target, s.registry)
}

// resolveAgainstRegistry determines the outcome for one already-recognized
// role and its mapped target agent identity, given the current registry
// state. It takes the target explicitly, so it can be tested with a
// deliberately unsupported identity without ever touching mappedAgent or
// constructing a Selector with a substitute policy.
func resolveAgainstRegistry(suppliedRole string, requested role.Role, target agent.ID, registry agent.Registry) Report {
	for _, status := range registry.List() {
		if status.Agent.ID != target {
			continue
		}
		if status.Availability == agent.Unavailable {
			return Report{SuppliedRole: suppliedRole, RequestedRole: requested, MappedAgentID: target, Outcome: AgentUnavailable}
		}
		return Report{SuppliedRole: suppliedRole, RequestedRole: requested, SelectedAgentID: target, Outcome: RoleSelected}
	}
	return Report{SuppliedRole: suppliedRole, RequestedRole: requested, MappedAgentID: target, Outcome: SelectionInconsistent}
}
