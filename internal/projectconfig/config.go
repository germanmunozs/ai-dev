// Package projectconfig reads, validates and encodes a project's Feature 011
// configuration, .ai-dev/agents.json. It is pure: it reads through fs.FS,
// writes nothing and starts nothing. The configuration arrives with cloned
// repositories, so it is untrusted input; it can only name catalog agents
// and the five roles (F011-FR-006).
package projectconfig

import (
	"slices"

	"github.com/germanmunozs/ai-dev/internal/agent"
	"github.com/germanmunozs/ai-dev/internal/role"
)

const (
	// Dir is the configuration directory, relative to the project root.
	Dir = ".ai-dev"
	// FileName is the configuration file, relative to the project root.
	FileName = ".ai-dev/agents.json"
	// SchemaVersion identifies the only supported file format.
	SchemaVersion = "ai-dev.agents/v1"
	// MaxSize is the largest file read, in bytes.
	MaxSize = 65536
)

// State is the result of loading the configuration.
type State uint8

const (
	Absent State = iota
	Valid
	Invalid
)

// Reason is one violation of F011-FR-005. Value is the untrusted value
// verbatim, or empty; renderers must quote it.
type Reason struct {
	Code  string
	Value string
}

var roleOrder = []role.Role{role.Specification, role.Planning, role.Implementation, role.Review, role.Remediation}

// Roles returns the five roles in configuration order.
func Roles() []role.Role { return slices.Clone(roleOrder) }

// Config is a validated configuration. Only New and Load build a non-zero
// Config, and both validate it, so a Config always holds catalog agents and
// one chosen agent per role.
type Config struct {
	agents []agent.ID // catalog order
	roles  []agent.ID // roleOrder
}

// New validates a configuration built from explicit choices. It returns the
// zero Config whenever there is a reason.
func New(agents []agent.ID, roles map[role.Role]agent.ID) (Config, []Reason) {
	names := make([]string, len(agents))
	for i, id := range agents {
		names[i] = string(id)
	}
	roleNames := make(map[string]string, len(roles))
	for r, id := range roles {
		roleNames[string(r)] = string(id)
	}
	return validate(names, roleNames, nil)
}

// validate applies the content rules in the contract's reason order, after
// any reasons already found (schema, type).
func validate(agents []string, roles map[string]string, reasons []Reason) (Config, []Reason) {
	if len(agents) == 0 {
		reasons = append(reasons, Reason{Code: "no_agents"})
	}
	seen := map[string]bool{}
	for _, id := range agents {
		if seen[id] {
			reasons = append(reasons, Reason{Code: "duplicate_agent", Value: id})
		}
		seen[id] = true
	}
	unknown := map[string]bool{}
	addUnknown := func(id string) {
		if !agent.Known(agent.ID(id)) && !unknown[id] {
			unknown[id] = true
			reasons = append(reasons, Reason{Code: "unknown_agent", Value: id})
		}
	}
	for _, id := range agents {
		addUnknown(id)
	}
	for _, r := range roleOrder {
		if id, ok := roles[string(r)]; ok {
			addUnknown(id)
		}
	}
	var extra []string
	for name := range roles {
		if _, ok := role.Recognized(name); !ok {
			extra = append(extra, name)
		}
	}
	slices.Sort(extra)
	for _, name := range extra {
		reasons = append(reasons, Reason{Code: "unknown_role", Value: name})
	}
	for _, r := range roleOrder {
		if _, ok := roles[string(r)]; !ok {
			reasons = append(reasons, Reason{Code: "missing_role", Value: string(r)})
		}
	}
	for _, r := range roleOrder {
		id, ok := roles[string(r)]
		if ok && agent.Known(agent.ID(id)) && !seen[id] {
			reasons = append(reasons, Reason{Code: "agent_not_chosen", Value: string(r) + "=" + id})
		}
	}
	if len(reasons) != 0 {
		return Config{}, reasons
	}
	var config Config
	for _, supported := range agent.Catalog() {
		if seen[string(supported.ID)] {
			config.agents = append(config.agents, supported.ID)
		}
	}
	for _, r := range roleOrder {
		config.roles = append(config.roles, agent.ID(roles[string(r)]))
	}
	return config, nil
}

// Agents returns the chosen agents in catalog order.
func (c Config) Agents() []agent.ID { return slices.Clone(c.agents) }

// Chosen reports whether id is a chosen agent.
func (c Config) Chosen(id agent.ID) bool { return slices.Contains(c.agents, id) }

// AgentFor returns the agent assigned to r.
func (c Config) AgentFor(r role.Role) (agent.ID, bool) {
	i := slices.Index(roleOrder, r)
	if i < 0 || i >= len(c.roles) {
		return "", false
	}
	return c.roles[i], true
}

// Equal reports whether two configurations have the same content.
func (c Config) Equal(other Config) bool {
	return slices.Equal(c.agents, other.agents) && slices.Equal(c.roles, other.roles)
}
