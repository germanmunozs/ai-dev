package gitmanager

import (
	"fmt"
	"io/fs"
	"time"
)

// canonicalEncoder maps validated model values onto canonical JSON values
// with the member names of data-model.md. Optional members are omitted when
// absent; collections are always present, as empty arrays when empty. The
// first encoding error is kept and returned by the caller.
type canonicalEncoder struct{ err error }

func (e *canonicalEncoder) fail(err error) {
	if e.err == nil {
		e.err = err
	}
}

// canonicalList encodes every element; an empty or nil slice is [].
func canonicalList[T any](values []T, encode func(T) any) []any {
	list := make([]any, 0, len(values))
	for _, value := range values {
		list = append(list, encode(value))
	}
	return list
}

func canonicalStrings[T ~string](values []T) []any {
	return canonicalList(values, func(v T) any { return string(v) })
}

func canonicalMap(values map[string]string) map[string]any {
	object := make(map[string]any, len(values))
	for key, value := range values {
		object[key] = value
	}
	return object
}

// optional adds key only when the value is present.
func optional(object map[string]any, key string, value any, present bool) {
	if present {
		object[key] = value
	}
}

// timestamp keeps the frozen UTC offset and whole seconds, which is all a
// Git author or committer date can carry.
func (e *canonicalEncoder) timestamp(t time.Time) any {
	if t.Nanosecond() != 0 {
		e.fail(fmt.Errorf("%w: timestamp with sub-second precision", errCanonicalSyntax))
	}
	return t.Format(time.RFC3339)
}

func (e *canonicalEncoder) path(v ObservedPath) any {
	object := map[string]any{"reversible_path_bytes_base64url": v.ReversiblePathBytesBase64URL}
	optional(object, "display_path_utf8", v.DisplayPathUTF8, v.DisplayPathUTF8 != "")
	return object
}

// ref uses the ObservedPath member names: data-model gives both the same shape.
func (e *canonicalEncoder) ref(v ObservedRef) any {
	object := map[string]any{"reversible_path_bytes_base64url": v.ReversibleBytesBase64URL}
	optional(object, "display_path_utf8", v.DisplayUTF8, v.DisplayUTF8 != "")
	return object
}

func fileType(mode fs.FileMode) string {
	switch {
	case mode.IsRegular():
		return "regular"
	case mode.IsDir():
		return "directory"
	case mode&fs.ModeSymlink != 0:
		return "symlink"
	default:
		return "other"
	}
}

func (e *canonicalEncoder) fileIdentity(v FileIdentity) any {
	return map[string]any{
		"device_decimal":    v.DeviceDecimal,
		"inode_decimal":     v.InodeDecimal,
		"file_type":         fileType(v.Mode),
		"permissions_octal": fmt.Sprintf("%04o", v.Mode.Perm()),
		"size":              canonicalInteger(uint64(v.Size)),
	}
}

func (e *canonicalEncoder) projectIdentity(v PhysicalProjectIdentity) any {
	object := map[string]any{
		"reversible_path_bytes_base64url": EncodeReversiblePath(v.reversiblePathBytes),
		"device_decimal":                  v.deviceDecimal,
		"inode_decimal":                   v.inodeDecimal,
		"digest":                          string(v.digest),
	}
	optional(object, "physical_path_utf8", v.physicalPathUTF8, v.physicalPathUTF8 != "")
	return object
}

func (e *canonicalEncoder) tool(v GitToolIdentity) any {
	return map[string]any{
		"invocation_mode":             string(v.InvocationMode),
		"invocation_path":             v.InvocationPath,
		"invocation_path_digest":      string(v.InvocationPathDigest),
		"invocation_device":           v.InvocationDevice,
		"invocation_inode":            v.InvocationInode,
		"effective_executable_path":   v.EffectiveExecutablePath,
		"effective_executable_digest": string(v.EffectiveExecutableDigest),
		"version":                     v.Version,
		"exec_path":                   v.ExecPath,
		"capabilities":                canonicalStrings(v.capabilities),
		"environment_selectors":       canonicalMap(v.environmentSelectors),
	}
}

func (e *canonicalEncoder) gitIdentity(v GitIdentity) any {
	return map[string]any{"name": v.Name, "email": v.Email}
}

