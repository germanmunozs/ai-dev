package gitmanager

// GitInspectOutputSchema is the schema of the inspect command's JSON output.
const GitInspectOutputSchema = "ai-dev.git-inspect-output/v1"

// EncodeInspectOutput renders a validated inspection as the canonical
// GitInspectOutput object (data-model §15): the complete redacted
// inspection plus its warnings and incompatibilities. It holds no
// mutation authority and no unredacted value.
func EncodeInspectOutput(v GitInspection) ([]byte, error) {
	if err := v.Validate(); err != nil {
		return nil, err
	}
	e := &canonicalEncoder{}
	inspection, _ := e.observation(v.Observation).(map[string]any)
	inspection["schema_version"] = v.SchemaVersion
	inspection["inspection_id"] = string(v.InspectionID)
	inspection["inspected_at"] = e.timestamp(v.InspectedAt)
	inspection["project_identity"] = e.projectIdentity(v.ProjectIdentity)
	inspection["tool_identity"] = e.tool(v.ToolIdentity)
	inspection["remotes_observation_state"] = string(v.RemotesObservationState)
	inspection["remotes_redacted"] = canonicalList(v.remotes, e.remote)
	warnings := canonicalStrings(v.warnings)
	incompatibilities := canonicalList(v.incompatibilities, func(i Incompatibility) any {
		object := map[string]any{"reason": string(i.Reason)}
		optional(object, "subject", i.Subject, i.Subject != "")
		return object
	})
	inspection["warnings"], inspection["incompatibilities"] = warnings, incompatibilities
	if e.err != nil {
		return nil, e.err
	}
	return MarshalCanonical(map[string]any{
		"schema_version":    GitInspectOutputSchema,
		"inspection":        inspection,
		"warnings":          warnings,
		"incompatibilities": incompatibilities,
	})
}

func (e *canonicalEncoder) remote(v RemoteRedacted) any {
	return map[string]any{
		"name":              v.Name,
		"fetch_urls":        canonicalList(v.fetchURLs, e.remoteURL),
		"push_urls":         canonicalList(v.pushURLs, e.remoteURL),
		"source_scope":      v.SourceScope,
		"source_digest":     string(v.SourceDigest),
		"observation_state": string(v.ObservationState),
	}
}
