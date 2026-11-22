package selection

import (
	"github.com/germanmunozs/ai-dev/internal/agent"
	"github.com/germanmunozs/ai-dev/internal/role"
)

// Selector resolves one Role Selection Request against the role assignment
// in effect and a supported-agent registry. It never launches, authenticates,
// probes the network, or otherwise invokes any coding agent — it holds no
// system.Executor, no system.Launcher, and no execution.Runner, so it has no
// path to do so.
type Selector struct {
	registry   agent.Registry
	assignment Assignment
}

// NewSelector builds a Selector that resolves through the fixed, unexported
// mappedAgent function, the built-in assignment.
func NewSelector(registry agent.Registry) Selector {
	return NewSelectorFor(registry, BuiltIn())
}

// NewSelectorFor builds a Selector for the assignment in effect. The only
// substitute for the built-in mapping is a validated project configuration
// (F011 Amendment A1); see Assignment.
func NewSelectorFor(registry agent.Registry, assignment Assignment) Selector {
	return Selector{registry: registry, assignment: assignment}
}

// Select resolves exactly one supplied role value (verbatim, as the caller
// supplied it) to a Role Selection Report.
func (s Selector) Select(suppliedRole string) Report {
	source := s.assignment.Source()
	requested, ok := role.Recognized(suppliedRole)
	if !ok {
		return Report{SuppliedRole: suppliedRole, Outcome: RoleUnsupported, Source: source}
	}
	target, ok := s.assignment.target(requested)
	if s.assignment.invalid || !ok {
		return Report{SuppliedRole: suppliedRole, RequestedRole: requested, Outcome: ConfigurationInvalid, Source: SourceProject}
	}
	report := resolveAgainstRegistry(suppliedRole, requested, target, s.registry)
	report.Source = source
	return report
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