func (e *canonicalEncoder) authorizedFile(v AuthorizedFile) any {
	return map[string]any{
		"path_utf8":                       v.PathUTF8,
		"reversible_path_bytes_base64url": v.ReversiblePathBytesBase64URL,
		"mode":                            v.Mode,
		"content_sha256":                  string(v.ContentSHA256),
		"blob_oid":                        string(v.BlobOID),
		"ignored":                         v.Ignored,
		"approval_kind":                   string(v.ApprovalKind),
		"individually_approved":           v.IndividuallyApproved,
		"observed_identity":               e.fileIdentity(v.ObservedIdentity),
	}
}

func (e *canonicalEncoder) developmentBase(v DevelopmentBase) any {
	return map[string]any{
		"base_oid":           string(v.BaseOID),
		"object_format":      string(v.ObjectFormat),
		"production_ref":     e.ref(v.ProductionRef),
		"production_tip_oid": string(v.ProductionTipOID),
	}
}

func (e *canonicalEncoder) intent(v ImmutableIntent) any {
	object := map[string]any{
		"project_id":                  string(v.ProjectID),
		"strategy":                    string(v.Strategy),
		"branch_parameters":           canonicalMap(v.branchParameters),
		"prefix_parameters":           canonicalMap(v.prefixParameters),
		"initialize_repository":       v.InitializeRepository,
		"object_format":               string(v.ObjectFormat),
		"ref_format":                  string(v.RefFormat),
		"tool_identity":               e.tool(v.ToolIdentity),
		"project_config_bytes_digest": string(v.ProjectConfigBytesDigest),
		"initial_commit":              v.InitialCommit,
		"authorized_files":            canonicalList(v.authorizedFiles, e.authorizedFile),
		"required_capabilities":       canonicalStrings(v.requiredCapabilities),
	}
	optional(object, "git_flow_dialect", string(v.GitFlowDialect), v.GitFlowDialect != "")
	if !v.DevelopmentBase.IsZero() {
		object["development_base"] = e.developmentBase(v.DevelopmentBase)
	}
	if v.InitialCommit {
		object["author"] = e.gitIdentity(v.Author)
		object["committer"] = e.gitIdentity(v.Committer)
		object["author_timestamp"] = e.timestamp(v.AuthorTimestamp)
		object["committer_timestamp"] = e.timestamp(v.CommitterTimestamp)
		object["commit_message_bytes_base64url"] = EncodeReversiblePath(v.commitMessageBytes)
	}
	return object
}

func (e *canonicalEncoder) condition(v Condition) any {
	object := map[string]any{"kind": string(v.Kind)}
	optional(object, "subject", v.Subject, v.Subject != "")
	optional(object, "expected", v.Expected, v.Expected != "")
	return object
}

func (e *canonicalEncoder) head(v HeadObservation) any {
	object := map[string]any{"state": string(v.State), "history_present": v.HistoryPresent}
	if !v.SymbolicRef.IsZero() {
		object["symbolic_ref"] = e.ref(v.SymbolicRef)
	}
	optional(object, "commit_oid", string(v.CommitOID), v.CommitOID != "")
	return object
}

func (e *canonicalEncoder) refObservation(v RefObservation) any {
	object := map[string]any{"name": e.ref(v.Name), "target_oid": string(v.TargetOID)}
	optional(object, "peeled_oid", string(v.PeeledOID), v.PeeledOID != "")
	return object
}

func (e *canonicalEncoder) index(v IndexObservation) any {
	object := map[string]any{
		"presence":   string(v.Presence),
		"lock_state": string(v.LockState),
		"extensions": canonicalStrings(v.extensions),
		"changes": canonicalList(v.changes, func(c IndexChange) any {
			return map[string]any{"path": e.path(c.Path), "kind": string(c.Kind)}
		}),
	}
	if v.Presence == IndexPresent {
		object["version"] = canonicalInteger(uint64(v.Version))
		object["entry_count_decimal"] = v.EntryCountDecimal
		object["content_digest"] = string(v.ContentDigest)
	}
	return object
}

func (e *canonicalEncoder) worktreeChange(v WorktreeChange) any {
	return map[string]any{"path": e.path(v.Path), "kind": string(v.Kind)}
}

func (e *canonicalEncoder) worktreePathIdentity(v WorktreePathIdentity) any {
	object := map[string]any{"path": e.path(v.Path), "identity": e.fileIdentity(v.Identity)}
	optional(object, "content_digest", string(v.ContentDigest), v.ContentDigest != "")
	return object
}

