package gitmanager

// ProjectConfigSchema is the schema of .ai-dev/config.json (PCF-01).
const ProjectConfigSchema = "ai-dev.project-config/v1"

// classifyProjectConfig decides whether existing .ai-dev/config.json bytes
// are a valid project configuration: strict canonical JSON with exactly the
// data-model members, the known schema, a UUIDv4 project ID and a canonical
// strategy. It returns the project ID of a valid file.
func classifyProjectConfig(data []byte) (ProjectID, bool) {
	value, err := UnmarshalCanonical(data, RecordDecodeLimits)
	if err != nil {
		return "", false
	}
	object, err := canonicalFields(value, []string{"schema_version", "project_id", "strategy", "strategy_parameters"}, nil)
	if err != nil {
		return "", false
	}
	schema, _ := object["schema_version"].(string)
	id, _ := object["project_id"].(string)
	strategy, _ := object["strategy"].(string)
	parameters, ok := object["strategy_parameters"].(map[string]any)
	if schema != ProjectConfigSchema || !Strategy(strategy).Valid() || !ok || ProjectID(id).Validate() != nil {
		return "", false
	}
	for _, parameter := range parameters {
		if _, isString := parameter.(string); !isString {
			return "", false
		}
	}
	return ProjectID(id), true
}
