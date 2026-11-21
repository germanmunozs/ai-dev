package selection

import (
	"github.com/germanmunozs/ai-dev/internal/agent"
	"github.com/germanmunozs/ai-dev/internal/projectconfig"
	"github.com/germanmunozs/ai-dev/internal/role"
)

// Source names where the role assignment in effect comes from (F011-FR-017).
type Source string

const (
	SourceBuiltIn Source = "built_in"
	SourceProject Source = "project"
)

// Assignment is the role assignment in effect (F011, Amendment A1 to 003).
// It has exactly three forms: the built-in mapping, a validated project
// configuration, or an invalid project configuration. Only a
// projectconfig.Config, which only validation can build, can replace the
// built-in mapping, so no caller can supply an unvalidated policy.
type Assignment struct {
	source  Source
	config  projectconfig.Config
	invalid bool
}

// BuiltIn is the fixed 003 FR-003 mapping, used without a configuration.
func BuiltIn() Assignment { return Assignment{source: SourceBuiltIn} }

// FromProject uses a validated project configuration. A zero Config was
// never validated and is treated as invalid.
func FromProject(config projectconfig.Config) Assignment {
	if len(config.Agents()) == 0 {
		return InvalidProject()
	}
	return Assignment{source: SourceProject, config: config}
}

// InvalidProject resolves every recognized role to configuration_invalid,
// never to the built-in mapping (F011-FR-016).
func InvalidProject() Assignment { return Assignment{source: SourceProject, invalid: true} }

// Source reports where the assignment comes from.
func (a Assignment) Source() Source { return a.source }

func (a Assignment) target(r role.Role) (agent.ID, bool) {
	if a.source == SourceProject {
		return a.config.AgentFor(r)
	}
	return mappedAgent(r)
}

// AgentFor returns the agent the assignment gives r, for reporting the
// assignment in effect (F011-FR-009; A1 permits viewing it). An invalid
// project assignment gives no agent.
func (a Assignment) AgentFor(r role.Role) (agent.ID, bool) {
	if a.invalid {
		return "", false
	}
	return a.target(r)
}