func (e *canonicalEncoder) configEntry(v ConfigEntry) any {
	object := map[string]any{
		"ordinal":         canonicalInteger(v.Ordinal),
		"key":             v.Key,
		"value_present":   v.ValuePresent,
		"value_redaction": string(v.ValueRedaction),
		"scope":           string(v.Scope),
	}
	optional(object, "value_redacted", v.ValueRedacted, v.ValuePresent)
	if !v.Origin.IsZero() {
		object["origin"] = e.path(v.Origin)
	}
	optional(object, "source_digest", string(v.SourceDigest), v.SourceDigest != "")
	return object
}

func (e *canonicalEncoder) projectConfig(v ProjectConfigObservation) any {
	object := map[string]any{"state": string(v.State), "path": e.path(v.Path)}
	if v.State == ProjectConfigValid || v.State == ProjectConfigInvalid {
		object["identity"] = e.fileIdentity(v.Identity)
		object["content_digest"] = string(v.ContentDigest)
	}
	optional(object, "project_id", string(v.ProjectID), v.ProjectID != "")
	return object
}

func (e *canonicalEncoder) gitFlowEntry(v GitFlowEntry) any {
	return map[string]any{"key": v.Key, "value": v.Value, "origin": v.Origin, "source_digest": string(v.SourceDigest)}
}

func (e *canonicalEncoder) gitFlow(v GitFlowEvidence) any {
	object := map[string]any{
		"observation_state": string(v.ObservationState),
		"classification":    string(v.Classification),
		"entries":           canonicalList(v.entries, e.gitFlowEntry),
		"prefixes":          canonicalList(v.prefixes, e.gitFlowEntry),
		"instance_bases":    canonicalList(v.instanceBases, e.gitFlowEntry),
		"source_digests":    canonicalStrings(v.sourceDigests),
		"issues":            canonicalStrings(v.issues),
	}
	optional(object, "dialect", string(v.Dialect), v.Dialect != "")
	optional(object, "production_branch", v.ProductionBranch, v.ProductionBranch != "")
	optional(object, "development_branch", v.DevelopmentBranch, v.DevelopmentBranch != "")
	return object
}

func (e *canonicalEncoder) sourceIdentity(v InspectedSourceIdentity) any {
	object := map[string]any{
		"scope":      v.Scope,
		"read_state": string(v.ReadState),
		"precedence": canonicalInteger(v.Precedence),
	}
	if !v.Path.IsZero() {
		object["reversible_path"] = e.path(v.Path)
	}
	optional(object, "physical_identity_digest", string(v.PhysicalIdentityDigest), v.PhysicalIdentityDigest != "")
	optional(object, "content_digest", string(v.ContentDigest), v.ContentDigest != "")
	optional(object, "observed_size_decimal", v.ObservedSizeDecimal, v.ObservedSizeDecimal != "")
	return object
}

func (e *canonicalEncoder) attributeSource(v AttributeSource) any {
	return map[string]any{"identity": e.sourceIdentity(v.Identity), "kind": string(v.Kind)}
}

func (e *canonicalEncoder) ignoreSource(v IgnoreSource) any {
	object := map[string]any{"identity": e.sourceIdentity(v.Identity), "kind": string(v.Kind)}
	if !v.BaseDirectory.IsZero() {
		object["base_directory"] = e.path(v.BaseDirectory)
	}
	optional(object, "directory_depth", canonicalInteger(v.DirectoryDepth), v.HasDirectoryDepth)
	return object
}

func (e *canonicalEncoder) ignoreEvaluation(v IgnoreEvaluation) any {
	object := map[string]any{
		"reversible_path_bytes_base64url": v.Path.ReversiblePathBytesBase64URL,
		"ordered_source_digests":          canonicalStrings(v.orderedSourceDigests),
		"matched_rules":                   canonicalStrings(v.matchedRules),
		"ignored":                         v.Ignored,
		"equivalence_proven":              v.EquivalenceProven,
	}
	optional(object, "display_path_utf8", v.Path.DisplayPathUTF8, v.Path.DisplayPathUTF8 != "")
	optional(object, "winning_rule_source", string(v.WinningRuleSource), v.WinningRuleSource != "")
	optional(object, "incompatibility_reason", string(v.IncompatibilityReason), v.IncompatibilityReason != "")
	return object
}

