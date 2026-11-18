package gitmanager

import "fmt"

// incompatibilityPrecedence orders inspection incompatibilities when more
// than one prevents planning: platform and identity first, then the
// repository kind, then configuration and conversion (PLN-02).
var incompatibilityPrecedence = []Reason{
	ReasonPlatformUnsupported, ReasonProjectIdentityUnavailable, ReasonFilesystemUnsupported, ReasonDubiousOwnership,
	ReasonAncestorRepository, ReasonRepositoryKindUnsupported, ReasonWorktreeConfigUnsupported,
	ReasonRepositoryStateIncompatible, ReasonLockConflict, ReasonGitCapabilityMissing, ReasonAttributesIncompatible,
	ReasonConfigurationConflict,
}

// inspectionRejection returns the refusal the inspection's incompatibilities
// imply for planning, or nil.
func inspectionRejection(inspection GitInspection) *PlanRejection {
	found := inspection.Incompatibilities()
	if len(found) == 0 {
		return nil
	}
	for _, reason := range incompatibilityPrecedence {
		for _, item := range found {
			if item.Reason == reason {
				return reject(reason, "%s", item.Subject)
			}
		}
	}
	return reject(found[0].Reason, "%s", found[0].Subject)
}

// repositoryRejection applies the repository-state rules that hold for
// every plan (F008-FR-112 intrinsic part, O-103B-1): an interrupted
// sequencer, an invalid HEAD and Git lock files refuse.
func repositoryRejection(observation RepositoryObservation) *PlanRejection {
	switch {
	case observation.SequencerState != SequencerNone:
		return reject(ReasonRepositoryStateIncompatible, "an interrupted %s operation is in progress", observation.SequencerState)
	case observation.Head.State == HeadStateInvalid:
		return reject(ReasonRepositoryStateIncompatible, "HEAD is invalid")
	case observation.ConfigLockState != LockFileAbsent:
		return reject(ReasonLockConflict, "the repository configuration lock exists")
	case observation.Index.LockState != LockFileAbsent:
		return reject(ReasonLockConflict, "the index lock exists")
	}
	return nil
}

// buildSnapshot freezes a reliable inspection (T054). Only an inspection
// without incompatibilities and with every group observed can become a
// precondition snapshot.
func buildSnapshot(inspection GitInspection, environment EnvironmentProfile, base DevelopmentBase) (PreconditionSnapshot, *PlanRejection) {
	if rejection := inspectionRejection(inspection); rejection != nil {
		return PreconditionSnapshot{}, rejection
	}
	if rejection := repositoryRejection(inspection.Observation); rejection != nil {
		return PreconditionSnapshot{}, rejection
	}
	snapshot := PreconditionSnapshot{
		ProjectIdentity: inspection.ProjectIdentity, Observation: inspection.Observation, EnvironmentProfile: environment,
		DevelopmentBase: base, ToolIdentity: inspection.ToolIdentity,
	}
	if err := snapshot.Validate(); err != nil {
		return PreconditionSnapshot{}, reject(ReasonRepositoryStateIncompatible, "the inspection cannot be frozen: %v", err)
	}
	return snapshot, nil
}

// renderProjectConfig renders the deterministic .ai-dev/config.json bytes:
// canonical JSON with the frozen project ID, the canonical strategy name
// and its explicit parameters (F008-FR-072–079, F008-FR-187).
func renderProjectConfig(projectID ProjectID, strategy Strategy, dialect GitFlowDialect, branches, prefixes map[string]string) ([]byte, error) {
	parameters := map[string]any{}
	for name, value := range branches {
		parameters[name] = value
	}
	for name, value := range prefixes {
		parameters[name+"_prefix"] = value
	}
	if dialect != "" {
		parameters["git_flow_dialect"] = string(dialect)
	}
	return MarshalCanonical(map[string]any{
		"schema_version": ProjectConfigSchema, "project_id": string(projectID), "strategy": string(strategy),
		"strategy_parameters": parameters,
	})
}

// requiredCapabilities are the toolchain capabilities the plan's rows need:
// discovery always, and initialization when a repository is created.
func requiredCapabilities(initialize bool) []string {
	capabilities := []string{"path-format"}
	if initialize {
		capabilities = append(capabilities, "initial-branch", "object-format", "ref-format")
	}
	return capabilities
}

// buildIntent freezes the validated strategy decision (T054).
func buildIntent(d strategyDecision, inspection GitInspection, projectID ProjectID) (ImmutableIntent, *PlanRejection) {
	format, refFormat := ObjectFormatSHA1, RefFormatFiles
	if !d.initialize {
		format, refFormat = inspection.Observation.ObjectFormat, inspection.Observation.RefFormat
	}
	config, err := renderProjectConfig(projectID, d.strategy, d.dialect, d.branchParameters, d.prefixParameters)
	if err != nil {
		return ImmutableIntent{}, reject(ReasonPlanInvalid, "project configuration cannot be rendered: %v", err)
	}
	capabilities := requiredCapabilities(d.initialize)
	for _, capability := range capabilities {
		if !containsValue(inspection.ToolIdentity.Capabilities(), capability) {
			return ImmutableIntent{}, reject(ReasonGitCapabilityMissing, "the Git toolchain lacks %s", capability)
		}
	}
	intent, err := NewImmutableIntent(ImmutableIntentInput{
		ProjectID: projectID, Strategy: d.strategy, GitFlowDialect: d.dialect, BranchParameters: d.branchParameters,
		DevelopmentBase: d.developmentBase, PrefixParameters: d.prefixParameters, InitializeRepository: d.initialize,
		ObjectFormat: format, RefFormat: refFormat, ToolIdentity: inspection.ToolIdentity,
		ProjectConfigBytesDigest: contentDigest(config), RequiredCapabilities: capabilities,
	})
	if err != nil {
		return ImmutableIntent{}, reject(ReasonPlanInvalid, "immutable intent: %v", err)
	}
	return intent, nil
}

// newProjectID draws a UUIDv4 for a project without a valid configuration.
func newProjectID(random RandomSource) (ProjectID, error) {
	b := make([]byte, 16)
	if random == nil {
		return "", fmt.Errorf("no random source")
	}
	if n, err := random.Read(b); err != nil || n != len(b) {
		return "", fmt.Errorf("random source failed")
	}
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return ProjectID(fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])), nil
}
