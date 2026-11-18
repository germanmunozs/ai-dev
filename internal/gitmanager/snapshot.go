package gitmanager

import (
	"fmt"
	"strings"
)

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
func buildIntent(d strategyDecision, inspection GitInspection, projectID ProjectID, commit *CommitRequest) (ImmutableIntent, *PlanRejection) {
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
	in := ImmutableIntentInput{
		ProjectID: projectID, Strategy: d.strategy, GitFlowDialect: d.dialect, BranchParameters: d.branchParameters,
		DevelopmentBase: d.developmentBase, PrefixParameters: d.prefixParameters, InitializeRepository: d.initialize,
		ObjectFormat: format, RefFormat: refFormat, ToolIdentity: inspection.ToolIdentity,
		ProjectConfigBytesDigest: contentDigest(config), RequiredCapabilities: capabilities,
	}
	if commit != nil {
		if rejection := freezeCommit(&in, d, inspection, commit, config); rejection != nil {
			return ImmutableIntent{}, rejection
		}
	}
	intent, err := NewImmutableIntent(in)
	if err != nil {
		return ImmutableIntent{}, reject(ReasonPlanInvalid, "immutable intent: %v", err)
	}
	return intent, nil
}

// newProjectID draws a UUIDv4 for a project without a valid configuration.
func newProjectID(random RandomSource) (ProjectID, error) {
	id, err := newUUIDv4(random)
	return ProjectID(id), err
}

// newUUIDv4 draws a random UUIDv4 from the injected source.
func newUUIDv4(random RandomSource) (string, error) {
	b := make([]byte, 16)
	if random == nil {
		return "", fmt.Errorf("no random source")
	}
	if n, err := random.Read(b); err != nil || n != len(b) {
		return "", fmt.Errorf("random source failed")
	}
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// freezeCommit validates the initial-commit request and freezes its
// message, identities, dates and per-file approvals into the intent, in
// the order message, identity, repository state, allowlist.
func freezeCommit(in *ImmutableIntentInput, d strategyDecision, inspection GitInspection, commit *CommitRequest, config []byte) *PlanRejection {
	if rejection := checkCommitMessage(commit.MessageBytes); rejection != nil {
		return rejection
	}
	for _, identity := range []GitIdentity{commit.Author, commit.Committer} {
		if rejection := checkIdentity(identity); rejection != nil {
			return rejection
		}
	}
	if commit.AuthorDate.IsZero() || commit.CommitterDate.IsZero() || commit.AuthorDate.Nanosecond() != 0 || commit.CommitterDate.Nanosecond() != 0 {
		return reject(ReasonIdentityInvalid, "author and committer dates must be explicit whole seconds")
	}
	if _, rejection := initialCommitRef(d.strategy, d.branchParameters, inspection.Observation); rejection != nil {
		return rejection
	}
	files, rejection := resolveAllowlist(inspection, commit.Includes, config, in.ObjectFormat)
	if rejection != nil {
		return rejection
	}
	in.InitialCommit, in.Author, in.Committer = true, commit.Author, commit.Committer
	in.AuthorTimestamp, in.CommitterTimestamp = commit.AuthorDate, commit.CommitterDate
	in.CommitMessageBytes, in.AuthorizedFiles = commit.MessageBytes, files
	return nil
}

// initialCommitRef is the ref the initial commit creates: the explicit
// initial branch of a new repository, or the unborn branch HEAD names in
// an existing one, which must then equal the strategy's declared branch.
func initialCommitRef(strategy Strategy, branches map[string]string, observation RepositoryObservation) (string, *PlanRejection) {
	declared := initialBranchOf(strategy, branches)
	if observation.RepositoryKind == RepositoryKindAbsent {
		return "refs/heads/" + declared, nil
	}
	if rejection := repositoryAllowsInitialCommit(observation, ""); rejection != nil {
		return "", rejection
	}
	head := string(observation.Head.SymbolicRef.Bytes())
	if !strings.HasPrefix(head, "refs/heads/") {
		return "", reject(ReasonRepositoryStateIncompatible, "HEAD does not name an unborn branch")
	}
	if declared != "" && head != "refs/heads/"+declared {
		return "", reject(ReasonStrategyConflict, "HEAD names a branch other than the strategy's initial branch")
	}
	if rejection := repositoryAllowsInitialCommit(observation, head); rejection != nil {
		return "", rejection
	}
	return head, nil
}