func (e *canonicalEncoder) conversion(v ConversionSettings) any {
	object := map[string]any{
		"observation_state": string(v.ObservationState),
		"settings": canonicalList(v.settings, func(s EffectiveConversionSetting) any {
			setting := map[string]any{
				"key":                  s.Key,
				"source_scope":         s.SourceScope,
				"source_digest":        string(s.SourceDigest),
				"interpretation_state": s.InterpretationState,
			}
			optional(setting, "configured_value_redacted", s.ConfiguredValueRedacted, s.ConfiguredValueRedacted != "")
			optional(setting, "effective_value_redacted", s.EffectiveValueRedacted, s.EffectiveValueRedacted != "")
			return setting
		}),
		"filter_drivers":         canonicalStrings(v.filterDrivers),
		"external_code_required": v.ExternalCodeRequired,
		"byte_identity_proven":   v.ByteIdentityProven,
		"risk":                   string(v.Risk),
		"source_digests":         canonicalStrings(v.sourceDigests),
	}
	optional(object, "incompatibility_reason", string(v.IncompatibilityReason), v.IncompatibilityReason != "")
	return object
}

func (e *canonicalEncoder) linkedWorktree(v LinkedWorktree) any {
	object := map[string]any{
		"observation_state": string(v.ObservationState),
		"worktree_path":     e.path(v.WorktreePath),
		"head":              e.head(v.Head),
		"availability":      string(v.Availability),
		"locked":            v.Locked,
	}
	if !v.GitDirPath.IsZero() {
		object["git_dir_path"] = e.path(v.GitDirPath)
	}
	optional(object, "common_dir_identity", string(v.CommonDirIdentity), v.CommonDirIdentity != "")
	optional(object, "lock_reason_redacted", v.LockReasonRedacted, v.LockReasonRedacted != "")
	optional(object, "prunable", v.Prunable, v.PrunableObserved)
	return object
}

func (e *canonicalEncoder) remoteURL(v RedactedRemoteURL) any {
	object := map[string]any{
		"ordinal":                      canonicalInteger(v.Ordinal),
		"display_redacted":             v.DisplayRedacted,
		"credential_material_detected": v.CredentialMaterialDetected,
		"redaction_state":              string(v.RedactionState),
	}
	optional(object, "original_digest", string(v.OriginalDigest), v.OriginalDigest != "")
	return object
}

func (e *canonicalEncoder) submodule(v Submodule) any {
	object := map[string]any{
		"observation_state":    string(v.ObservationState),
		"name":                 v.Name,
		"path":                 e.path(v.Path),
		"recorded_commit_oid":  string(v.RecordedCommitOID),
		"declaration_digest":   string(v.DeclarationDigest),
		"initialization_state": string(v.InitializationState),
	}
	if !v.URLRedacted.IsZero() {
		object["url_redacted"] = e.remoteURL(v.URLRedacted)
	}
	optional(object, "observed_path_identity", string(v.ObservedPathIdentity), v.ObservedPathIdentity != "")
	optional(object, "observed_head_oid", string(v.ObservedHeadOID), v.ObservedHeadOID != "")
	optional(object, "observed_repository_kind", string(v.ObservedRepositoryKind), v.ObservedRepositoryKind != "")
	return object
}

func (e *canonicalEncoder) filesystemProfile(v FilesystemCapabilityProfile) any {
	object := map[string]any{
		"role":                          string(v.Role),
		"goos":                          v.GOOS,
		"filesystem_type":               v.FilesystemType,
		"mount_identity":                v.MountIdentity,
		"device_decimal":                v.DeviceDecimal,
		"inode_decimal":                 v.InodeDecimal,
		"mount_flags":                   canonicalStrings(v.mountFlags),
		"local":                         v.Local,
		"read_only":                     v.ReadOnly,
		"profile_version":               v.ProfileVersion,
		"exclusive_create":              v.ExclusiveCreate,
		"advisory_lock":                 v.AdvisoryLock,
		"file_sync":                     v.FileSync,
		"directory_sync":                v.DirectorySync,
		"atomic_same_filesystem_rename": v.AtomicSameFilesystemRename,
		"atomic_replace":                v.AtomicReplace,
		"stable_identity":               v.StableIdentity,
		"production_supported":          v.ProductionSupported,
	}
	optional(object, "probe_evidence_digest", string(v.ProbeEvidenceDigest), v.ProbeEvidenceDigest != "")
	return object
}

