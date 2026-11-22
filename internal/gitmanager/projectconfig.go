package gitmanager

// ProjectConfigSchema is the schema of .ai-dev/config.json (PCF-01).
const ProjectConfigSchema = "ai-dev.project-config/v1"

// projectConfig is a parsed valid .ai-dev/config.json.
type projectConfig struct {
	id       ProjectID
	strategy Strategy
	dialect  GitFlowDialect
	branches map[string]string
}

var branchParameterNames = []string{BranchInitial, BranchPrimary, BranchTrunk, BranchProduction, BranchDevelopment}

// classifyProjectConfig decides whether existing .ai-dev/config.json bytes
// are a valid project configuration and returns its project ID.
func classifyProjectConfig(data []byte) (ProjectID, bool) {
	config, valid := parseProjectConfig(data)
	return config.id, valid
}

// parseProjectConfig accepts strict canonical JSON with exactly the
// data-model members, the known schema, a UUIDv4 project ID, a canonical
// strategy and string parameters.
func parseProjectConfig(data []byte) (projectConfig, bool) {
	value, err := UnmarshalCanonical(data, RecordDecodeLimits)
	if err != nil {
		return projectConfig{}, false
	}
	object, err := canonicalFields(value, []string{"schema_version", "project_id", "strategy", "strategy_parameters"}, nil)
	if err != nil {
		return projectConfig{}, false
	}
	schema, _ := object["schema_version"].(string)
	id, _ := object["project_id"].(string)
	strategy, _ := object["strategy"].(string)
	parameters, ok := object["strategy_parameters"].(map[string]any)
	if schema != ProjectConfigSchema || !Strategy(strategy).Valid() || !ok || ProjectID(id).Validate() != nil {
		return projectConfig{}, false
	}
	config := projectConfig{id: ProjectID(id), strategy: Strategy(strategy), branches: map[string]string{}}
	for name, parameter := range parameters {
		value, isString := parameter.(string)
		if !isString {
			return projectConfig{}, false
		}
		switch {
		case name == "git_flow_dialect":
			config.dialect = GitFlowDialect(value)
		case containsValue(branchParameterNames, name):
			config.branches[name] = value
		}
	}
	return config, true
}