func (e *canonicalEncoder) observation(v RepositoryObservation) any {
	object := map[string]any{
		"repository_kind":                           string(v.RepositoryKind),
		"sequencer_state":                           string(v.SequencerState),
		"head":                                      e.head(v.Head),
		"refs_observation_state":                    string(v.RefsObservationState),
		"refs":                                      canonicalList(v.refs, e.refObservation),
		"index":                                     e.index(v.Index),
		"config_lock_state":                         string(v.ConfigLockState),
		"worktree_observation_state":                string(v.WorktreeObservationState),
		"worktree_changes":                          canonicalList(v.worktreeChanges, e.worktreeChange),
		"worktree_path_identities":                  canonicalList(v.worktreePathIdentities, e.worktreePathIdentity),
		"configuration_observation_state":           string(v.ConfigurationObservationState),
		"effective_configuration":                   canonicalList(v.effectiveConfiguration, e.configEntry),
		"project_config":                            e.projectConfig(v.ProjectConfig),
		"git_flow_evidence":                         e.gitFlow(v.GitFlowEvidence),
		"attribute_sources_observation_state":       string(v.AttributeSourcesObservationState),
		"attribute_sources":                         canonicalList(v.attributeSources, e.attributeSource),
		"ignore_sources_observation_state":          string(v.IgnoreSourcesObservationState),
		"ignore_sources":                            canonicalList(v.ignoreSources, e.ignoreSource),
		"ignore_evaluations_observation_state":      string(v.IgnoreEvaluationsObservationState),
		"ignore_evaluations":                        canonicalList(v.ignoreEvaluations, e.ignoreEvaluation),
		"conversion_settings":                       e.conversion(v.ConversionSettings),
		"linked_worktrees_observation_state":        string(v.LinkedWorktreesObservationState),
		"linked_worktrees":                          canonicalList(v.linkedWorktrees, e.linkedWorktree),
		"submodules_observation_state":              string(v.SubmodulesObservationState),
		"submodules":                                canonicalList(v.submodules, e.submodule),
		"filesystem_capabilities_observation_state": string(v.FilesystemCapabilitiesObservationState),
		"filesystem_capabilities":                   canonicalList(v.filesystemCapabilities, e.filesystemProfile),
	}
	if !v.GitDir.IsZero() {
		object["git_dir"] = e.path(v.GitDir)
	}
	if !v.CommonDir.IsZero() {
		object["common_dir"] = e.path(v.CommonDir)
	}
	optional(object, "object_format", string(v.ObjectFormat), v.ObjectFormat != "")
	optional(object, "ref_format", string(v.RefFormat), v.RefFormat != "")
	return object
}

func (e *canonicalEncoder) environment(v EnvironmentProfile) any {
	return map[string]any{
		"version":         v.Version,
		"state_root":      e.path(v.StateRoot),
		"home":            e.path(v.Home),
		"xdg_config_home": e.path(v.XDGConfigHome),
	}
}

func (e *canonicalEncoder) snapshot(v PreconditionSnapshot) any {
	object := map[string]any{
		"project_identity":       e.projectIdentity(v.ProjectIdentity),
		"repository_observation": e.observation(v.Observation),
		"environment_profile":    e.environment(v.EnvironmentProfile),
		"tool_identity":          e.tool(v.ToolIdentity),
	}
	if !v.DevelopmentBase.IsZero() {
		object["development_base"] = e.developmentBase(v.DevelopmentBase)
	}
	return object
}

func (e *canonicalEncoder) operation(v DerivedOperation) any {
	object := map[string]any{
		"operation_id":                  string(v.OperationID),
		"ordinal":                       canonicalInteger(v.Ordinal),
		"kind":                          string(v.Kind),
		"target_identity":               v.TargetIdentity,
		"preconditions":                 canonicalList(v.preconditions, e.condition),
		"affected_paths":                canonicalList(v.affectedPaths, e.path),
		"affected_refs":                 canonicalList(v.affectedRefs, e.ref),
		"affected_config_keys":          canonicalStrings(v.affectedConfigKeys),
		"expected_effects":              canonicalList(v.expectedEffects, e.condition),
		"expected_postconditions":       canonicalList(v.expectedPostconditions, e.condition),
		"failure_outcomes":              canonicalStrings(v.failureOutcomes),
		"partial_effect_classification": string(v.PartialEffectClassification),
		"recovery_instruction":          string(v.RecoveryInstruction),
		"temporary_resource_roles":      canonicalStrings(v.temporaryResourceRoles),
	}
	if len(v.exactArgv) > 0 {
		object["exact_argv"] = canonicalStrings(v.exactArgv)
		object["environment_profile"] = v.EnvironmentProfile
	}
	return object
}
