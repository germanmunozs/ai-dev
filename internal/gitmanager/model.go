package gitmanager

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Model conventions, applied to every entity in this file:
//
//   - one constructor per entity with collections; it copies caller-owned
//     collections and returns the value together with its Validate result;
//   - Validate checks the complete current state of the value, so it holds
//     for every instance regardless of how it was built;
//   - scalar fields are exported; slices and maps are unexported and reached
//     only through accessors that return copies.

var errInvalidModel = errors.New("invalid git management model")

type PlanID string
type PlanDigest string
type InspectionID string
type AuthorizationID string
type ApplyID string
type OperationID string
type ProjectIdentityDigest string
type ContentDigest string
type BlobOID string
type ObjectOID string
type ProjectID string

func (v PlanID) Validate() error       { return validateOpaque("plan ID", string(v)) }
func (v InspectionID) Validate() error { return validateOpaque("inspection ID", string(v)) }
func (v PlanDigest) Validate() error   { return validateSHA256("plan digest", string(v)) }
func (v ProjectIdentityDigest) Validate() error {
	return validateSHA256("project identity digest", string(v))
}
func (v ContentDigest) Validate() error { return validateSHA256("content digest", string(v)) }
func (v AuthorizationID) Validate() error {
	return validateUUIDv4("authorization ID", string(v))
}
func (v ApplyID) Validate() error   { return validateUUIDv4("apply ID", string(v)) }
func (v ProjectID) Validate() error { return validateUUIDv4("project ID", string(v)) }
func (v OperationID) Validate() error {
	parts := strings.Split(string(v), ":")
	if len(parts) != 3 || parts[0] == "" {
		return invalid("operation ID")
	}
	if _, err := strconv.ParseUint(parts[0], 10, 64); err != nil || (len(parts[0]) > 1 && parts[0][0] == '0') {
		return invalid("operation ID ordinal")
	}
	return validateSHA256("operation ID digest", parts[1]+":"+parts[2])
}
func (v BlobOID) Validate() error { return validateOID("blob OID", string(v)) }

// ValidateFor additionally requires the OID to have the frozen object
// format's length. It never resolves the object.
func (v BlobOID) ValidateFor(format ObjectFormat) error {
	return validateOIDFor("blob OID", string(v), format)
}

// ObjectOID identifies commits, trees, gitlinks, and observed HEAD objects.
// It is not interchangeable with BlobOID and proves no object existence.
func (v ObjectOID) Validate() error { return validateOID("object OID", string(v)) }
func (v ObjectOID) ValidateFor(format ObjectFormat) error {
	return validateOIDFor("object OID", string(v), format)
}

type ObjectFormat string

const (
	ObjectFormatSHA1   ObjectFormat = "sha1"
	ObjectFormatSHA256 ObjectFormat = "sha256"
)

func (v ObjectFormat) Valid() bool { return v == ObjectFormatSHA1 || v == ObjectFormatSHA256 }

type RefFormat string

const (
	RefFormatFiles    RefFormat = "files"
	RefFormatReftable RefFormat = "reftable"
)

func (v RefFormat) Valid() bool { return v == RefFormatFiles || v == RefFormatReftable }

type InvocationMode string

const (
	InvocationModeLauncher InvocationMode = "launcher"
	InvocationModeDirect   InvocationMode = "direct"
)

func (v InvocationMode) Valid() bool { return v == InvocationModeLauncher || v == InvocationModeDirect }

type RepositoryKind string

const (
	RepositoryKindAbsent         RepositoryKind = "absent"
	RepositoryKindNormal         RepositoryKind = "normal"
	RepositoryKindBare           RepositoryKind = "bare"
	RepositoryKindLinkedWorktree RepositoryKind = "linked_worktree"
	RepositoryKindSubmodule      RepositoryKind = "submodule"
	RepositoryKindNested         RepositoryKind = "nested"
	RepositoryKindAncestor       RepositoryKind = "ancestor"
	RepositoryKindInaccessible   RepositoryKind = "inaccessible"
	RepositoryKindAmbiguous      RepositoryKind = "ambiguous"
)

func (v RepositoryKind) Valid() bool {
	switch v {
	case RepositoryKindAbsent, RepositoryKindNormal, RepositoryKindBare, RepositoryKindLinkedWorktree,
		RepositoryKindSubmodule, RepositoryKindNested, RepositoryKindAncestor, RepositoryKindInaccessible,
		RepositoryKindAmbiguous:
		return true
	default:
		return false
	}
}

type HeadState string

const (
	HeadStateUnborn   HeadState = "unborn"
	HeadStateAttached HeadState = "attached"
	HeadStateDetached HeadState = "detached"
	HeadStateInvalid  HeadState = "invalid"
)

func (v HeadState) Valid() bool {
	return v == HeadStateUnborn || v == HeadStateAttached || v == HeadStateDetached || v == HeadStateInvalid
}

type Strategy string

const (
	StrategyBasic      Strategy = "basic"
	StrategyGitFlow    Strategy = "git-flow"
	StrategyGitHubFlow Strategy = "github-flow"
	StrategyTrunkBased Strategy = "trunk-based"
)

func (v Strategy) Valid() bool {
	return v == StrategyBasic || v == StrategyGitFlow || v == StrategyGitHubFlow || v == StrategyTrunkBased
}

type GitFlowDialect string

const (
	GitFlowDialectAVH  GitFlowDialect = "avh"
	GitFlowDialectNext GitFlowDialect = "git-flow-next"
)

func (v GitFlowDialect) Valid() bool { return v == GitFlowDialectAVH || v == GitFlowDialectNext }

type AuthorizationState string

const (
	AuthorizationStateIssued           AuthorizationState = "issued"
	AuthorizationStateClaimed          AuthorizationState = "claimed"
	AuthorizationStateMutationStarted  AuthorizationState = "mutation_started"
	AuthorizationStateApplied          AuthorizationState = "applied"
	AuthorizationStateFailed           AuthorizationState = "failed"
	AuthorizationStatePartiallyApplied AuthorizationState = "partially_applied"
	AuthorizationStateIndeterminate    AuthorizationState = "state_indeterminate"
)

func (v AuthorizationState) Valid() bool {
	switch v {
	case AuthorizationStateIssued, AuthorizationStateClaimed, AuthorizationStateMutationStarted,
		AuthorizationStateApplied, AuthorizationStateFailed, AuthorizationStatePartiallyApplied,
		AuthorizationStateIndeterminate:
		return true
	default:
		return false
	}
}

func (v AuthorizationState) terminal() bool {
	return v == AuthorizationStateApplied || v == AuthorizationStateFailed ||
		v == AuthorizationStatePartiallyApplied || v == AuthorizationStateIndeterminate
}

type ApplyStatus string

const (
	ApplyStatusApplied          ApplyStatus = "applied"
	ApplyStatusAlreadyApplied   ApplyStatus = "already_applied"
	ApplyStatusRefused          ApplyStatus = "refused"
	ApplyStatusFailed           ApplyStatus = "failed"
	ApplyStatusPartiallyApplied ApplyStatus = "partially_applied"
	ApplyStatusIndeterminate    ApplyStatus = "state_indeterminate"
)

func (v ApplyStatus) Valid() bool {
	switch v {
	case ApplyStatusApplied, ApplyStatusAlreadyApplied, ApplyStatusRefused, ApplyStatusFailed,
		ApplyStatusPartiallyApplied, ApplyStatusIndeterminate:
		return true
	default:
		return false
	}
}

type Reason string

const (
	ReasonInteractiveTerminalRequired            Reason = "interactive_terminal_required"
	ReasonInvalidStrategy                        Reason = "invalid_strategy"
	ReasonMissingRequiredBranch                  Reason = "missing_required_branch"
	ReasonRepositoryAbsent                       Reason = "repository_absent"
	ReasonAncestorRepository                     Reason = "ancestor_repository"
	ReasonRepositoryKindUnsupported              Reason = "repository_kind_unsupported"
	ReasonRepositoryStateIncompatible            Reason = "repository_state_incompatible"
	ReasonWorktreeConfigUnsupported              Reason = "worktree_config_unsupported"
	ReasonProjectIdentityUnavailable             Reason = "project_identity_unavailable"
	ReasonPlanInvalid                            Reason = "plan_invalid"
	ReasonPlanStale                              Reason = "plan_stale"
	ReasonConcurrentChange                       Reason = "concurrent_change"
	ReasonInterrupted                            Reason = "interrupted"
	ReasonAuthorizationInputIncomplete           Reason = "authorization_input_incomplete"
	ReasonAuthorizationIDExists                  Reason = "authorization_id_exists"
	ReasonAuthorizationMissing                   Reason = "authorization_missing"
	ReasonAuthorizationMismatch                  Reason = "authorization_mismatch"
	ReasonAuthorizationExpired                   Reason = "authorization_expired"
	ReasonAuthorizationConsumed                  Reason = "authorization_consumed"
	ReasonAuditUnavailable                       Reason = "audit_unavailable"
	ReasonAuditFailedAfterMutation               Reason = "audit_failed_after_mutation"
	ReasonResourceReleaseFailed                  Reason = "resource_release_failed"
	ReasonLockConflict                           Reason = "lock_conflict"
	ReasonGitCapabilityMissing                   Reason = "git_capability_missing"
	ReasonDevelopmentBranchPendingInitialHistory Reason = "development_branch_pending_initial_history"
	ReasonOutputExists                           Reason = "output_exists"
	ReasonUnsafeOutputPath                       Reason = "unsafe_output_path"
	ReasonOutputFailed                           Reason = "output_failed"
	ReasonOperationFailed                        Reason = "operation_failed"
	ReasonAttributesIncompatible                 Reason = "attributes_incompatible"
	ReasonPathUnrepresentable                    Reason = "path_unrepresentable"
	ReasonPathNormalizationIncompatible          Reason = "path_normalization_incompatible"
	ReasonPathCollision                          Reason = "path_collision"
	ReasonSensitiveContentSuspected              Reason = "sensitive_content_suspected"
	ReasonCommitMessageInvalid                   Reason = "commit_message_invalid"
	ReasonIdentityInvalid                        Reason = "identity_invalid"
	ReasonPlatformUnsupported                    Reason = "platform_unsupported"
	ReasonFilesystemUnsupported                  Reason = "filesystem_unsupported"
	ReasonDubiousOwnership                       Reason = "dubious_ownership"
	ReasonStrategyConflict                       Reason = "strategy_conflict"
	ReasonConfigurationConflict                  Reason = "configuration_conflict"
)

var validReasons = map[Reason]struct{}{
	ReasonInteractiveTerminalRequired: {}, ReasonInvalidStrategy: {}, ReasonMissingRequiredBranch: {},
	ReasonRepositoryAbsent: {}, ReasonAncestorRepository: {}, ReasonRepositoryKindUnsupported: {},
	ReasonRepositoryStateIncompatible: {}, ReasonWorktreeConfigUnsupported: {}, ReasonProjectIdentityUnavailable: {},
	ReasonPlanInvalid: {}, ReasonPlanStale: {}, ReasonConcurrentChange: {}, ReasonInterrupted: {}, ReasonAuthorizationInputIncomplete: {},
	ReasonAuthorizationIDExists: {}, ReasonAuthorizationMissing: {}, ReasonAuthorizationMismatch: {},
	ReasonAuthorizationExpired: {}, ReasonAuthorizationConsumed: {}, ReasonAuditUnavailable: {},
	ReasonAuditFailedAfterMutation: {}, ReasonResourceReleaseFailed: {}, ReasonLockConflict: {},
	ReasonGitCapabilityMissing: {}, ReasonDevelopmentBranchPendingInitialHistory: {}, ReasonOutputExists: {},
	ReasonUnsafeOutputPath: {}, ReasonOutputFailed: {}, ReasonOperationFailed: {}, ReasonAttributesIncompatible: {},
	ReasonPathUnrepresentable: {}, ReasonPathNormalizationIncompatible: {}, ReasonPathCollision: {},
	ReasonSensitiveContentSuspected: {}, ReasonCommitMessageInvalid: {}, ReasonIdentityInvalid: {},
	ReasonPlatformUnsupported: {}, ReasonFilesystemUnsupported: {}, ReasonDubiousOwnership: {},
	ReasonStrategyConflict: {}, ReasonConfigurationConflict: {},
}

func (v Reason) Valid() bool { _, ok := validReasons[v]; return ok }

type ResourceRole string

const (
	ResourceRoleApplyLock       ResourceRole = "apply_lock"
	ResourceRoleIndexLock       ResourceRole = "index_lock"
	ResourceRoleConfigTemporary ResourceRole = "config_temporary"
	ResourceRolePreparedIndex   ResourceRole = "prepared_index"
)

func (v ResourceRole) Valid() bool {
	return v == ResourceRoleApplyLock || v == ResourceRoleIndexLock ||
		v == ResourceRoleConfigTemporary || v == ResourceRolePreparedIndex
}

type ApprovalKind string

const (
	ApprovalKindNormal  ApprovalKind = "normal"
	ApprovalKindIgnored ApprovalKind = "ignored"
)

func (v ApprovalKind) Valid() bool { return v == ApprovalKindNormal || v == ApprovalKindIgnored }

type OperationKind string

const (
	OperationKindAuditWrite                   OperationKind = "audit_write"
	OperationKindAuthorizationTransition      OperationKind = "authorization_transition"
	OperationKindApplyLock                    OperationKind = "apply_lock"
	OperationKindRepositoryInit               OperationKind = "repository_init"
	OperationKindGitConfigWrite               OperationKind = "git_config_write"
	OperationKindCreateProjectConfigDirectory OperationKind = "create_project_config_directory"
	OperationKindProjectConfigWrite           OperationKind = "project_config_write"
	OperationKindInitialCommit                OperationKind = "initial_commit"
	OperationKindInitialRefCAS                OperationKind = "initial_ref_cas"
	OperationKindDevelopmentRefCAS            OperationKind = "development_ref_cas"
	OperationKindPreparedIndex                OperationKind = "prepared_index"
	OperationKindVerification                 OperationKind = "verification"
	OperationKindResourceRelease              OperationKind = "resource_release"
)

func (v OperationKind) Valid() bool {
	switch v {
	case OperationKindAuditWrite, OperationKindAuthorizationTransition, OperationKindApplyLock,
		OperationKindRepositoryInit, OperationKindGitConfigWrite, OperationKindCreateProjectConfigDirectory,
		OperationKindProjectConfigWrite, OperationKindInitialCommit, OperationKindInitialRefCAS,
		OperationKindDevelopmentRefCAS, OperationKindPreparedIndex, OperationKindVerification,
		OperationKindResourceRelease:
		return true
	default:
		return false
	}
}

func ValidateDecimalIdentity(value string) error {
	if value == "0" {
		return nil
	}
	if value == "" || value[0] == '0' {
		return invalid("decimal identity")
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return invalid("decimal identity")
		}
	}
	return nil
}

func EncodeReversiblePath(path []byte) string { return base64.RawURLEncoding.EncodeToString(path) }

func DecodeReversiblePath(encoded string) ([]byte, error) {
	for _, c := range []byte(encoded) {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return nil, invalid("reversible path alphabet")
		}
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("%w: reversible path: %v", errInvalidModel, err)
	}
	if EncodeReversiblePath(decoded) != encoded {
		return nil, invalid("non-canonical reversible path")
	}
	return decoded, nil
}

type PhysicalProjectIdentity struct {
	physicalPathUTF8    string
	reversiblePathBytes []byte
	deviceDecimal       string
	inodeDecimal        string
	digest              ProjectIdentityDigest
}

func NewPhysicalProjectIdentity(display string, path []byte, device, inode string) (PhysicalProjectIdentity, error) {
	v := PhysicalProjectIdentity{
		physicalPathUTF8:    display,
		reversiblePathBytes: cloneSlice(path),
		deviceDecimal:       device,
		inodeDecimal:        inode,
		digest:              ComputeProjectIdentityDigest(path, device, inode),
	}
	return v, v.Validate()
}

func ComputeProjectIdentityDigest(path []byte, device, inode string) ProjectIdentityDigest {
	h := sha256.New()
	_, _ = h.Write(path)
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(device))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(inode))
	return ProjectIdentityDigest(fmt.Sprintf("sha256:%x", h.Sum(nil)))
}

func (v PhysicalProjectIdentity) Validate() error {
	if len(v.reversiblePathBytes) == 0 {
		return invalid("physical project path")
	}
	if utf8.Valid(v.reversiblePathBytes) {
		if v.physicalPathUTF8 != string(v.reversiblePathBytes) {
			return invalid("physical project display path")
		}
	} else if v.physicalPathUTF8 != "" {
		return invalid("non-UTF-8 physical project display path")
	}
	if err := ValidateDecimalIdentity(v.deviceDecimal); err != nil {
		return err
	}
	if err := ValidateDecimalIdentity(v.inodeDecimal); err != nil {
		return err
	}
	if err := v.digest.Validate(); err != nil || v.digest != ComputeProjectIdentityDigest(v.reversiblePathBytes, v.deviceDecimal, v.inodeDecimal) {
		return invalid("physical project identity digest")
	}
	return nil
}

// IsZero reports whether v is the unset identity used for optional members.
func (v PhysicalProjectIdentity) IsZero() bool {
	return v.physicalPathUTF8 == "" && len(v.reversiblePathBytes) == 0 && v.deviceDecimal == "" &&
		v.inodeDecimal == "" && v.digest == ""
}

func (v PhysicalProjectIdentity) PhysicalPathUTF8() string { return v.physicalPathUTF8 }
func (v PhysicalProjectIdentity) ReversiblePathBytes() []byte {
	return cloneSlice(v.reversiblePathBytes)
}
func (v PhysicalProjectIdentity) DeviceDecimal() string         { return v.deviceDecimal }
func (v PhysicalProjectIdentity) InodeDecimal() string          { return v.inodeDecimal }
func (v PhysicalProjectIdentity) Digest() ProjectIdentityDigest { return v.digest }

type GitToolIdentity struct {
	InvocationMode            InvocationMode
	InvocationPath            string
	InvocationPathDigest      ContentDigest
	InvocationDevice          string
	InvocationInode           string
	EffectiveExecutablePath   string
	EffectiveExecutableDigest ContentDigest
	Version                   string
	ExecPath                  string
	capabilities              []string
	environmentSelectors      map[string]string
}

type GitToolIdentityInput struct {
	InvocationMode            InvocationMode
	InvocationPath            string
	InvocationPathDigest      ContentDigest
	InvocationDevice          string
	InvocationInode           string
	EffectiveExecutablePath   string
	EffectiveExecutableDigest ContentDigest
	Version                   string
	ExecPath                  string
	Capabilities              []string
	EnvironmentSelectors      map[string]string
}

func NewGitToolIdentity(in GitToolIdentityInput) (GitToolIdentity, error) {
	v := GitToolIdentity{
		InvocationMode:            in.InvocationMode,
		InvocationPath:            in.InvocationPath,
		InvocationPathDigest:      in.InvocationPathDigest,
		InvocationDevice:          in.InvocationDevice,
		InvocationInode:           in.InvocationInode,
		EffectiveExecutablePath:   in.EffectiveExecutablePath,
		EffectiveExecutableDigest: in.EffectiveExecutableDigest,
		Version:                   in.Version,
		ExecPath:                  in.ExecPath,
		capabilities:              cloneSlice(in.Capabilities),
		environmentSelectors:      cloneMap(in.EnvironmentSelectors),
	}
	return v, v.Validate()
}

func (v GitToolIdentity) Validate() error {
	if !v.InvocationMode.Valid() || v.InvocationPath == "" || v.EffectiveExecutablePath == "" || v.Version == "" || v.ExecPath == "" {
		return invalid("Git tool identity fields")
	}
	if len(v.capabilities) == 0 {
		return invalid("Git capabilities")
	}
	return firstError(
		v.InvocationPathDigest.Validate(),
		v.EffectiveExecutableDigest.Validate(),
		ValidateDecimalIdentity(v.InvocationDevice),
		ValidateDecimalIdentity(v.InvocationInode),
		validateLabels("Git capability", v.capabilities),
	)
}

func (v GitToolIdentity) Capabilities() []string { return cloneSlice(v.capabilities) }
func (v GitToolIdentity) EnvironmentSelectors() map[string]string {
	return cloneMap(v.environmentSelectors)
}

// ObservationState distinguishes a successful empty observation from
// absence, non-applicability, lack of observation, and observation failure.
type ObservationState string

const (
	ObservationNotApplicable ObservationState = "not_applicable"
	ObservationNotObserved   ObservationState = "not_observed"
	ObservationObservedEmpty ObservationState = "observed_empty"
	ObservationObserved      ObservationState = "observed"
	ObservationAbsent        ObservationState = "absent"
	ObservationError         ObservationState = "error"
)

func (v ObservationState) Valid() bool {
	switch v {
	case ObservationNotApplicable, ObservationNotObserved, ObservationObservedEmpty,
		ObservationObserved, ObservationAbsent, ObservationError:
		return true
	default:
		return false
	}
}

// unreliable reports the states that planning cannot rely on and that must
// therefore be explained by a warning or incompatibility.
func (v ObservationState) unreliable() bool {
	return v == ObservationNotObserved || v == ObservationError
}

// validateObservedGroup enforces that a collection and its companion state
// agree: only an observed group carries entries.
func validateObservedGroup(name string, state ObservationState, entries int) error {
	if !state.Valid() || (state == ObservationObserved) != (entries > 0) {
		return invalid(name + " observation state")
	}
	return nil
}

// validateReversible checks one exact-bytes value: the display member is
// present exactly when the bytes are valid UTF-8 and then equals them.
func validateReversible(name, encoded, display string) ([]byte, error) {
	raw, err := DecodeReversiblePath(encoded)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, invalid(name)
	}
	if utf8.Valid(raw) {
		if display != string(raw) {
			return nil, invalid(name + " display")
		}
	} else if display != "" {
		return nil, invalid("non-UTF-8 " + name + " display")
	}
	return raw, nil
}

// ObservedPath preserves exact path bytes. The display value is present
// exactly when the bytes are valid UTF-8 and then equals them unnormalized.
type ObservedPath struct {
	ReversiblePathBytesBase64URL string
	DisplayPathUTF8              string
}

func NewObservedPath(path []byte) ObservedPath {
	v := ObservedPath{ReversiblePathBytesBase64URL: EncodeReversiblePath(path)}
	if utf8.Valid(path) {
		v.DisplayPathUTF8 = string(path)
	}
	return v
}

// IsZero reports whether v is the unset path used for optional members.
func (v ObservedPath) IsZero() bool { return v == ObservedPath{} }

func (v ObservedPath) Validate() error {
	raw, err := validateReversible("observed path", v.ReversiblePathBytesBase64URL, v.DisplayPathUTF8)
	if err != nil {
		return err
	}
	return CheckPathBytes(raw)
}

// Bytes returns the exact path bytes; it is meaningful only for a valid value.
func (v ObservedPath) Bytes() []byte {
	raw, _ := DecodeReversiblePath(v.ReversiblePathBytesBase64URL)
	return raw
}

func validateOptionalPath(v ObservedPath) error {
	if v.IsZero() {
		return nil
	}
	return v.Validate()
}

// ObservedRef preserves the exact bytes of a full reference name below
// refs/. It has the same encoding as ObservedPath but is a distinct type.
type ObservedRef struct {
	ReversibleBytesBase64URL string
	DisplayUTF8              string
}

func NewObservedRef(name []byte) ObservedRef {
	v := ObservedRef{ReversibleBytesBase64URL: EncodeReversiblePath(name)}
	if utf8.Valid(name) {
		v.DisplayUTF8 = string(name)
	}
	return v
}

func (v ObservedRef) IsZero() bool { return v == ObservedRef{} }

func (v ObservedRef) Validate() error {
	raw, err := validateReversible("observed ref", v.ReversibleBytesBase64URL, v.DisplayUTF8)
	if err != nil {
		return err
	}
	if !bytes.HasPrefix(raw, []byte("refs/")) || len(raw) == len("refs/") || bytes.IndexByte(raw, 0) >= 0 {
		return invalid("observed ref name")
	}
	return nil
}

func (v ObservedRef) Bytes() []byte {
	raw, _ := DecodeReversiblePath(v.ReversibleBytesBase64URL)
	return raw
}

type SourceReadState string

const (
	SourceReadAbsent      SourceReadState = "absent"
	SourceReadRead        SourceReadState = "read"
	SourceReadUnreadable  SourceReadState = "unreadable"
	SourceReadAmbiguous   SourceReadState = "ambiguous"
	SourceReadUnsupported SourceReadState = "unsupported"
)

func (v SourceReadState) Valid() bool {
	switch v {
	case SourceReadAbsent, SourceReadRead, SourceReadUnreadable, SourceReadAmbiguous, SourceReadUnsupported:
		return true
	default:
		return false
	}
}

// InspectedSourceIdentity is the provenance shared by attribute and ignore
// sources. A read source has a descriptor-verified identity and content
// digest; an absent source has neither fabricated bytes nor digest.
type InspectedSourceIdentity struct {
	Scope                  string
	Path                   ObservedPath
	PhysicalIdentityDigest ContentDigest
	ContentDigest          ContentDigest
	ReadState              SourceReadState
	ObservedSizeDecimal    string
	Precedence             uint64
}

func (v InspectedSourceIdentity) Validate() error {
	if !v.ReadState.Valid() {
		return invalid("source read state")
	}
	if err := firstError(validateOpaque("source scope", v.Scope), validateOptionalPath(v.Path)); err != nil {
		return err
	}
	switch v.ReadState {
	case SourceReadRead:
		if v.Path.IsZero() || v.PhysicalIdentityDigest == "" || v.ContentDigest == "" {
			return invalid("read source identity")
		}
	case SourceReadAbsent:
		if v.PhysicalIdentityDigest != "" || v.ContentDigest != "" || v.ObservedSizeDecimal != "" {
			return invalid("absent source identity")
		}
	}
	if v.PhysicalIdentityDigest != "" {
		if err := v.PhysicalIdentityDigest.Validate(); err != nil {
			return err
		}
	}
	if v.ContentDigest != "" {
		if err := v.ContentDigest.Validate(); err != nil {
			return err
		}
	}
	if v.ObservedSizeDecimal != "" {
		return ValidateDecimalIdentity(v.ObservedSizeDecimal)
	}
	return nil
}

type RedactionState string

const (
	RedactionNotNeeded RedactionState = "not_needed"
	RedactionRedacted  RedactionState = "redacted"
	RedactionOpaque    RedactionState = "opaque"
)

func (v RedactionState) Valid() bool {
	return v == RedactionNotNeeded || v == RedactionRedacted || v == RedactionOpaque
}

// OpaqueMarker is the only value stored for a URL or configuration value
// that cannot be redacted with demonstrable safety.
const OpaqueMarker = "<opaque>"

// RedactedRemoteURL never holds an original URL, credentials, userinfo, or
// tokens: the inspector redacts before constructing it.
type RedactedRemoteURL struct {
	Ordinal                    uint64
	DisplayRedacted            string
	CredentialMaterialDetected bool
	RedactionState             RedactionState
	OriginalDigest             ContentDigest
}

// IsZero reports whether v is the unset URL used for optional members.
func (v RedactedRemoteURL) IsZero() bool { return v == RedactedRemoteURL{} }

func (v RedactedRemoteURL) Validate() error {
	if !v.RedactionState.Valid() || validateOpaque("redacted URL", v.DisplayRedacted) != nil {
		return invalid("redacted remote URL")
	}
	if (v.RedactionState == RedactionOpaque) != (v.DisplayRedacted == OpaqueMarker) {
		return invalid("opaque remote URL marker")
	}
	if v.RedactionState == RedactionNotNeeded && v.CredentialMaterialDetected {
		return invalid("unredacted credential material")
	}
	if v.OriginalDigest != "" {
		return v.OriginalDigest.Validate()
	}
	return nil
}

type RemoteRedacted struct {
	Name             string
	SourceScope      string
	SourceDigest     ContentDigest
	ObservationState ObservationState
	fetchURLs        []RedactedRemoteURL
	pushURLs         []RedactedRemoteURL
}

type RemoteRedactedInput struct {
	Name             string
	FetchURLs        []RedactedRemoteURL
	PushURLs         []RedactedRemoteURL
	SourceScope      string
	SourceDigest     ContentDigest
	ObservationState ObservationState
}

func NewRemoteRedacted(in RemoteRedactedInput) (RemoteRedacted, error) {
	v := RemoteRedacted{
		Name:             in.Name,
		SourceScope:      in.SourceScope,
		SourceDigest:     in.SourceDigest,
		ObservationState: in.ObservationState,
		fetchURLs:        cloneSlice(in.FetchURLs),
		pushURLs:         cloneSlice(in.PushURLs),
	}
	return v, v.Validate()
}

func (v RemoteRedacted) Validate() error {
	if err := firstError(
		validateOpaque("remote name", v.Name),
		validateOpaque("remote source scope", v.SourceScope),
		v.SourceDigest.Validate(),
	); err != nil {
		return err
	}
	urls := len(v.fetchURLs) + len(v.pushURLs)
	switch v.ObservationState {
	case ObservationObserved:
		if urls == 0 {
			return invalid("observed remote without URLs")
		}
	case ObservationObservedEmpty, ObservationError:
		if urls != 0 {
			return invalid("remote observation state")
		}
	default:
		return invalid("remote observation state")
	}
	for _, list := range [][]RedactedRemoteURL{v.fetchURLs, v.pushURLs} {
		for i, url := range list {
			if url.Ordinal != uint64(i) {
				return invalid("remote URL order")
			}
			if err := url.Validate(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (v RemoteRedacted) FetchURLs() []RedactedRemoteURL { return cloneSlice(v.fetchURLs) }
func (v RemoteRedacted) PushURLs() []RedactedRemoteURL  { return cloneSlice(v.pushURLs) }

type GitFlowClassification string

const (
	GitFlowAbsent              GitFlowClassification = "absent"
	GitFlowCompleteAVH         GitFlowClassification = "complete_avh"
	GitFlowCompleteGitFlowNext GitFlowClassification = "complete_git_flow_next"
	GitFlowPartial             GitFlowClassification = "partial"
	GitFlowMixed               GitFlowClassification = "mixed"
	GitFlowContradictory       GitFlowClassification = "contradictory"
	GitFlowUnrecognized        GitFlowClassification = "unrecognized"
	GitFlowUnobservable        GitFlowClassification = "unobservable"
)

func (v GitFlowClassification) Valid() bool {
	switch v {
	case GitFlowAbsent, GitFlowCompleteAVH, GitFlowCompleteGitFlowNext, GitFlowPartial, GitFlowMixed,
		GitFlowContradictory, GitFlowUnrecognized, GitFlowUnobservable:
		return true
	default:
		return false
	}
}

// GitFlowEntry is one safely parsed gitflow.* key/value with its provenance.
type GitFlowEntry struct {
	Key          string
	Value        string
	Origin       string
	SourceDigest ContentDigest
}

func (v GitFlowEntry) Validate() error {
	if !strings.HasPrefix(v.Key, "gitflow.") || validateOpaque("Git Flow key", v.Key) != nil ||
		validateOpaque("Git Flow origin", v.Origin) != nil || !utf8.ValidString(v.Value) {
		return invalid("Git Flow entry")
	}
	return v.SourceDigest.Validate()
}

// GitFlowEvidence is observed profile evidence. It selects no strategy and
// grants no authorization. Equality of a complete classification with its
// normative profile is established by the inspector that builds the value.
type GitFlowEvidence struct {
	ObservationState  ObservationState
	Classification    GitFlowClassification
	Dialect           GitFlowDialect
	ProductionBranch  string
	DevelopmentBranch string
	entries           []GitFlowEntry
	prefixes          []GitFlowEntry
	instanceBases     []GitFlowEntry
	sourceDigests     []ContentDigest
	issues            []string
}

type GitFlowEvidenceInput struct {
	ObservationState  ObservationState
	Classification    GitFlowClassification
	Dialect           GitFlowDialect
	Entries           []GitFlowEntry
	ProductionBranch  string
	DevelopmentBranch string
	Prefixes          []GitFlowEntry
	InstanceBases     []GitFlowEntry
	SourceDigests     []ContentDigest
	Issues            []string
}

func NewGitFlowEvidence(in GitFlowEvidenceInput) (GitFlowEvidence, error) {
	v := GitFlowEvidence{
		ObservationState:  in.ObservationState,
		Classification:    in.Classification,
		Dialect:           in.Dialect,
		ProductionBranch:  in.ProductionBranch,
		DevelopmentBranch: in.DevelopmentBranch,
		entries:           cloneSlice(in.Entries),
		prefixes:          cloneSlice(in.Prefixes),
		instanceBases:     cloneSlice(in.InstanceBases),
		sourceDigests:     cloneSlice(in.SourceDigests),
		issues:            cloneSlice(in.Issues),
	}
	return v, v.Validate()
}

func (v GitFlowEvidence) Validate() error {
	if !v.ObservationState.Valid() || !v.Classification.Valid() {
		return invalid("Git Flow evidence")
	}
	observedData := len(v.entries) + len(v.prefixes) + len(v.instanceBases)
	switch v.Classification {
	case GitFlowAbsent:
		if (v.ObservationState != ObservationObservedEmpty && v.ObservationState != ObservationAbsent) ||
			observedData != 0 || v.Dialect != "" || v.ProductionBranch != "" || v.DevelopmentBranch != "" {
			return invalid("absent Git Flow evidence")
		}
	case GitFlowUnobservable:
		if !v.ObservationState.unreliable() || observedData != 0 || len(v.issues) == 0 || v.Dialect != "" {
			return invalid("unobservable Git Flow evidence")
		}
	case GitFlowCompleteAVH, GitFlowCompleteGitFlowNext:
		want := GitFlowDialectAVH
		if v.Classification == GitFlowCompleteGitFlowNext {
			want = GitFlowDialectNext
		}
		if v.ObservationState != ObservationObserved || v.Dialect != want || len(v.entries) == 0 || len(v.prefixes) == 0 ||
			validateOpaque("production branch", v.ProductionBranch) != nil ||
			validateOpaque("development branch", v.DevelopmentBranch) != nil {
			return invalid("complete Git Flow evidence")
		}
	default: // partial, mixed, contradictory, unrecognized
		if v.ObservationState != ObservationObserved || observedData == 0 || len(v.sourceDigests) == 0 {
			return invalid("incomplete Git Flow evidence")
		}
		if v.Dialect != "" && (v.Classification != GitFlowPartial || !v.Dialect.Valid()) {
			return invalid("Git Flow evidence dialect")
		}
	}
	for _, list := range [][]GitFlowEntry{v.entries, v.prefixes, v.instanceBases} {
		for i, entry := range list {
			if err := entry.Validate(); err != nil {
				return err
			}
			if i > 0 && !gitFlowEntryLess(list[i-1], entry) {
				return invalid("Git Flow entry order")
			}
		}
	}
	return firstError(validateEach(v.sourceDigests), validateLabels("Git Flow issue", v.issues))
}

func gitFlowEntryLess(a, b GitFlowEntry) bool {
	if a.Key != b.Key {
		return a.Key < b.Key
	}
	return a.Origin < b.Origin
}

func (v GitFlowEvidence) Entries() []GitFlowEntry        { return cloneSlice(v.entries) }
func (v GitFlowEvidence) Prefixes() []GitFlowEntry       { return cloneSlice(v.prefixes) }
func (v GitFlowEvidence) InstanceBases() []GitFlowEntry  { return cloneSlice(v.instanceBases) }
func (v GitFlowEvidence) SourceDigests() []ContentDigest { return cloneSlice(v.sourceDigests) }
func (v GitFlowEvidence) Issues() []string               { return cloneSlice(v.issues) }

type AttributeSourceKind string

const (
	AttributeSourceSystem         AttributeSourceKind = "system_attributes"
	AttributeSourceGlobalExplicit AttributeSourceKind = "global_attributes_explicit"
	AttributeSourceGlobalDefault  AttributeSourceKind = "global_attributes_default"
	AttributeSourceRepository     AttributeSourceKind = "repository_gitattributes"
	AttributeSourceInfo           AttributeSourceKind = "info_attributes"
	AttributeSourcePostInitIntent AttributeSourceKind = "post_init_intent"
)

func (v AttributeSourceKind) Valid() bool {
	switch v {
	case AttributeSourceSystem, AttributeSourceGlobalExplicit, AttributeSourceGlobalDefault,
		AttributeSourceRepository, AttributeSourceInfo, AttributeSourcePostInitIntent:
		return true
	default:
		return false
	}
}

// AttributeSource is nominally distinct from IgnoreSource: their catalogues
// and precedence rules are not interchangeable.
type AttributeSource struct {
	Identity InspectedSourceIdentity
	Kind     AttributeSourceKind
}

func (v AttributeSource) Validate() error {
	if !v.Kind.Valid() {
		return invalid("attribute source kind")
	}
	if v.Kind == AttributeSourceRepository && v.Identity.Path.IsZero() {
		return invalid("repository attribute source path")
	}
	// Known post-init intent never fabricates an observed filesystem source.
	if v.Kind == AttributeSourcePostInitIntent && v.Identity.PhysicalIdentityDigest != "" {
		return invalid("post-init attribute source identity")
	}
	return v.Identity.Validate()
}

type IgnoreSourceKind string

const (
	IgnoreSourceCoreExcludesFile IgnoreSourceKind = "core_excludes_file"
	IgnoreSourceInfoExclude      IgnoreSourceKind = "info_exclude"
	IgnoreSourceRepository       IgnoreSourceKind = "repository_gitignore"
	IgnoreSourcePostInitIntent   IgnoreSourceKind = "post_init_intent"
)

func (v IgnoreSourceKind) Valid() bool {
	switch v {
	case IgnoreSourceCoreExcludesFile, IgnoreSourceInfoExclude, IgnoreSourceRepository, IgnoreSourcePostInitIntent:
		return true
	default:
		return false
	}
}

type IgnoreSource struct {
	Identity          InspectedSourceIdentity
	Kind              IgnoreSourceKind
	BaseDirectory     ObservedPath
	DirectoryDepth    uint64
	HasDirectoryDepth bool
}

func (v IgnoreSource) Validate() error {
	if !v.Kind.Valid() {
		return invalid("ignore source kind")
	}
	if v.Kind == IgnoreSourceRepository && (v.BaseDirectory.IsZero() || !v.HasDirectoryDepth || v.Identity.Path.IsZero()) {
		return invalid("repository ignore source location")
	}
	if !v.HasDirectoryDepth && v.DirectoryDepth != 0 {
		return invalid("ignore source directory depth")
	}
	if v.Kind == IgnoreSourcePostInitIntent && v.Identity.PhysicalIdentityDigest != "" {
		return invalid("post-init ignore source identity")
	}
	return firstError(validateOptionalPath(v.BaseDirectory), v.Identity.Validate())
}

// IgnoreEvaluation links one candidate to the ordered ignore sources that
// decided it and, when ignored, to its winning source.
type IgnoreEvaluation struct {
	Path                  ObservedPath
	Ignored               bool
	WinningRuleSource     ContentDigest
	EquivalenceProven     bool
	IncompatibilityReason Reason
	orderedSourceDigests  []ContentDigest
	matchedRules          []string
}

type IgnoreEvaluationInput struct {
	Path                  ObservedPath
	OrderedSourceDigests  []ContentDigest
	MatchedRules          []string
	Ignored               bool
	WinningRuleSource     ContentDigest
	EquivalenceProven     bool
	IncompatibilityReason Reason
}

func NewIgnoreEvaluation(in IgnoreEvaluationInput) (IgnoreEvaluation, error) {
	v := IgnoreEvaluation{
		Path:                  in.Path,
		Ignored:               in.Ignored,
		WinningRuleSource:     in.WinningRuleSource,
		EquivalenceProven:     in.EquivalenceProven,
		IncompatibilityReason: in.IncompatibilityReason,
		orderedSourceDigests:  cloneSlice(in.OrderedSourceDigests),
		matchedRules:          cloneSlice(in.MatchedRules),
	}
	return v, v.Validate()
}

func (v IgnoreEvaluation) Validate() error {
	if err := firstError(v.Path.Validate(), validateEach(v.orderedSourceDigests)); err != nil {
		return err
	}
	if v.Ignored && v.WinningRuleSource == "" {
		return invalid("ignored evaluation winning source")
	}
	if v.WinningRuleSource != "" && !containsValue(v.orderedSourceDigests, v.WinningRuleSource) {
		return invalid("ignore evaluation winning source")
	}
	if v.IncompatibilityReason != "" && (v.IncompatibilityReason != ReasonAttributesIncompatible || v.EquivalenceProven) {
		return invalid("ignore evaluation incompatibility")
	}
	for _, rule := range v.matchedRules {
		if rule == "" || !utf8.ValidString(rule) {
			return invalid("ignore evaluation matched rule")
		}
	}
	return nil
}

func (v IgnoreEvaluation) OrderedSourceDigests() []ContentDigest {
	return cloneSlice(v.orderedSourceDigests)
}
func (v IgnoreEvaluation) MatchedRules() []string { return cloneSlice(v.matchedRules) }

type ConversionRisk string

const (
	ConversionRiskNone         ConversionRisk = "none"
	ConversionRiskNeutral      ConversionRisk = "neutral"
	ConversionRiskMayTransform ConversionRisk = "may_transform"
	ConversionRiskExternalCode ConversionRisk = "external_code"
	ConversionRiskUnknown      ConversionRisk = "unknown"
	ConversionRiskIncompatible ConversionRisk = "incompatible"
)

func (v ConversionRisk) Valid() bool {
	switch v {
	case ConversionRiskNone, ConversionRiskNeutral, ConversionRiskMayTransform,
		ConversionRiskExternalCode, ConversionRiskUnknown, ConversionRiskIncompatible:
		return true
	default:
		return false
	}
}

// EffectiveConversionSetting keeps the configured and effective values of
// one conversion mechanism distinct, together with their origin.
type EffectiveConversionSetting struct {
	Key                     string
	ConfiguredValueRedacted string
	EffectiveValueRedacted  string
	SourceScope             string
	SourceDigest            ContentDigest
	InterpretationState     string
}

func (v EffectiveConversionSetting) Validate() error {
	return firstError(
		validateOpaque("conversion setting key", v.Key),
		validateOpaque("conversion setting scope", v.SourceScope),
		validateOpaque("conversion interpretation state", v.InterpretationState),
		v.SourceDigest.Validate(),
	)
}

// ConversionSettings never results from running a filter or configured
// command; it only records what safe in-process parsing established.
type ConversionSettings struct {
	ObservationState      ObservationState
	ExternalCodeRequired  bool
	ByteIdentityProven    bool
	Risk                  ConversionRisk
	IncompatibilityReason Reason
	settings              []EffectiveConversionSetting
	filterDrivers         []string
	sourceDigests         []ContentDigest
}

type ConversionSettingsInput struct {
	ObservationState      ObservationState
	Settings              []EffectiveConversionSetting
	FilterDrivers         []string
	ExternalCodeRequired  bool
	ByteIdentityProven    bool
	Risk                  ConversionRisk
	SourceDigests         []ContentDigest
	IncompatibilityReason Reason
}

func NewConversionSettings(in ConversionSettingsInput) (ConversionSettings, error) {
	v := ConversionSettings{
		ObservationState:      in.ObservationState,
		ExternalCodeRequired:  in.ExternalCodeRequired,
		ByteIdentityProven:    in.ByteIdentityProven,
		Risk:                  in.Risk,
		IncompatibilityReason: in.IncompatibilityReason,
		settings:              cloneSlice(in.Settings),
		filterDrivers:         cloneSlice(in.FilterDrivers),
		sourceDigests:         cloneSlice(in.SourceDigests),
	}
	return v, v.Validate()
}

func (v ConversionSettings) Validate() error {
	if !v.ObservationState.Valid() || !v.Risk.Valid() {
		return invalid("conversion settings")
	}
	relevant := len(v.settings) + len(v.filterDrivers)
	switch v.Risk {
	case ConversionRiskNone:
		if v.ObservationState != ObservationObservedEmpty || relevant != 0 {
			return invalid("conversion risk none")
		}
	case ConversionRiskNeutral:
		if v.ObservationState != ObservationObserved || relevant == 0 {
			return invalid("conversion risk neutral")
		}
	case ConversionRiskExternalCode:
		if !v.ExternalCodeRequired {
			return invalid("conversion risk external code")
		}
	}
	unproven := v.ExternalCodeRequired || (v.Risk != ConversionRiskNone && v.Risk != ConversionRiskNeutral)
	if v.ByteIdentityProven && unproven {
		return invalid("conversion byte identity proof")
	}
	if v.ObservationState.unreliable() && v.Risk != ConversionRiskUnknown && v.Risk != ConversionRiskIncompatible {
		return invalid("conversion observation state")
	}
	if (v.Risk == ConversionRiskIncompatible) != (v.IncompatibilityReason == ReasonAttributesIncompatible) ||
		(v.IncompatibilityReason != "" && v.IncompatibilityReason != ReasonAttributesIncompatible) {
		return invalid("conversion incompatibility reason")
	}
	return firstError(validateEach(v.settings), validateLabels("filter driver", v.filterDrivers), validateEach(v.sourceDigests))
}

func (v ConversionSettings) Settings() []EffectiveConversionSetting { return cloneSlice(v.settings) }
func (v ConversionSettings) FilterDrivers() []string                { return cloneSlice(v.filterDrivers) }
func (v ConversionSettings) SourceDigests() []ContentDigest         { return cloneSlice(v.sourceDigests) }

// HeadObservation is the HEAD state together with the facts that each state
// requires or prohibits.
type HeadObservation struct {
	State          HeadState
	SymbolicRef    ObservedRef
	CommitOID      ObjectOID
	HistoryPresent bool
}

func (v HeadObservation) Validate() error {
	if !v.State.Valid() {
		return invalid("HEAD state")
	}
	if !v.SymbolicRef.IsZero() {
		if err := v.SymbolicRef.Validate(); err != nil {
			return err
		}
	}
	if v.CommitOID != "" {
		if err := v.CommitOID.Validate(); err != nil {
			return err
		}
	}
	switch v.State {
	case HeadStateAttached:
		if v.SymbolicRef.IsZero() || v.CommitOID == "" || !v.HistoryPresent {
			return invalid("attached HEAD")
		}
	case HeadStateDetached:
		if !v.SymbolicRef.IsZero() || v.CommitOID == "" || !v.HistoryPresent {
			return invalid("detached HEAD")
		}
	case HeadStateUnborn, HeadStateInvalid:
		// An unborn HEAD may name the branch it will create; other refs may
		// still carry history.
		if v.CommitOID != "" {
			return invalid(string(v.State) + " HEAD commit")
		}
	}
	return nil
}

type WorktreeAvailability string

const (
	WorktreeAvailable    WorktreeAvailability = "available"
	WorktreeMissing      WorktreeAvailability = "missing"
	WorktreeLocked       WorktreeAvailability = "locked"
	WorktreeInaccessible WorktreeAvailability = "inaccessible"
	WorktreeAmbiguous    WorktreeAvailability = "ambiguous"
)

func (v WorktreeAvailability) Valid() bool {
	switch v {
	case WorktreeAvailable, WorktreeMissing, WorktreeLocked, WorktreeInaccessible, WorktreeAmbiguous:
		return true
	default:
		return false
	}
}

// LinkedWorktree is a read-only observation. Lock and prunable fields are
// never mutation authority.
type LinkedWorktree struct {
	ObservationState   ObservationState
	WorktreePath       ObservedPath
	GitDirPath         ObservedPath
	CommonDirIdentity  ContentDigest
	Head               HeadObservation
	Availability       WorktreeAvailability
	Locked             bool
	LockReasonRedacted string
	PrunableObserved   bool
	Prunable           bool
}

func (v LinkedWorktree) Validate() error {
	if (v.ObservationState != ObservationObserved && v.ObservationState != ObservationError) || !v.Availability.Valid() {
		return invalid("linked worktree")
	}
	if err := firstError(v.WorktreePath.Validate(), validateOptionalPath(v.GitDirPath), v.Head.Validate()); err != nil {
		return err
	}
	if v.Availability == WorktreeAvailable && (v.ObservationState != ObservationObserved || v.CommonDirIdentity == "") {
		return invalid("available linked worktree association")
	}
	if v.CommonDirIdentity != "" {
		if err := v.CommonDirIdentity.Validate(); err != nil {
			return err
		}
	}
	if (v.Availability == WorktreeLocked && !v.Locked) || (!v.Locked && v.LockReasonRedacted != "") ||
		(!v.PrunableObserved && v.Prunable) {
		return invalid("linked worktree lock observation")
	}
	if v.LockReasonRedacted != "" {
		return validateOpaque("worktree lock reason", v.LockReasonRedacted)
	}
	return nil
}

type SubmoduleInitialization string

const (
	SubmoduleDeclaredUninitialized SubmoduleInitialization = "declared_uninitialized"
	SubmoduleInitializedMatching   SubmoduleInitialization = "initialized_matching"
	SubmoduleInitializedMismatched SubmoduleInitialization = "initialized_mismatched"
	SubmoduleMissing               SubmoduleInitialization = "missing"
	SubmoduleInaccessible          SubmoduleInitialization = "inaccessible"
	SubmoduleAmbiguous             SubmoduleInitialization = "ambiguous"
)

func (v SubmoduleInitialization) Valid() bool {
	switch v {
	case SubmoduleDeclaredUninitialized, SubmoduleInitializedMatching, SubmoduleInitializedMismatched,
		SubmoduleMissing, SubmoduleInaccessible, SubmoduleAmbiguous:
		return true
	default:
		return false
	}
}

// Submodule keeps the superproject declaration distinct from the
// non-recursive filesystem observation.
type Submodule struct {
	ObservationState       ObservationState
	Name                   string
	Path                   ObservedPath
	RecordedCommitOID      ObjectOID
	URLRedacted            RedactedRemoteURL
	DeclarationDigest      ContentDigest
	InitializationState    SubmoduleInitialization
	ObservedPathIdentity   ContentDigest
	ObservedHeadOID        ObjectOID
	ObservedRepositoryKind RepositoryKind
}

func (v Submodule) Validate() error {
	if (v.ObservationState != ObservationObserved && v.ObservationState != ObservationError) || !v.InitializationState.Valid() {
		return invalid("submodule")
	}
	if err := firstError(
		validateOpaque("submodule name", v.Name),
		v.Path.Validate(),
		v.RecordedCommitOID.Validate(),
		v.DeclarationDigest.Validate(),
	); err != nil {
		return err
	}
	if !v.URLRedacted.IsZero() {
		if err := v.URLRedacted.Validate(); err != nil {
			return err
		}
	}
	switch v.InitializationState {
	case SubmoduleInitializedMatching:
		if v.ObservedPathIdentity == "" || v.ObservedHeadOID != v.RecordedCommitOID {
			return invalid("matching submodule observation")
		}
	case SubmoduleInitializedMismatched:
		if v.ObservedHeadOID == "" || v.ObservedHeadOID == v.RecordedCommitOID {
			return invalid("mismatched submodule observation")
		}
	case SubmoduleDeclaredUninitialized:
		if v.ObservedPathIdentity != "" || v.ObservedHeadOID != "" {
			return invalid("uninitialized submodule observation")
		}
	}
	if v.ObservedPathIdentity != "" {
		if err := v.ObservedPathIdentity.Validate(); err != nil {
			return err
		}
	}
	if v.ObservedHeadOID != "" {
		if err := v.ObservedHeadOID.Validate(); err != nil {
			return err
		}
	}
	if v.ObservedRepositoryKind != "" && !v.ObservedRepositoryKind.Valid() {
		return invalid("submodule repository kind")
	}
	return nil
}

// FilesystemRole names the location a capability profile describes.
type FilesystemRole string

const (
	FilesystemRoleProject          FilesystemRole = "project"
	FilesystemRoleStateRoot        FilesystemRole = "state_root"
	FilesystemRolePlanOutputParent FilesystemRole = "plan_output_parent"
	FilesystemRolePreparedIndex    FilesystemRole = "prepared_index"
)

func (v FilesystemRole) Valid() bool {
	return v == FilesystemRoleProject || v == FilesystemRoleStateRoot ||
		v == FilesystemRolePlanOutputParent || v == FilesystemRolePreparedIndex
}

type FilesystemCapabilityProfile struct {
	Role                       FilesystemRole
	GOOS                       string
	FilesystemType             string
	MountIdentity              string
	DeviceDecimal              string
	InodeDecimal               string
	Local                      bool
	ReadOnly                   bool
	ProfileVersion             string
	ExclusiveCreate            bool
	AdvisoryLock               bool
	FileSync                   bool
	DirectorySync              bool
	AtomicSameFilesystemRename bool
	AtomicReplace              bool
	StableIdentity             bool
	ProductionSupported        bool
	ProbeEvidenceDigest        ContentDigest
	mountFlags                 []string
}

type FilesystemCapabilityProfileInput struct {
	Role                       FilesystemRole
	GOOS                       string
	FilesystemType             string
	MountIdentity              string
	DeviceDecimal              string
	InodeDecimal               string
	MountFlags                 []string
	Local                      bool
	ReadOnly                   bool
	ProfileVersion             string
	ExclusiveCreate            bool
	AdvisoryLock               bool
	FileSync                   bool
	DirectorySync              bool
	AtomicSameFilesystemRename bool
	AtomicReplace              bool
	StableIdentity             bool
	ProductionSupported        bool
	ProbeEvidenceDigest        ContentDigest
}

func NewFilesystemCapabilityProfile(in FilesystemCapabilityProfileInput) (FilesystemCapabilityProfile, error) {
	v := FilesystemCapabilityProfile{
		Role:                       in.Role,
		GOOS:                       in.GOOS,
		FilesystemType:             in.FilesystemType,
		MountIdentity:              in.MountIdentity,
		DeviceDecimal:              in.DeviceDecimal,
		InodeDecimal:               in.InodeDecimal,
		Local:                      in.Local,
		ReadOnly:                   in.ReadOnly,
		ProfileVersion:             in.ProfileVersion,
		ExclusiveCreate:            in.ExclusiveCreate,
		AdvisoryLock:               in.AdvisoryLock,
		FileSync:                   in.FileSync,
		DirectorySync:              in.DirectorySync,
		AtomicSameFilesystemRename: in.AtomicSameFilesystemRename,
		AtomicReplace:              in.AtomicReplace,
		StableIdentity:             in.StableIdentity,
		ProductionSupported:        in.ProductionSupported,
		ProbeEvidenceDigest:        in.ProbeEvidenceDigest,
		mountFlags:                 cloneSlice(in.MountFlags),
	}
	return v, v.Validate()
}

func (v FilesystemCapabilityProfile) Validate() error {
	if !v.Role.Valid() {
		return invalid("filesystem capability role")
	}
	if err := firstError(
		validateOpaque("filesystem GOOS", v.GOOS),
		validateOpaque("filesystem type", v.FilesystemType),
		validateOpaque("filesystem mount identity", v.MountIdentity),
		validateOpaque("filesystem profile version", v.ProfileVersion),
		ValidateDecimalIdentity(v.DeviceDecimal),
		ValidateDecimalIdentity(v.InodeDecimal),
		validateLabels("filesystem mount flag", v.mountFlags),
	); err != nil {
		return err
	}
	primitives := v.ExclusiveCreate && v.AdvisoryLock && v.FileSync && v.DirectorySync &&
		v.AtomicSameFilesystemRename && v.AtomicReplace && v.StableIdentity
	if v.ProductionSupported && (!v.Local || v.ReadOnly || !primitives) {
		return invalid("filesystem production support")
	}
	if v.ProbeEvidenceDigest != "" {
		return v.ProbeEvidenceDigest.Validate()
	}
	return nil
}

func (v FilesystemCapabilityProfile) MountFlags() []string { return cloneSlice(v.mountFlags) }

// SequencerState records an interrupted multi-step Git operation observed in
// the Git directory. Any value other than SequencerNone makes every plan
// inapplicable (O-103B-1); the planner, not the model, enforces that.
type SequencerState string

const (
	SequencerNone       SequencerState = "none"
	SequencerMerge      SequencerState = "merge"
	SequencerRebase     SequencerState = "rebase"
	SequencerCherryPick SequencerState = "cherry_pick"
	SequencerRevert     SequencerState = "revert"
	SequencerBisect     SequencerState = "bisect"
	SequencerAm         SequencerState = "am"
	SequencerUnknown    SequencerState = "unknown"
)

func (v SequencerState) Valid() bool {
	switch v {
	case SequencerNone, SequencerMerge, SequencerRebase, SequencerCherryPick, SequencerRevert,
		SequencerBisect, SequencerAm, SequencerUnknown:
		return true
	default:
		return false
	}
}

// RefObservation is one row of the bounded ref snapshot. HEAD is reported by
// HeadObservation, never here.
type RefObservation struct {
	Name      ObservedRef
	TargetOID ObjectOID
	PeeledOID ObjectOID
}

func (v RefObservation) Validate() error {
	if err := firstError(v.Name.Validate(), v.TargetOID.Validate()); err != nil {
		return err
	}
	if v.PeeledOID != "" {
		return v.PeeledOID.Validate()
	}
	return nil
}

type IndexPresence string

const (
	IndexAbsent     IndexPresence = "absent"
	IndexPresent    IndexPresence = "present"
	IndexUnreadable IndexPresence = "unreadable"
)

func (v IndexPresence) Valid() bool {
	return v == IndexAbsent || v == IndexPresent || v == IndexUnreadable
}

// LockFileState reports a Git lock file such as index.lock or config.lock.
type LockFileState string

const (
	LockFileAbsent       LockFileState = "absent"
	LockFilePresent      LockFileState = "present"
	LockFileUnobservable LockFileState = "unobservable"
)

func (v LockFileState) Valid() bool {
	return v == LockFileAbsent || v == LockFilePresent || v == LockFileUnobservable
}

type IndexChangeKind string

const (
	IndexChangeStaged      IndexChangeKind = "staged"
	IndexChangeConflicted  IndexChangeKind = "conflicted"
	IndexChangeTypeChanged IndexChangeKind = "type_changed"
	IndexChangeModeChanged IndexChangeKind = "mode_changed"
)

func (v IndexChangeKind) Valid() bool {
	return v == IndexChangeStaged || v == IndexChangeConflicted || v == IndexChangeTypeChanged || v == IndexChangeModeChanged
}

type IndexChange struct {
	Path ObservedPath
	Kind IndexChangeKind
}

func (v IndexChange) Validate() error {
	if !v.Kind.Valid() {
		return invalid("index change kind")
	}
	return v.Path.Validate()
}

// IndexObservation describes the normal index without trusting Git to have
// refreshed it.
type IndexObservation struct {
	Presence          IndexPresence
	Version           uint32
	EntryCountDecimal string
	ContentDigest     ContentDigest
	LockState         LockFileState
	extensions        []string
	changes           []IndexChange
}

type IndexObservationInput struct {
	Presence          IndexPresence
	Version           uint32
	EntryCountDecimal string
	ContentDigest     ContentDigest
	LockState         LockFileState
	Extensions        []string
	Changes           []IndexChange
}

func NewIndexObservation(in IndexObservationInput) (IndexObservation, error) {
	v := IndexObservation{
		Presence:          in.Presence,
		Version:           in.Version,
		EntryCountDecimal: in.EntryCountDecimal,
		ContentDigest:     in.ContentDigest,
		LockState:         in.LockState,
		extensions:        cloneSlice(in.Extensions),
		changes:           cloneSlice(in.Changes),
	}
	return v, v.Validate()
}

func (v IndexObservation) Validate() error {
	if !v.Presence.Valid() || !v.LockState.Valid() {
		return invalid("index observation")
	}
	if v.Presence != IndexPresent {
		if v.Version != 0 || v.EntryCountDecimal != "" || v.ContentDigest != "" || len(v.extensions) != 0 || len(v.changes) != 0 {
			return invalid(string(v.Presence) + " index observation")
		}
		return nil
	}
	if v.Version < 2 || v.Version > 4 {
		return invalid("index version")
	}
	if err := firstError(ValidateDecimalIdentity(v.EntryCountDecimal), v.ContentDigest.Validate(), validateEach(v.changes)); err != nil {
		return err
	}
	for _, signature := range v.extensions {
		if len(signature) != 4 {
			return invalid("index extension signature")
		}
	}
	if v.Empty() && len(v.changes) != 0 {
		return invalid("changes in an empty index")
	}
	for i := 1; i < len(v.changes); i++ {
		if !pathKindLess(v.changes[i-1].Path, string(v.changes[i-1].Kind), v.changes[i].Path, string(v.changes[i].Kind)) {
			return invalid("index change order")
		}
	}
	return nil
}

// Empty reports whether the index holds no entries; an absent index is empty.
func (v IndexObservation) Empty() bool {
	return v.Presence == IndexAbsent || (v.Presence == IndexPresent && v.EntryCountDecimal == "0")
}

func (v IndexObservation) Extensions() []string   { return cloneSlice(v.extensions) }
func (v IndexObservation) Changes() []IndexChange { return cloneSlice(v.changes) }

type WorktreeChangeKind string

const (
	WorktreeChangeUnstaged    WorktreeChangeKind = "unstaged"
	WorktreeChangeUntracked   WorktreeChangeKind = "untracked"
	WorktreeChangeIgnored     WorktreeChangeKind = "ignored"
	WorktreeChangeTypeChanged WorktreeChangeKind = "type_changed"
	WorktreeChangeModeChanged WorktreeChangeKind = "mode_changed"
)

func (v WorktreeChangeKind) Valid() bool {
	switch v {
	case WorktreeChangeUnstaged, WorktreeChangeUntracked, WorktreeChangeIgnored,
		WorktreeChangeTypeChanged, WorktreeChangeModeChanged:
		return true
	default:
		return false
	}
}

type WorktreeChange struct {
	Path ObservedPath
	Kind WorktreeChangeKind
}

func (v WorktreeChange) Validate() error {
	if !v.Kind.Valid() {
		return invalid("worktree change kind")
	}
	return v.Path.Validate()
}

// WorktreePathIdentity freezes one path whose identity a plan relies on or
// must preserve. Regular files also carry their content digest.
type WorktreePathIdentity struct {
	Path          ObservedPath
	Identity      FileIdentity
	ContentDigest ContentDigest
}

func (v WorktreePathIdentity) Validate() error {
	if err := firstError(v.Path.Validate(), v.Identity.Validate()); err != nil {
		return err
	}
	if v.Identity.Mode.IsRegular() != (v.ContentDigest != "") {
		return invalid("worktree path content digest")
	}
	if v.ContentDigest != "" {
		return v.ContentDigest.Validate()
	}
	return nil
}

type ConfigScope string

const (
	ConfigScopeSystem   ConfigScope = "system"
	ConfigScopeXDG      ConfigScope = "xdg"
	ConfigScopeGlobal   ConfigScope = "global"
	ConfigScopeLocal    ConfigScope = "local"
	ConfigScopeWorktree ConfigScope = "worktree"
	ConfigScopeCommand  ConfigScope = "command"
)

func (v ConfigScope) Valid() bool {
	switch v {
	case ConfigScopeSystem, ConfigScopeXDG, ConfigScopeGlobal, ConfigScopeLocal, ConfigScopeWorktree, ConfigScopeCommand:
		return true
	default:
		return false
	}
}

// ConfigEntry is one safely parsed configuration entry in Git resolution
// order. Its value is redacted by the same rules as remote URLs.
type ConfigEntry struct {
	Ordinal        uint64
	Key            string
	ValuePresent   bool
	ValueRedacted  string
	ValueRedaction RedactionState
	Scope          ConfigScope
	Origin         ObservedPath
	SourceDigest   ContentDigest
}

func (v ConfigEntry) Validate() error {
	if !v.ValueRedaction.Valid() || !v.Scope.Valid() {
		return invalid("config entry")
	}
	if err := validateConfigKey(v.Key); err != nil {
		return err
	}
	if !v.ValuePresent && (v.ValueRedacted != "" || v.ValueRedaction != RedactionNotNeeded) {
		return invalid("config entry without value")
	}
	if !utf8.ValidString(v.ValueRedacted) || strings.ContainsRune(v.ValueRedacted, 0) {
		return invalid("config entry value")
	}
	if (v.ValueRedaction == RedactionOpaque) != (v.ValueRedacted == OpaqueMarker) {
		return invalid("opaque config value marker")
	}
	// Only command-line overrides lack a file origin.
	if v.Scope == ConfigScopeCommand {
		if !v.Origin.IsZero() || v.SourceDigest != "" {
			return invalid("command config origin")
		}
		return nil
	}
	return firstError(v.Origin.Validate(), v.SourceDigest.Validate())
}

// ValidateConfigKey reports whether key is a canonical Git configuration
// key: section[.subsection].name with lowercase section and name, where the
// subsection keeps its exact bytes.
func ValidateConfigKey(key string) error { return validateConfigKey(key) }

func validateConfigKey(key string) error {
	first, last := strings.IndexByte(key, '.'), strings.LastIndexByte(key, '.')
	if first <= 0 || last == len(key)-1 || !utf8.ValidString(key) {
		return invalid("config key")
	}
	if !configNamePart(key[:first], false) || !configNamePart(key[last+1:], true) {
		return invalid("config key")
	}
	for _, r := range key[first+1 : last+1] {
		if r == 0 || r == '\n' {
			return invalid("config subsection")
		}
	}
	return nil
}

// configNamePart reports whether s is a lowercase Git section or variable
// name; a variable name must begin with a letter.
func configNamePart(s string, letterFirst bool) bool {
	if s == "" {
		return false
	}
	for i, c := range s {
		letter := c >= 'a' && c <= 'z'
		if !letter && c != '-' && !(c >= '0' && c <= '9') {
			return false
		}
		if letterFirst && i == 0 && !letter {
			return false
		}
	}
	return true
}

type ProjectConfigState string

const (
	ProjectConfigAbsent     ProjectConfigState = "absent"
	ProjectConfigValid      ProjectConfigState = "valid"
	ProjectConfigInvalid    ProjectConfigState = "invalid"
	ProjectConfigUnreadable ProjectConfigState = "unreadable"
)

func (v ProjectConfigState) Valid() bool {
	return v == ProjectConfigAbsent || v == ProjectConfigValid || v == ProjectConfigInvalid || v == ProjectConfigUnreadable
}

// ProjectConfigObservation decides between a verified no-op and a conflict
// for the planned .ai-dev/config.json write.
type ProjectConfigObservation struct {
	State         ProjectConfigState
	Path          ObservedPath
	Identity      FileIdentity
	ContentDigest ContentDigest
	ProjectID     ProjectID
}

func (v ProjectConfigObservation) Validate() error {
	if !v.State.Valid() {
		return invalid("project config state")
	}
	if err := v.Path.Validate(); err != nil {
		return err
	}
	present := v.State == ProjectConfigValid || v.State == ProjectConfigInvalid
	if !present {
		if v.Identity != (FileIdentity{}) || v.ContentDigest != "" || v.ProjectID != "" {
			return invalid(string(v.State) + " project config")
		}
		return nil
	}
	if err := firstError(v.Identity.Validate(), v.ContentDigest.Validate()); err != nil {
		return err
	}
	if v.State == ProjectConfigValid {
		return v.ProjectID.Validate()
	}
	if v.ProjectID != "" {
		return invalid("invalid project config project ID")
	}
	return nil
}

// Incompatibility is one fail-closed finding that prevents safe planning.
type Incompatibility struct {
	Reason  Reason
	Subject string
}

func (v Incompatibility) Validate() error {
	if !v.Reason.Valid() {
		return invalid("incompatibility reason")
	}
	if v.Subject != "" {
		return validateOpaque("incompatibility subject", v.Subject)
	}
	return nil
}

// RepositoryObservation is everything an inspection observes about the
// project that a precondition snapshot freezes. Remotes, the toolchain, and
// inspection metadata are deliberately outside it.
type RepositoryObservation struct {
	RepositoryKind                         RepositoryKind
	GitDir                                 ObservedPath
	CommonDir                              ObservedPath
	ObjectFormat                           ObjectFormat
	RefFormat                              RefFormat
	SequencerState                         SequencerState
	Head                                   HeadObservation
	RefsObservationState                   ObservationState
	Index                                  IndexObservation
	ConfigLockState                        LockFileState
	WorktreeObservationState               ObservationState
	ConfigurationObservationState          ObservationState
	ProjectConfig                          ProjectConfigObservation
	GitFlowEvidence                        GitFlowEvidence
	AttributeSourcesObservationState       ObservationState
	IgnoreSourcesObservationState          ObservationState
	IgnoreEvaluationsObservationState      ObservationState
	ConversionSettings                     ConversionSettings
	LinkedWorktreesObservationState        ObservationState
	SubmodulesObservationState             ObservationState
	FilesystemCapabilitiesObservationState ObservationState
	refs                                   []RefObservation
	worktreeChanges                        []WorktreeChange
	worktreePathIdentities                 []WorktreePathIdentity
	effectiveConfiguration                 []ConfigEntry
	attributeSources                       []AttributeSource
	ignoreSources                          []IgnoreSource
	ignoreEvaluations                      []IgnoreEvaluation
	linkedWorktrees                        []LinkedWorktree
	submodules                             []Submodule
	filesystemCapabilities                 []FilesystemCapabilityProfile
}

type RepositoryObservationInput struct {
	RepositoryKind                         RepositoryKind
	GitDir                                 ObservedPath
	CommonDir                              ObservedPath
	ObjectFormat                           ObjectFormat
	RefFormat                              RefFormat
	SequencerState                         SequencerState
	Head                                   HeadObservation
	RefsObservationState                   ObservationState
	Refs                                   []RefObservation
	Index                                  IndexObservation
	ConfigLockState                        LockFileState
	WorktreeObservationState               ObservationState
	WorktreeChanges                        []WorktreeChange
	WorktreePathIdentities                 []WorktreePathIdentity
	ConfigurationObservationState          ObservationState
	EffectiveConfiguration                 []ConfigEntry
	ProjectConfig                          ProjectConfigObservation
	GitFlowEvidence                        GitFlowEvidence
	AttributeSourcesObservationState       ObservationState
	AttributeSources                       []AttributeSource
	IgnoreSourcesObservationState          ObservationState
	IgnoreSources                          []IgnoreSource
	IgnoreEvaluationsObservationState      ObservationState
	IgnoreEvaluations                      []IgnoreEvaluation
	ConversionSettings                     ConversionSettings
	LinkedWorktreesObservationState        ObservationState
	LinkedWorktrees                        []LinkedWorktree
	SubmodulesObservationState             ObservationState
	Submodules                             []Submodule
	FilesystemCapabilitiesObservationState ObservationState
	FilesystemCapabilities                 []FilesystemCapabilityProfile
}

func NewRepositoryObservation(in RepositoryObservationInput) (RepositoryObservation, error) {
	v := RepositoryObservation{
		RepositoryKind:                         in.RepositoryKind,
		GitDir:                                 in.GitDir,
		CommonDir:                              in.CommonDir,
		ObjectFormat:                           in.ObjectFormat,
		RefFormat:                              in.RefFormat,
		SequencerState:                         in.SequencerState,
		Head:                                   in.Head,
		RefsObservationState:                   in.RefsObservationState,
		Index:                                  in.Index,
		ConfigLockState:                        in.ConfigLockState,
		WorktreeObservationState:               in.WorktreeObservationState,
		ConfigurationObservationState:          in.ConfigurationObservationState,
		ProjectConfig:                          in.ProjectConfig,
		GitFlowEvidence:                        in.GitFlowEvidence,
		AttributeSourcesObservationState:       in.AttributeSourcesObservationState,
		IgnoreSourcesObservationState:          in.IgnoreSourcesObservationState,
		IgnoreEvaluationsObservationState:      in.IgnoreEvaluationsObservationState,
		ConversionSettings:                     in.ConversionSettings,
		LinkedWorktreesObservationState:        in.LinkedWorktreesObservationState,
		SubmodulesObservationState:             in.SubmodulesObservationState,
		FilesystemCapabilitiesObservationState: in.FilesystemCapabilitiesObservationState,
		refs:                                   cloneSlice(in.Refs),
		worktreeChanges:                        cloneSlice(in.WorktreeChanges),
		worktreePathIdentities:                 cloneSlice(in.WorktreePathIdentities),
		effectiveConfiguration:                 cloneSlice(in.EffectiveConfiguration),
		attributeSources:                       cloneSlice(in.AttributeSources),
		ignoreSources:                          cloneSlice(in.IgnoreSources),
		ignoreEvaluations:                      cloneSlice(in.IgnoreEvaluations),
		linkedWorktrees:                        cloneSlice(in.LinkedWorktrees),
		submodules:                             cloneSlice(in.Submodules),
		filesystemCapabilities:                 cloneSlice(in.FilesystemCapabilities),
	}
	return v, v.Validate()
}

func (v RepositoryObservation) Validate() error {
	if !v.RepositoryKind.Valid() || !v.SequencerState.Valid() || !v.ConfigLockState.Valid() {
		return invalid("repository observation")
	}
	if (v.ObjectFormat != "" && !v.ObjectFormat.Valid()) || (v.RefFormat != "" && !v.RefFormat.Valid()) {
		return invalid("repository formats")
	}
	if err := firstError(validateOptionalPath(v.GitDir), validateOptionalPath(v.CommonDir), v.Head.Validate()); err != nil {
		return err
	}
	located := !v.GitDir.IsZero() && !v.CommonDir.IsZero() && v.ObjectFormat != "" && v.RefFormat != ""
	unlocated := v.GitDir.IsZero() && v.CommonDir.IsZero() && v.ObjectFormat == "" && v.RefFormat == ""
	switch v.RepositoryKind {
	case RepositoryKindAbsent:
		if !unlocated || v.SequencerState != SequencerNone || v.Head != (HeadObservation{State: HeadStateUnborn}) ||
			v.RefsObservationState != ObservationNotApplicable || v.Index.Presence != IndexAbsent ||
			v.ConfigLockState != LockFileAbsent {
			return invalid("absent repository observation")
		}
	case RepositoryKindNormal:
		if !located {
			return invalid("normal repository observation")
		}
	}
	if err := firstError(
		validateObservedGroup("refs", v.RefsObservationState, len(v.refs)),
		validateObservedGroup("worktree changes", v.WorktreeObservationState, len(v.worktreeChanges)),
		validateObservedGroup("effective configuration", v.ConfigurationObservationState, len(v.effectiveConfiguration)),
		validateObservedGroup("attribute sources", v.AttributeSourcesObservationState, len(v.attributeSources)),
		validateObservedGroup("ignore sources", v.IgnoreSourcesObservationState, len(v.ignoreSources)),
		validateObservedGroup("ignore evaluations", v.IgnoreEvaluationsObservationState, len(v.ignoreEvaluations)),
		validateObservedGroup("linked worktrees", v.LinkedWorktreesObservationState, len(v.linkedWorktrees)),
		validateObservedGroup("submodules", v.SubmodulesObservationState, len(v.submodules)),
		validateObservedGroup("filesystem capabilities", v.FilesystemCapabilitiesObservationState, len(v.filesystemCapabilities)),
		v.Index.Validate(),
		v.ProjectConfig.Validate(),
		v.GitFlowEvidence.Validate(),
		v.ConversionSettings.Validate(),
		validateEach(v.refs),
		validateEach(v.worktreeChanges),
		validateEach(v.worktreePathIdentities),
		validateEach(v.effectiveConfiguration),
		validateEach(v.attributeSources),
		validateEach(v.ignoreSources),
		validateEach(v.ignoreEvaluations),
		validateEach(v.linkedWorktrees),
		validateEach(v.submodules),
		validateEach(v.filesystemCapabilities),
	); err != nil {
		return err
	}
	if v.ObjectFormat != "" {
		for _, ref := range v.refs {
			if ref.TargetOID.ValidateFor(v.ObjectFormat) != nil || (ref.PeeledOID != "" && ref.PeeledOID.ValidateFor(v.ObjectFormat) != nil) {
				return invalid("ref object format")
			}
		}
	}
	for i := 1; i < len(v.refs); i++ {
		if bytes.Compare(v.refs[i-1].Name.Bytes(), v.refs[i].Name.Bytes()) >= 0 {
			return invalid("ref order")
		}
	}
	for i := 1; i < len(v.worktreeChanges); i++ {
		a, b := v.worktreeChanges[i-1], v.worktreeChanges[i]
		if !pathKindLess(a.Path, string(a.Kind), b.Path, string(b.Kind)) {
			return invalid("worktree change order")
		}
	}
	identities := make(map[string]bool, len(v.worktreePathIdentities))
	for i, identity := range v.worktreePathIdentities {
		if i > 0 && bytes.Compare(v.worktreePathIdentities[i-1].Path.Bytes(), identity.Path.Bytes()) >= 0 {
			return invalid("worktree path identity order")
		}
		identities[identity.Path.ReversiblePathBytesBase64URL] = true
	}
	// Every changed path is frozen so its preservation can be verified.
	for _, change := range v.worktreeChanges {
		if !identities[change.Path.ReversiblePathBytesBase64URL] {
			return invalid("worktree change without path identity")
		}
	}
	for i, entry := range v.effectiveConfiguration {
		if entry.Ordinal != uint64(i) {
			return invalid("configuration order")
		}
	}
	// An evaluation may cite only sources this observation actually holds.
	sources := make(map[ContentDigest]bool, len(v.ignoreSources))
	for _, source := range v.ignoreSources {
		sources[source.Identity.ContentDigest] = true
	}
	for _, evaluation := range v.ignoreEvaluations {
		for _, digest := range evaluation.orderedSourceDigests {
			if !sources[digest] {
				return invalid("ignore evaluation source")
			}
		}
	}
	roles := make(map[FilesystemRole]bool, len(v.filesystemCapabilities))
	for _, profile := range v.filesystemCapabilities {
		if roles[profile.Role] {
			return invalid("duplicate filesystem profile role")
		}
		roles[profile.Role] = true
	}
	return nil
}

// unreliableGroups names every member that planning cannot rely on.
func (v RepositoryObservation) unreliableGroups() []string {
	var names []string
	for _, group := range []struct {
		name  string
		state ObservationState
	}{
		{"refs", v.RefsObservationState}, {"worktree", v.WorktreeObservationState},
		{"configuration", v.ConfigurationObservationState}, {"git_flow_evidence", v.GitFlowEvidence.ObservationState},
		{"attribute_sources", v.AttributeSourcesObservationState}, {"ignore_sources", v.IgnoreSourcesObservationState},
		{"ignore_evaluations", v.IgnoreEvaluationsObservationState}, {"conversion_settings", v.ConversionSettings.ObservationState},
		{"linked_worktrees", v.LinkedWorktreesObservationState}, {"submodules", v.SubmodulesObservationState},
		{"filesystem_capabilities", v.FilesystemCapabilitiesObservationState},
	} {
		if group.state.unreliable() {
			names = append(names, group.name)
		}
	}
	if v.Index.Presence == IndexUnreadable || v.Index.LockState == LockFileUnobservable {
		names = append(names, "index")
	}
	if v.ConfigLockState == LockFileUnobservable {
		names = append(names, "config_lock")
	}
	if v.ProjectConfig.State == ProjectConfigUnreadable {
		names = append(names, "project_config")
	}
	return names
}

func (v RepositoryObservation) Refs() []RefObservation { return cloneSlice(v.refs) }
func (v RepositoryObservation) WorktreeChanges() []WorktreeChange {
	return cloneSlice(v.worktreeChanges)
}
func (v RepositoryObservation) WorktreePathIdentities() []WorktreePathIdentity {
	return cloneSlice(v.worktreePathIdentities)
}
func (v RepositoryObservation) EffectiveConfiguration() []ConfigEntry {
	return cloneSlice(v.effectiveConfiguration)
}
func (v RepositoryObservation) AttributeSources() []AttributeSource {
	return cloneSlice(v.attributeSources)
}
func (v RepositoryObservation) IgnoreSources() []IgnoreSource { return cloneSlice(v.ignoreSources) }
func (v RepositoryObservation) IgnoreEvaluations() []IgnoreEvaluation {
	return cloneSlice(v.ignoreEvaluations)
}
func (v RepositoryObservation) LinkedWorktrees() []LinkedWorktree {
	return cloneSlice(v.linkedWorktrees)
}
func (v RepositoryObservation) Submodules() []Submodule { return cloneSlice(v.submodules) }
func (v RepositoryObservation) FilesystemCapabilities() []FilesystemCapabilityProfile {
	return cloneSlice(v.filesystemCapabilities)
}

// GitInspection is one complete read-only observation. It never grants
// mutation authority.
type GitInspection struct {
	SchemaVersion           string
	InspectionID            InspectionID
	InspectedAt             time.Time
	ProjectIdentity         PhysicalProjectIdentity
	Observation             RepositoryObservation
	RemotesObservationState ObservationState
	ToolIdentity            GitToolIdentity
	remotes                 []RemoteRedacted
	warnings                []string
	incompatibilities       []Incompatibility
}

type GitInspectionInput struct {
	SchemaVersion           string
	InspectionID            InspectionID
	InspectedAt             time.Time
	ProjectIdentity         PhysicalProjectIdentity
	Observation             RepositoryObservation
	RemotesObservationState ObservationState
	RemotesRedacted         []RemoteRedacted
	ToolIdentity            GitToolIdentity
	Warnings                []string
	Incompatibilities       []Incompatibility
}

func NewGitInspection(in GitInspectionInput) (GitInspection, error) {
	v := GitInspection{
		SchemaVersion:           in.SchemaVersion,
		InspectionID:            in.InspectionID,
		InspectedAt:             in.InspectedAt,
		ProjectIdentity:         in.ProjectIdentity,
		Observation:             in.Observation,
		RemotesObservationState: in.RemotesObservationState,
		ToolIdentity:            in.ToolIdentity,
		remotes:                 cloneSlice(in.RemotesRedacted),
		warnings:                cloneSlice(in.Warnings),
		incompatibilities:       cloneSlice(in.Incompatibilities),
	}
	return v, v.Validate()
}

func (v GitInspection) Validate() error {
	if v.SchemaVersion == "" || v.InspectedAt.IsZero() {
		return invalid("Git inspection")
	}
	if err := firstError(
		v.InspectionID.Validate(),
		v.ProjectIdentity.Validate(),
		v.ToolIdentity.Validate(),
		v.Observation.Validate(),
		validateObservedGroup("remotes", v.RemotesObservationState, len(v.remotes)),
		validateEach(v.remotes),
		validateLabels("inspection warning", v.warnings),
		validateEach(v.incompatibilities),
	); err != nil {
		return err
	}
	names := make(map[string]bool, len(v.remotes))
	for _, remote := range v.remotes {
		if names[remote.Name] {
			return invalid("duplicate remote")
		}
		names[remote.Name] = true
	}
	// Missing filesystem profiles are explicit incompatibilities, and any
	// unreliable group must be explained rather than read as empty success.
	if v.Observation.FilesystemCapabilitiesObservationState != ObservationObserved && len(v.incompatibilities) == 0 {
		return invalid("unexplained missing filesystem capabilities")
	}
	unreliable := len(v.Observation.unreliableGroups()) > 0 || v.RemotesObservationState.unreliable()
	if unreliable && len(v.warnings)+len(v.incompatibilities) == 0 {
		return invalid("unexplained unreliable observation")
	}
	return nil
}

func (v GitInspection) RemotesRedacted() []RemoteRedacted { return cloneSlice(v.remotes) }
func (v GitInspection) Warnings() []string                { return cloneSlice(v.warnings) }
func (v GitInspection) Incompatibilities() []Incompatibility {
	return cloneSlice(v.incompatibilities)
}

// ProjectConfigPath is the mandatory member of every initial-commit allowlist.
const ProjectConfigPath = ".ai-dev/config.json"

type AuthorizedFile struct {
	PathUTF8                     string
	ReversiblePathBytesBase64URL string
	Mode                         string
	ContentSHA256                ContentDigest
	BlobOID                      BlobOID
	Ignored                      bool
	ApprovalKind                 ApprovalKind
	IndividuallyApproved         bool
	ObservedIdentity             FileIdentity
}

func (v AuthorizedFile) Validate() error {
	if !validAuthorizedPath(v.PathUTF8) || (v.Mode != "100644" && v.Mode != "100755") || !v.ApprovalKind.Valid() || !v.IndividuallyApproved {
		return invalid("authorized file")
	}
	path, err := DecodeReversiblePath(v.ReversiblePathBytesBase64URL)
	if err != nil {
		return err
	}
	if !utf8.Valid(path) || string(path) != v.PathUTF8 {
		return invalid("authorized file reversible path")
	}
	if v.Ignored != (v.ApprovalKind == ApprovalKindIgnored) {
		return invalid("authorized file approval kind")
	}
	if !v.ObservedIdentity.Mode.IsRegular() {
		return invalid("authorized file observed type")
	}
	if err := firstError(v.ContentSHA256.Validate(), v.BlobOID.Validate(), v.ObservedIdentity.Validate(), CheckPathBytes(path)); err != nil {
		return err
	}
	return CheckLimit(LimitAuthorizedFileBytes, v.ObservedIdentity.Size)
}

func validAuthorizedPath(value string) bool {
	if value == "" || !utf8.ValidString(value) || strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") {
		return false
	}
	if len(value) >= 2 && value[1] == ':' {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || r == '|' || strings.ContainsRune("*?[", r) {
			return false
		}
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

// validateAuthorizedFiles checks the file count before anything else, then
// every file, repeated paths, and the aggregate authorized bytes.
func validateAuthorizedFiles(files []AuthorizedFile) error {
	if err := CheckLimit(LimitAuthorizedFiles, int64(len(files))); err != nil {
		return err
	}
	aggregate, err := NewBudget(LimitAggregateAuthorizedBytes)
	if err != nil {
		return err
	}
	seen := make(map[string]bool, len(files))
	for _, file := range files {
		if err := file.Validate(); err != nil {
			return err
		}
		if seen[file.PathUTF8] {
			return invalid("duplicate authorized file")
		}
		seen[file.PathUTF8] = true
		if err := aggregate.Add(file.ObservedIdentity.Size); err != nil {
			return err
		}
	}
	return nil
}

type GitIdentity struct{ Name, Email string }

func (v GitIdentity) Validate() error {
	if validateOpaque("identity name", v.Name) != nil || validateOpaque("identity email", v.Email) != nil {
		return invalid("Git identity")
	}
	return nil
}

// DevelopmentBase is the F008-FR-046 request to create an absent development
// branch in existing history. A valid base always equals the frozen
// production tip; any other value was refused during planning.
type DevelopmentBase struct {
	BaseOID          ObjectOID
	ObjectFormat     ObjectFormat
	ProductionRef    ObservedRef
	ProductionTipOID ObjectOID
}

func (v DevelopmentBase) IsZero() bool { return v == DevelopmentBase{} }

func (v DevelopmentBase) Validate() error {
	if err := firstError(
		v.BaseOID.ValidateFor(v.ObjectFormat),
		v.ProductionTipOID.ValidateFor(v.ObjectFormat),
		v.ProductionRef.Validate(),
	); err != nil {
		return err
	}
	if v.BaseOID != v.ProductionTipOID {
		return invalid("development base differs from production tip")
	}
	return nil
}

// ImmutableIntent is everything frozen at plan time. Nothing in it is
// inferred or reread from Git configuration during apply.
type ImmutableIntent struct {
	ProjectID                ProjectID
	Strategy                 Strategy
	GitFlowDialect           GitFlowDialect
	DevelopmentBase          DevelopmentBase
	InitializeRepository     bool
	ObjectFormat             ObjectFormat
	RefFormat                RefFormat
	ToolIdentity             GitToolIdentity
	ProjectConfigBytesDigest ContentDigest
	InitialCommit            bool
	Author                   GitIdentity
	Committer                GitIdentity
	AuthorTimestamp          time.Time
	CommitterTimestamp       time.Time
	branchParameters         map[string]string
	prefixParameters         map[string]string
	commitMessageBytes       []byte
	authorizedFiles          []AuthorizedFile
	requiredCapabilities     []string
}

type ImmutableIntentInput struct {
	ProjectID                ProjectID
	Strategy                 Strategy
	GitFlowDialect           GitFlowDialect
	BranchParameters         map[string]string
	DevelopmentBase          DevelopmentBase
	PrefixParameters         map[string]string
	InitializeRepository     bool
	ObjectFormat             ObjectFormat
	RefFormat                RefFormat
	ToolIdentity             GitToolIdentity
	ProjectConfigBytesDigest ContentDigest
	InitialCommit            bool
	Author                   GitIdentity
	Committer                GitIdentity
	AuthorTimestamp          time.Time
	CommitterTimestamp       time.Time
	CommitMessageBytes       []byte
	AuthorizedFiles          []AuthorizedFile
	RequiredCapabilities     []string
}

func NewImmutableIntent(in ImmutableIntentInput) (ImmutableIntent, error) {
	v := ImmutableIntent{
		ProjectID:                in.ProjectID,
		Strategy:                 in.Strategy,
		GitFlowDialect:           in.GitFlowDialect,
		DevelopmentBase:          in.DevelopmentBase,
		InitializeRepository:     in.InitializeRepository,
		ObjectFormat:             in.ObjectFormat,
		RefFormat:                in.RefFormat,
		ToolIdentity:             in.ToolIdentity,
		ProjectConfigBytesDigest: in.ProjectConfigBytesDigest,
		InitialCommit:            in.InitialCommit,
		Author:                   in.Author,
		Committer:                in.Committer,
		AuthorTimestamp:          in.AuthorTimestamp,
		CommitterTimestamp:       in.CommitterTimestamp,
		branchParameters:         cloneMap(in.BranchParameters),
		prefixParameters:         cloneMap(in.PrefixParameters),
		commitMessageBytes:       cloneSlice(in.CommitMessageBytes),
		authorizedFiles:          cloneSlice(in.AuthorizedFiles),
		requiredCapabilities:     cloneSlice(in.RequiredCapabilities),
	}
	return v, v.Validate()
}

func (v ImmutableIntent) Validate() error {
	if !v.Strategy.Valid() || !v.ObjectFormat.Valid() || !v.RefFormat.Valid() || len(v.requiredCapabilities) == 0 {
		return invalid("immutable intent")
	}
	if err := firstError(
		v.ProjectID.Validate(),
		v.ToolIdentity.Validate(),
		v.ProjectConfigBytesDigest.Validate(),
		validateLabels("required capability", v.requiredCapabilities),
		validateAuthorizedFiles(v.authorizedFiles),
	); err != nil {
		return err
	}
	if v.GitFlowDialect != "" && (v.Strategy != StrategyGitFlow || !v.GitFlowDialect.Valid()) {
		return invalid("Git Flow dialect")
	}
	// New repositories freeze sha1, files, and an explicit branch; a new Git
	// Flow configuration also needs its explicit dialect.
	if v.InitializeRepository {
		if v.ObjectFormat != ObjectFormatSHA1 || v.RefFormat != RefFormatFiles || len(v.branchParameters) == 0 ||
			(v.Strategy == StrategyGitFlow && v.GitFlowDialect == "") {
			return invalid("new repository intent")
		}
	}
	for name, value := range v.branchParameters {
		if validateOpaque("branch parameter name", name) != nil || validateOpaque("branch parameter", value) != nil {
			return invalid("branch parameters")
		}
	}
	for name, value := range v.prefixParameters {
		if validateOpaque("prefix parameter name", name) != nil || !utf8.ValidString(value) {
			return invalid("prefix parameters")
		}
	}
	if !v.DevelopmentBase.IsZero() {
		if v.Strategy != StrategyGitFlow || v.InitializeRepository || v.DevelopmentBase.ObjectFormat != v.ObjectFormat {
			return invalid("development base intent")
		}
		if err := v.DevelopmentBase.Validate(); err != nil {
			return err
		}
	}
	if !v.InitialCommit {
		if len(v.authorizedFiles) != 0 || len(v.commitMessageBytes) != 0 || v.Author != (GitIdentity{}) ||
			v.Committer != (GitIdentity{}) || !v.AuthorTimestamp.IsZero() || !v.CommitterTimestamp.IsZero() {
			return invalid("unrequested initial commit intent")
		}
		return nil
	}
	if err := CheckLimit(LimitCommitMessageBytes, int64(len(v.commitMessageBytes))); err != nil {
		return err
	}
	if v.Author.Validate() != nil || v.Committer.Validate() != nil || v.AuthorTimestamp.IsZero() ||
		v.CommitterTimestamp.IsZero() || len(v.commitMessageBytes) == 0 {
		return invalid("initial commit intent")
	}
	config := false
	for _, file := range v.authorizedFiles {
		if file.BlobOID.ValidateFor(v.ObjectFormat) != nil {
			return invalid("authorized file object format")
		}
		if file.PathUTF8 == ProjectConfigPath {
			config = file.Mode == "100644"
		}
	}
	if !config {
		return invalid("initial commit project config")
	}
	return nil
}

func (v ImmutableIntent) BranchParameters() map[string]string { return cloneMap(v.branchParameters) }
func (v ImmutableIntent) PrefixParameters() map[string]string { return cloneMap(v.prefixParameters) }
func (v ImmutableIntent) CommitMessageBytes() []byte          { return cloneSlice(v.commitMessageBytes) }
func (v ImmutableIntent) AuthorizedFiles() []AuthorizedFile   { return cloneSlice(v.authorizedFiles) }
func (v ImmutableIntent) RequiredCapabilities() []string      { return cloneSlice(v.requiredCapabilities) }

// Fixed lookup leaves under the state root that every Git subprocess uses as
// HOME and XDG_CONFIG_HOME. They must never exist.
const (
	EnvironmentHomeLeaf = "environment/nonexistent-home-v1"
	EnvironmentXDGLeaf  = "environment/nonexistent-xdg-v1"
)

// EnvironmentProfile freezes the exact bytes a Git subprocess environment
// uses, so plan, digest, operation IDs, and execution agree.
type EnvironmentProfile struct {
	Version       string
	StateRoot     ObservedPath
	Home          ObservedPath
	XDGConfigHome ObservedPath
}

func (v EnvironmentProfile) Validate() error {
	if err := firstError(
		validateOpaque("environment profile version", v.Version),
		v.StateRoot.Validate(), v.Home.Validate(), v.XDGConfigHome.Validate(),
	); err != nil {
		return err
	}
	root := v.StateRoot.Bytes()
	if root[0] != '/' || bytes.HasSuffix(root, []byte("/")) {
		return invalid("environment state root")
	}
	leaf := func(name string) []byte { return append(append(cloneSlice(root), '/'), name...) }
	if !bytes.Equal(v.Home.Bytes(), leaf(EnvironmentHomeLeaf)) || !bytes.Equal(v.XDGConfigHome.Bytes(), leaf(EnvironmentXDGLeaf)) {
		return invalid("environment profile leaves")
	}
	return nil
}

// PreconditionSnapshot is the complete, digest-covered state a plan was
// derived from. Its observation must be reliable throughout.
type PreconditionSnapshot struct {
	ProjectIdentity    PhysicalProjectIdentity
	Observation        RepositoryObservation
	EnvironmentProfile EnvironmentProfile
	DevelopmentBase    DevelopmentBase
	ToolIdentity       GitToolIdentity
}

func (v PreconditionSnapshot) Validate() error {
	if err := firstError(
		v.ProjectIdentity.Validate(),
		v.Observation.Validate(),
		v.EnvironmentProfile.Validate(),
		v.ToolIdentity.Validate(),
	); err != nil {
		return err
	}
	if groups := v.Observation.unreliableGroups(); len(groups) > 0 {
		return invalid("unreliable snapshot observation: " + strings.Join(groups, ", "))
	}
	if v.Observation.FilesystemCapabilitiesObservationState != ObservationObserved {
		return invalid("snapshot filesystem capabilities")
	}
	if !v.DevelopmentBase.IsZero() {
		if err := v.DevelopmentBase.Validate(); err != nil {
			return err
		}
		if v.DevelopmentBase.ObjectFormat != v.Observation.ObjectFormat {
			return invalid("snapshot development base object format")
		}
	}
	return nil
}

type ConditionKind string

const (
	ConditionRepositoryAbsent ConditionKind = "repository_absent"
	ConditionRepositoryKind   ConditionKind = "repository_kind"
	ConditionHeadState        ConditionKind = "head_state"
	ConditionHeadSymbolicRef  ConditionKind = "head_symbolic_ref"
	ConditionRefAbsent        ConditionKind = "ref_absent"
	ConditionRefTarget        ConditionKind = "ref_target"
	ConditionReflogAbsent     ConditionKind = "reflog_absent"
	ConditionObjectType       ConditionKind = "object_type"
	ConditionPathAbsent       ConditionKind = "path_absent"
	ConditionPathIdentity     ConditionKind = "path_identity"
	ConditionPathContent      ConditionKind = "path_content"
	ConditionPathMode         ConditionKind = "path_mode"
	ConditionIndexAbsent      ConditionKind = "index_absent"
	ConditionIndexEmpty       ConditionKind = "index_empty"
	ConditionIndexTree        ConditionKind = "index_tree"
	ConditionLockFileAbsent   ConditionKind = "index_lock_absent"
	ConditionConfigAbsent     ConditionKind = "config_absent"
	ConditionConfigValue      ConditionKind = "config_value"
	ConditionPathStatusClean  ConditionKind = "path_status_clean"
)

type conditionSubject uint8

const (
	subjectNone conditionSubject = iota
	subjectPath
	subjectRef
	subjectOID
	subjectConfigKey
)

type conditionExpected uint8

const (
	expectedNone conditionExpected = iota
	expectedRepositoryKind
	expectedHeadState
	expectedOID
	expectedObjectType
	expectedDigest
	expectedIdentity
	expectedMode
	expectedConfigValue
)

// conditionShapes is the closed condition catalogue of SCHEMA-103B-09.
var conditionShapes = map[ConditionKind]struct {
	subject  conditionSubject
	expected conditionExpected
}{
	ConditionRepositoryAbsent: {subjectNone, expectedNone},
	ConditionRepositoryKind:   {subjectNone, expectedRepositoryKind},
	ConditionHeadState:        {subjectNone, expectedHeadState},
	ConditionHeadSymbolicRef:  {subjectRef, expectedNone},
	ConditionRefAbsent:        {subjectRef, expectedNone},
	ConditionRefTarget:        {subjectRef, expectedOID},
	ConditionReflogAbsent:     {subjectRef, expectedNone},
	ConditionObjectType:       {subjectOID, expectedObjectType},
	ConditionPathAbsent:       {subjectPath, expectedNone},
	ConditionPathIdentity:     {subjectPath, expectedIdentity},
	ConditionPathContent:      {subjectPath, expectedDigest},
	ConditionPathMode:         {subjectPath, expectedMode},
	ConditionIndexAbsent:      {subjectNone, expectedNone},
	ConditionIndexEmpty:       {subjectNone, expectedNone},
	ConditionIndexTree:        {subjectNone, expectedOID},
	ConditionLockFileAbsent:   {subjectNone, expectedNone},
	ConditionConfigAbsent:     {subjectConfigKey, expectedNone},
	ConditionConfigValue:      {subjectConfigKey, expectedConfigValue},
	ConditionPathStatusClean:  {subjectPath, expectedNone},
}

func (v ConditionKind) Valid() bool { _, ok := conditionShapes[v]; return ok }

// Condition is one typed precondition, expected effect, or postcondition.
// Paths and refs in Subject are unpadded base64url of their exact bytes.
type Condition struct {
	Kind     ConditionKind
	Subject  string
	Expected string
}

func (v Condition) Validate() error {
	shape, ok := conditionShapes[v.Kind]
	if !ok {
		return invalid("condition kind")
	}
	if err := validateConditionSubject(shape.subject, v.Subject); err != nil {
		return err
	}
	return validateConditionExpected(shape.expected, v.Expected)
}

func validateConditionSubject(kind conditionSubject, subject string) error {
	switch kind {
	case subjectPath:
		raw, err := DecodeReversiblePath(subject)
		if err != nil {
			return err
		}
		return NewObservedPath(raw).Validate()
	case subjectRef:
		raw, err := DecodeReversiblePath(subject)
		if err != nil {
			return err
		}
		return NewObservedRef(raw).Validate()
	case subjectOID:
		return ObjectOID(subject).Validate()
	case subjectConfigKey:
		return validateConfigKey(subject)
	}
	if subject != "" {
		return invalid("condition subject")
	}
	return nil
}

func validateConditionExpected(kind conditionExpected, expected string) error {
	switch kind {
	case expectedRepositoryKind:
		if !RepositoryKind(expected).Valid() {
			return invalid("condition repository kind")
		}
	case expectedHeadState:
		if !HeadState(expected).Valid() {
			return invalid("condition HEAD state")
		}
	case expectedOID:
		return ObjectOID(expected).Validate()
	case expectedObjectType:
		if expected != "blob" && expected != "tree" && expected != "commit" {
			return invalid("condition object type")
		}
	case expectedDigest:
		return ContentDigest(expected).Validate()
	case expectedIdentity:
		device, inode, found := strings.Cut(expected, ":")
		if !found {
			return invalid("condition identity")
		}
		return firstError(ValidateDecimalIdentity(device), ValidateDecimalIdentity(inode))
	case expectedMode:
		if len(expected) == 0 || len(expected) > 6 || strings.Trim(expected, "01234567") != "" {
			return invalid("condition mode")
		}
	case expectedConfigValue:
		if expected == "" || !utf8.ValidString(expected) || strings.ContainsRune(expected, 0) {
			return invalid("condition config value")
		}
	default:
		if expected != "" {
			return invalid("condition expected value")
		}
	}
	return nil
}

type PartialEffectClassification string

const (
	PartialEffectNone            PartialEffectClassification = "none"
	PartialEffectAtomic          PartialEffectClassification = "atomic"
	PartialEffectPartialPossible PartialEffectClassification = "partial_possible"
)

func (v PartialEffectClassification) Valid() bool {
	return v == PartialEffectNone || v == PartialEffectAtomic || v == PartialEffectPartialPossible
}

type RecoveryInstruction string

const (
	RecoveryNotApplicable            RecoveryInstruction = "not_applicable"
	RecoverySupervisedReconciliation RecoveryInstruction = "supervised_reconciliation"
)

func (v RecoveryInstruction) Valid() bool {
	return v == RecoveryNotApplicable || v == RecoverySupervisedReconciliation
}

// DerivedOperation is one planner-regenerated operation. Runtime apply IDs,
// owner tokens, PIDs, audit timestamps, and temporary paths never appear in
// it, and an operation read from an input file is never dispatched directly.
type DerivedOperation struct {
	OperationID                 OperationID
	Ordinal                     uint64
	Kind                        OperationKind
	EnvironmentProfile          string
	StdinDigest                 ContentDigest
	TargetIdentity              string
	PartialEffectClassification PartialEffectClassification
	RecoveryInstruction         RecoveryInstruction
	exactArgv                   []string
	preconditions               []Condition
	affectedPaths               []ObservedPath
	affectedRefs                []ObservedRef
	affectedConfigKeys          []string
	expectedEffects             []Condition
	expectedPostconditions      []Condition
	failureOutcomes             []Reason
	temporaryResourceRoles      []ResourceRole
}

type DerivedOperationInput struct {
	OperationID                 OperationID
	Ordinal                     uint64
	Kind                        OperationKind
	ExactArgv                   []string
	EnvironmentProfile          string
	StdinDigest                 ContentDigest
	TargetIdentity              string
	Preconditions               []Condition
	AffectedPaths               []ObservedPath
	AffectedRefs                []ObservedRef
	AffectedConfigKeys          []string
	ExpectedEffects             []Condition
	ExpectedPostconditions      []Condition
	FailureOutcomes             []Reason
	PartialEffectClassification PartialEffectClassification
	RecoveryInstruction         RecoveryInstruction
	TemporaryResourceRoles      []ResourceRole
}

func NewDerivedOperation(in DerivedOperationInput) (DerivedOperation, error) {
	v := DerivedOperation{
		OperationID:                 in.OperationID,
		Ordinal:                     in.Ordinal,
		Kind:                        in.Kind,
		EnvironmentProfile:          in.EnvironmentProfile,
		StdinDigest:                 in.StdinDigest,
		TargetIdentity:              in.TargetIdentity,
		PartialEffectClassification: in.PartialEffectClassification,
		RecoveryInstruction:         in.RecoveryInstruction,
		exactArgv:                   cloneSlice(in.ExactArgv),
		preconditions:               cloneSlice(in.Preconditions),
		affectedPaths:               cloneSlice(in.AffectedPaths),
		affectedRefs:                cloneSlice(in.AffectedRefs),
		affectedConfigKeys:          cloneSlice(in.AffectedConfigKeys),
		expectedEffects:             cloneSlice(in.ExpectedEffects),
		expectedPostconditions:      cloneSlice(in.ExpectedPostconditions),
		failureOutcomes:             cloneSlice(in.FailureOutcomes),
		temporaryResourceRoles:      cloneSlice(in.TemporaryResourceRoles),
	}
	return v, v.Validate()
}

func (v DerivedOperation) Validate() error {
	if err := v.OperationID.Validate(); err != nil {
		return err
	}
	if !v.Kind.Valid() || !v.PartialEffectClassification.Valid() || !v.RecoveryInstruction.Valid() || len(v.preconditions) == 0 {
		return invalid("derived operation")
	}
	if !strings.HasPrefix(string(v.OperationID), strconv.FormatUint(v.Ordinal, 10)+":") {
		return invalid("operation ordinal")
	}
	// Only verification is effect-free; everything else declares what it
	// changes and how it can fail.
	verification := v.Kind == OperationKindVerification
	if verification != (len(v.expectedEffects) == 0) || verification != (v.PartialEffectClassification == PartialEffectNone) ||
		(!verification && len(v.failureOutcomes) == 0) {
		return invalid("operation effects")
	}
	if (v.PartialEffectClassification == PartialEffectNone) != (v.RecoveryInstruction == RecoveryNotApplicable) {
		return invalid("operation recovery instruction")
	}
	// A Git invocation is exact argv under a named environment profile;
	// neither appears without the other. Only a Git invocation reads stdin,
	// and its exact bytes are bound by digest (O-T011-1).
	if (len(v.exactArgv) > 0) != (v.EnvironmentProfile != "") {
		return invalid("operation argv and environment profile")
	}
	if v.StdinDigest != "" {
		if len(v.exactArgv) == 0 {
			return invalid("operation stdin without Git invocation")
		}
		if err := v.StdinDigest.Validate(); err != nil {
			return err
		}
	}
	for _, argument := range v.exactArgv {
		if strings.ContainsRune(argument, 0) {
			return invalid("operation argv")
		}
	}
	for _, role := range v.temporaryResourceRoles {
		if !role.Valid() {
			return invalid("operation temporary resource role")
		}
	}
	for _, reason := range v.failureOutcomes {
		if !reason.Valid() {
			return invalid("operation failure outcome")
		}
	}
	for _, key := range v.affectedConfigKeys {
		if err := validateConfigKey(key); err != nil {
			return err
		}
	}
	return firstError(
		validateOpaque("operation target identity", v.TargetIdentity),
		validateEach(v.preconditions),
		validateEach(v.expectedEffects),
		validateEach(v.expectedPostconditions),
		validateEach(v.affectedPaths),
		validateEach(v.affectedRefs),
	)
}

func (v DerivedOperation) ExactArgv() []string           { return cloneSlice(v.exactArgv) }
func (v DerivedOperation) Preconditions() []Condition    { return cloneSlice(v.preconditions) }
func (v DerivedOperation) AffectedPaths() []ObservedPath { return cloneSlice(v.affectedPaths) }
func (v DerivedOperation) AffectedRefs() []ObservedRef   { return cloneSlice(v.affectedRefs) }
func (v DerivedOperation) AffectedConfigKeys() []string  { return cloneSlice(v.affectedConfigKeys) }
func (v DerivedOperation) ExpectedEffects() []Condition  { return cloneSlice(v.expectedEffects) }
func (v DerivedOperation) ExpectedPostconditions() []Condition {
	return cloneSlice(v.expectedPostconditions)
}
func (v DerivedOperation) FailureOutcomes() []Reason { return cloneSlice(v.failureOutcomes) }
func (v DerivedOperation) TemporaryResourceRoles() []ResourceRole {
	return cloneSlice(v.temporaryResourceRoles)
}

type ProhibitedOperation string

// prohibitedOperationCatalogue is the closed, ordered minimum every plan
// lists completely (SCHEMA-103B-11).
var prohibitedOperationCatalogue = []ProhibitedOperation{
	"push", "fetch", "pull", "merge", "rebase", "tag_mutation", "remote_mutation", "branch_delete",
	"branch_rename", "reset", "checkout", "switch", "stash", "gc", "amend", "automatic_restore",
	"submodule_mutation", "linked_worktree_mutation", "git_flow_hooks", "git_flow_feature_finish",
}

func (v ProhibitedOperation) Valid() bool { return containsValue(prohibitedOperationCatalogue, v) }

// ProhibitedOperations returns the complete catalogue in its fixed order.
func ProhibitedOperations() []ProhibitedOperation { return cloneSlice(prohibitedOperationCatalogue) }

type RecoveryPolicy string

// RecoveryPolicySupervisedReconciliation is the only recovery policy: no
// automatic rollback, retry, repair, or cleanup ever runs.
const RecoveryPolicySupervisedReconciliation RecoveryPolicy = "supervised_reconciliation"

// GitInitializationPlanSchema is the only accepted plan schema version.
const GitInitializationPlanSchema = "ai-dev.git-init-plan/v1"

// GitInitializationPlan separates excluded metadata (IDs and timestamps)
// from the covered members that determine PlanDigest.
type GitInitializationPlan struct {
	SchemaVersion          string
	PlanID                 PlanID
	GeneratedAt            time.Time
	ExpiresAt              time.Time // zero when the plan does not expire
	InspectionID           InspectionID
	InspectedAt            time.Time
	ImmutableIntent        ImmutableIntent
	PreconditionSnapshot   PreconditionSnapshot
	RecoveryPolicy         RecoveryPolicy
	PlanDigest             PlanDigest
	derivedOperations      []DerivedOperation
	expectedPostconditions []Condition
	prohibitedOperations   []ProhibitedOperation
	partialEffectPoints    []OperationID
}

type GitInitializationPlanInput struct {
	SchemaVersion          string
	PlanID                 PlanID
	GeneratedAt            time.Time
	ExpiresAt              time.Time
	InspectionID           InspectionID
	InspectedAt            time.Time
	ImmutableIntent        ImmutableIntent
	PreconditionSnapshot   PreconditionSnapshot
	DerivedOperations      []DerivedOperation
	ExpectedPostconditions []Condition
	ProhibitedOperations   []ProhibitedOperation
	PartialEffectPoints    []OperationID
	RecoveryPolicy         RecoveryPolicy
	PlanDigest             PlanDigest
}

func NewGitInitializationPlan(in GitInitializationPlanInput) (GitInitializationPlan, error) {
	v := GitInitializationPlan{
		SchemaVersion:          in.SchemaVersion,
		PlanID:                 in.PlanID,
		GeneratedAt:            in.GeneratedAt,
		ExpiresAt:              in.ExpiresAt,
		InspectionID:           in.InspectionID,
		InspectedAt:            in.InspectedAt,
		ImmutableIntent:        in.ImmutableIntent,
		PreconditionSnapshot:   in.PreconditionSnapshot,
		RecoveryPolicy:         in.RecoveryPolicy,
		PlanDigest:             in.PlanDigest,
		derivedOperations:      cloneSlice(in.DerivedOperations),
		expectedPostconditions: cloneSlice(in.ExpectedPostconditions),
		prohibitedOperations:   cloneSlice(in.ProhibitedOperations),
		partialEffectPoints:    cloneSlice(in.PartialEffectPoints),
	}
	return v, v.Validate()
}

func (v GitInitializationPlan) Validate() error {
	if err := CheckLimit(LimitDerivedOperations, int64(len(v.derivedOperations))); err != nil {
		return err
	}
	postconditions := int64(len(v.expectedPostconditions))
	for _, operation := range v.derivedOperations {
		postconditions += int64(len(operation.expectedPostconditions))
	}
	if err := CheckLimit(LimitPostconditions, postconditions); err != nil {
		return err
	}
	if v.SchemaVersion != GitInitializationPlanSchema || len(v.derivedOperations) == 0 || len(v.expectedPostconditions) == 0 ||
		v.GeneratedAt.IsZero() || v.InspectedAt.IsZero() || v.RecoveryPolicy != RecoveryPolicySupervisedReconciliation {
		return invalid("initialization plan")
	}
	if !v.ExpiresAt.IsZero() && !v.ExpiresAt.After(v.GeneratedAt) {
		return invalid("initialization plan expiration")
	}
	if err := firstError(
		v.PlanID.Validate(),
		v.InspectionID.Validate(),
		v.ImmutableIntent.Validate(),
		v.PreconditionSnapshot.Validate(),
		v.PlanDigest.Validate(),
		validateEach(v.expectedPostconditions),
	); err != nil {
		return err
	}
	if v.ImmutableIntent.DevelopmentBase != v.PreconditionSnapshot.DevelopmentBase {
		return invalid("plan development base")
	}
	if len(v.prohibitedOperations) != len(prohibitedOperationCatalogue) {
		return invalid("plan prohibited operations")
	}
	for i, operation := range v.prohibitedOperations {
		if operation != prohibitedOperationCatalogue[i] {
			return invalid("plan prohibited operations")
		}
	}
	// Partial-effect points are exactly the operations that can leave an
	// effect behind, in plan order.
	var points []OperationID
	for i, operation := range v.derivedOperations {
		if operation.Ordinal != uint64(i) {
			return invalid("operation order")
		}
		if err := operation.Validate(); err != nil {
			return err
		}
		if operation.PartialEffectClassification != PartialEffectNone {
			points = append(points, operation.OperationID)
		}
	}
	if len(points) != len(v.partialEffectPoints) {
		return invalid("plan partial-effect points")
	}
	for i, point := range points {
		if v.partialEffectPoints[i] != point {
			return invalid("plan partial-effect points")
		}
	}
	// Identifiers and the digest are derived values: a plan whose stored
	// values differ from their recomputation was not produced by the planner.
	for _, operation := range v.derivedOperations {
		id, err := deriveOperationID(operation.Ordinal, operation.Kind, operation.exactArgv, operation.EnvironmentProfile,
			operation.StdinDigest, operation.TargetIdentity, operation.preconditions, operation.expectedEffects,
			operation.PartialEffectClassification, v.PreconditionSnapshot.EnvironmentProfile)
		if err != nil {
			return err
		}
		if id != operation.OperationID {
			return invalid("operation ID derivation")
		}
	}
	covered, err := CanonicalCoveredPlan(v)
	if err != nil {
		return err
	}
	if PlanDigest(fmt.Sprintf("sha256:%x", sha256.Sum256(covered))) != v.PlanDigest {
		return invalid("plan digest")
	}
	return nil
}

func (v GitInitializationPlan) DerivedOperations() []DerivedOperation {
	return cloneSlice(v.derivedOperations)
}
func (v GitInitializationPlan) ExpectedPostconditions() []Condition {
	return cloneSlice(v.expectedPostconditions)
}
func (v GitInitializationPlan) ProhibitedOperations() []ProhibitedOperation {
	return cloneSlice(v.prohibitedOperations)
}
func (v GitInitializationPlan) PartialEffectPoints() []OperationID {
	return cloneSlice(v.partialEffectPoints)
}

// GitAuthorization is local supervised mutation authority for at most one
// mutation sequence. DeclaredOperator is a label, not an authentication.
type GitAuthorization struct {
	SchemaVersion           string
	AuthorizationID         AuthorizationID
	PlanDigest              PlanDigest
	ProjectIdentity         PhysicalProjectIdentity
	Strategy                Strategy
	GitFlowDialect          GitFlowDialect
	InitialCommitAuthorized bool
	DeclaredOperator        string
	IssuedAt                time.Time
	ExpiresAt               time.Time // zero when the authorization does not expire
	State                   AuthorizationState
	StateVersion            uint64
	OwnerApplyID            ApplyID
	TerminalResultDigest    ContentDigest
	PreviousAuthorizationID AuthorizationID
	PreviousApplyID         ApplyID
	PreviousPlanDigest      PlanDigest
	authorizedOperationIDs  []OperationID
	authorizedRefs          []ObservedRef
	authorizedConfigKeys    []string
	approvedFiles           []AuthorizedFile
	transitionHistory       []AuthorizationTransition
}

// AuthorizationTransition is one immutable history entry. OwnerApplyID is
// the apply attempt that performed the transition.
type AuthorizationTransition struct {
	From         AuthorizationState
	To           AuthorizationState
	StateVersion uint64
	OwnerApplyID ApplyID
}

type GitAuthorizationInput struct {
	SchemaVersion           string
	AuthorizationID         AuthorizationID
	PlanDigest              PlanDigest
	ProjectIdentity         PhysicalProjectIdentity
	AuthorizedOperationIDs  []OperationID
	Strategy                Strategy
	GitFlowDialect          GitFlowDialect
	AuthorizedRefs          []ObservedRef
	AuthorizedConfigKeys    []string
	InitialCommitAuthorized bool
	ApprovedFiles           []AuthorizedFile
	DeclaredOperator        string
	IssuedAt                time.Time
	ExpiresAt               time.Time
	State                   AuthorizationState
	StateVersion            uint64
	OwnerApplyID            ApplyID
	TerminalResultDigest    ContentDigest
	PreviousAuthorizationID AuthorizationID
	PreviousApplyID         ApplyID
	PreviousPlanDigest      PlanDigest
	TransitionHistory       []AuthorizationTransition
}

func NewGitAuthorization(in GitAuthorizationInput) (GitAuthorization, error) {
	v := GitAuthorization{
		SchemaVersion:           in.SchemaVersion,
		AuthorizationID:         in.AuthorizationID,
		PlanDigest:              in.PlanDigest,
		ProjectIdentity:         in.ProjectIdentity,
		Strategy:                in.Strategy,
		GitFlowDialect:          in.GitFlowDialect,
		InitialCommitAuthorized: in.InitialCommitAuthorized,
		DeclaredOperator:        in.DeclaredOperator,
		IssuedAt:                in.IssuedAt,
		ExpiresAt:               in.ExpiresAt,
		State:                   in.State,
		StateVersion:            in.StateVersion,
		OwnerApplyID:            in.OwnerApplyID,
		TerminalResultDigest:    in.TerminalResultDigest,
		PreviousAuthorizationID: in.PreviousAuthorizationID,
		PreviousApplyID:         in.PreviousApplyID,
		PreviousPlanDigest:      in.PreviousPlanDigest,
		authorizedOperationIDs:  cloneSlice(in.AuthorizedOperationIDs),
		authorizedRefs:          cloneSlice(in.AuthorizedRefs),
		authorizedConfigKeys:    cloneSlice(in.AuthorizedConfigKeys),
		approvedFiles:           cloneSlice(in.ApprovedFiles),
		transitionHistory:       cloneSlice(in.TransitionHistory),
	}
	return v, v.Validate()
}

func (v GitAuthorization) Validate() error {
	if v.SchemaVersion == "" || !v.State.Valid() || v.StateVersion == 0 || !v.Strategy.Valid() ||
		len(v.authorizedOperationIDs) == 0 || v.IssuedAt.IsZero() {
		return invalid("authorization")
	}
	if v.GitFlowDialect != "" && (v.Strategy != StrategyGitFlow || !v.GitFlowDialect.Valid()) {
		return invalid("authorization Git Flow dialect")
	}
	if !v.ExpiresAt.IsZero() && !v.ExpiresAt.After(v.IssuedAt) {
		return invalid("authorization expiration")
	}
	if v.InitialCommitAuthorized != (len(v.approvedFiles) > 0) {
		return invalid("authorization initial commit approvals")
	}
	for _, key := range v.authorizedConfigKeys {
		if err := validateConfigKey(key); err != nil {
			return err
		}
	}
	if err := firstError(
		v.AuthorizationID.Validate(),
		v.PlanDigest.Validate(),
		v.ProjectIdentity.Validate(),
		validateOpaque("declared operator", v.DeclaredOperator),
		validateEach(v.authorizedOperationIDs),
		validateEach(v.authorizedRefs),
		validateAuthorizedFiles(v.approvedFiles),
	); err != nil {
		return err
	}
	// A reconciliation authorization links to all three prior identifiers.
	linked := v.PreviousAuthorizationID != "" || v.PreviousApplyID != "" || v.PreviousPlanDigest != ""
	if linked {
		if err := firstError(v.PreviousAuthorizationID.Validate(), v.PreviousApplyID.Validate(), v.PreviousPlanDigest.Validate()); err != nil {
			return err
		}
		if v.PreviousAuthorizationID == v.AuthorizationID {
			return invalid("self-linked authorization")
		}
	}
	if v.State.terminal() {
		if err := v.TerminalResultDigest.Validate(); err != nil {
			return invalid("terminal authorization")
		}
	} else if v.TerminalResultDigest != "" {
		return invalid("premature terminal digest")
	}
	return validateAuthorizationHistory(v.State, v.StateVersion, v.OwnerApplyID, v.transitionHistory)
}

// Transition returns the authorization that results from applying req. It
// reports ErrAuthorizationConflict when the compared state, version, or
// owner differs, and never mutates v.
func (v GitAuthorization) Transition(req AuthorizationTransitionRequest) (GitAuthorization, error) {
	if err := req.Validate(); err != nil {
		return GitAuthorization{}, err
	}
	if req.AuthorizationID != v.AuthorizationID || req.ProjectIdentityDigest != v.ProjectIdentity.Digest() ||
		req.ExpectedState != v.State || req.ExpectedVersion != v.StateVersion || req.ExpectedOwner != v.OwnerApplyID {
		return GitAuthorization{}, ErrAuthorizationConflict
	}
	next := v
	next.State = req.To
	next.StateVersion = v.StateVersion + 1
	next.OwnerApplyID = req.Owner
	if req.To == AuthorizationStateIssued {
		next.OwnerApplyID = "" // a clean pre-mutation release leaves no owner
	}
	next.TerminalResultDigest = req.TerminalResultDigest
	next.transitionHistory = append(cloneSlice(v.transitionHistory), AuthorizationTransition{
		From: v.State, To: req.To, StateVersion: next.StateVersion, OwnerApplyID: req.Owner,
	})
	return next, next.Validate()
}

func (v GitAuthorization) AuthorizedOperationIDs() []OperationID {
	return cloneSlice(v.authorizedOperationIDs)
}
func (v GitAuthorization) AuthorizedRefs() []ObservedRef   { return cloneSlice(v.authorizedRefs) }
func (v GitAuthorization) AuthorizedConfigKeys() []string  { return cloneSlice(v.authorizedConfigKeys) }
func (v GitAuthorization) ApprovedFiles() []AuthorizedFile { return cloneSlice(v.approvedFiles) }
func (v GitAuthorization) TransitionHistory() []AuthorizationTransition {
	return cloneSlice(v.transitionHistory)
}

// validateAuthorizationHistory replays the history from the initial issued
// state. The owner of a transition is the attempt that performed it, so it
// may only change on the claim and on a proven orphan reassignment.
func validateAuthorizationHistory(state AuthorizationState, version uint64, owner ApplyID, history []AuthorizationTransition) error {
	current, currentVersion, currentOwner := AuthorizationStateIssued, uint64(1), ApplyID("")
	for _, transition := range history {
		if transition.From != current || transition.StateVersion != currentVersion+1 ||
			transition.OwnerApplyID.Validate() != nil || !authorizationTransitionAllowed(transition.From, transition.To) {
			return invalid("authorization transition history")
		}
		reassignment := transition.From == AuthorizationStateClaimed && transition.To == AuthorizationStateClaimed
		if current != AuthorizationStateIssued && reassignment == (transition.OwnerApplyID == currentOwner) {
			return invalid("authorization transition owner")
		}
		current, currentVersion, currentOwner = transition.To, transition.StateVersion, transition.OwnerApplyID
		if current == AuthorizationStateIssued {
			currentOwner = ""
		}
	}
	if current != state || currentVersion != version {
		return invalid("authorization state history")
	}
	if owner != currentOwner {
		return invalid("authorization owner")
	}
	return nil
}

func authorizationTransitionAllowed(from, to AuthorizationState) bool {
	switch from {
	case AuthorizationStateIssued:
		return to == AuthorizationStateClaimed
	case AuthorizationStateClaimed:
		return to == AuthorizationStateIssued || to == AuthorizationStateClaimed || to == AuthorizationStateMutationStarted
	case AuthorizationStateMutationStarted:
		return to.terminal()
	default:
		return false
	}
}

type GitApplyAttempt struct {
	SchemaVersion      string
	ApplyID            ApplyID
	AuthorizationID    AuthorizationID
	DeclaredPlanDigest PlanDigest
	ProjectIdentity    PhysicalProjectIdentity
	StartedAt          time.Time
	InvocationMode     InvocationMode
	ResultStatus       ApplyStatus
	ResultReason       Reason
}

func (v GitApplyAttempt) Validate() error {
	if v.SchemaVersion == "" || v.StartedAt.IsZero() || !v.InvocationMode.Valid() {
		return invalid("apply attempt")
	}
	if err := v.ApplyID.Validate(); err != nil {
		return err
	}
	if v.AuthorizationID != "" {
		if err := v.AuthorizationID.Validate(); err != nil {
			return err
		}
	}
	if v.DeclaredPlanDigest != "" {
		if err := v.DeclaredPlanDigest.Validate(); err != nil {
			return err
		}
	}
	if !v.ProjectIdentity.IsZero() {
		if err := v.ProjectIdentity.Validate(); err != nil {
			return err
		}
	}
	if v.ResultStatus != "" && !v.ResultStatus.Valid() {
		return invalid("attempt result status")
	}
	if v.ResultReason != "" && (!v.ResultReason.Valid() || v.ResultStatus == "") {
		return invalid("attempt result reason")
	}
	return nil
}

const SecondaryFailurePhaseResourceRelease = "resource_release"

// SecondaryFailure records one failed owned-resource release. It never
// contains a temporary path, token, descriptor, secret, or file content.
type SecondaryFailure struct {
	Code              Reason
	Phase             string
	ResourceRole      ResourceRole
	OperationID       OperationID
	FailureClass      string
	OwnershipVerified bool
	AuditRecordDigest ContentDigest
	ReleaseOrdinal    uint64
}

func (v SecondaryFailure) Validate() error {
	if v.Code != ReasonResourceReleaseFailed || v.Phase != SecondaryFailurePhaseResourceRelease || !v.ResourceRole.Valid() ||
		validateOpaque("secondary failure class", v.FailureClass) != nil {
		return invalid("secondary failure")
	}
	if v.OperationID != "" {
		if err := v.OperationID.Validate(); err != nil {
			return err
		}
	}
	if v.AuditRecordDigest != "" {
		if err := v.AuditRecordDigest.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func secondaryFailureLess(a, b SecondaryFailure) bool {
	if a.ReleaseOrdinal != b.ReleaseOrdinal {
		return a.ReleaseOrdinal < b.ReleaseOrdinal
	}
	return a.ResourceRole < b.ResourceRole
}

// SortSecondaryFailures orders by planned release ordinal, then resource role.
func SortSecondaryFailures(values []SecondaryFailure) {
	sort.SliceStable(values, func(i, j int) bool { return secondaryFailureLess(values[i], values[j]) })
}

// SecurityFinding identifies a high-risk category without the matched bytes.
type SecurityFinding struct {
	Path     ObservedPath
	Category string
}

func (v SecurityFinding) Validate() error {
	return firstError(v.Path.Validate(), validateOpaque("security finding category", v.Category))
}

// GitApplyResult is the result of one apply invocation. Status is derived
// from verified effects; Reason is the single primary cause and never
// substitutes for it. DurableAuthorizationState is empty only for a refusal
// that never resolved an authorization.
type GitApplyResult struct {
	SchemaVersion             string
	ApplyID                   ApplyID
	AuthorizationID           AuthorizationID
	PlanID                    PlanID
	PlanDigest                PlanDigest
	Status                    ApplyStatus
	Reason                    Reason
	DurableAuthorizationState AuthorizationState
	ExitCode                  int
	PriorAuthorizationState   AuthorizationState
	BeforeSnapshotDigest      ContentDigest
	AfterSnapshotDigest       ContentDigest
	FailedOperationID         OperationID
	CommitOID                 ObjectOID
	IndexTreeOID              ObjectOID
	DevelopmentBranchCreated  bool
	AuditStatus               string
	RecoveryRequired          bool
	secondaryFailures         []SecondaryFailure
	completedOperationIDs     []OperationID
	notStartedOperationIDs    []OperationID
	indeterminateOperationIDs []OperationID
	verifiedEffects           []Condition
	warnings                  []string
	findings                  []SecurityFinding
}

type GitApplyResultInput struct {
	SchemaVersion             string
	ApplyID                   ApplyID
	AuthorizationID           AuthorizationID
	PlanID                    PlanID
	PlanDigest                PlanDigest
	Status                    ApplyStatus
	Reason                    Reason
	DurableAuthorizationState AuthorizationState
	ExitCode                  int
	SecondaryFailures         []SecondaryFailure
	PriorAuthorizationState   AuthorizationState
	BeforeSnapshotDigest      ContentDigest
	AfterSnapshotDigest       ContentDigest
	CompletedOperationIDs     []OperationID
	FailedOperationID         OperationID
	NotStartedOperationIDs    []OperationID
	IndeterminateOperationIDs []OperationID
	VerifiedEffects           []Condition
	CommitOID                 ObjectOID
	IndexTreeOID              ObjectOID
	DevelopmentBranchCreated  bool
	AuditStatus               string
	Warnings                  []string
	Findings                  []SecurityFinding
	RecoveryRequired          bool
}

func NewGitApplyResult(in GitApplyResultInput) (GitApplyResult, error) {
	v := GitApplyResult{
		SchemaVersion:             in.SchemaVersion,
		ApplyID:                   in.ApplyID,
		AuthorizationID:           in.AuthorizationID,
		PlanID:                    in.PlanID,
		PlanDigest:                in.PlanDigest,
		Status:                    in.Status,
		Reason:                    in.Reason,
		DurableAuthorizationState: in.DurableAuthorizationState,
		ExitCode:                  in.ExitCode,
		PriorAuthorizationState:   in.PriorAuthorizationState,
		BeforeSnapshotDigest:      in.BeforeSnapshotDigest,
		AfterSnapshotDigest:       in.AfterSnapshotDigest,
		FailedOperationID:         in.FailedOperationID,
		CommitOID:                 in.CommitOID,
		IndexTreeOID:              in.IndexTreeOID,
		DevelopmentBranchCreated:  in.DevelopmentBranchCreated,
		AuditStatus:               in.AuditStatus,
		RecoveryRequired:          in.RecoveryRequired,
		secondaryFailures:         cloneSlice(in.SecondaryFailures),
		completedOperationIDs:     cloneSlice(in.CompletedOperationIDs),
		notStartedOperationIDs:    cloneSlice(in.NotStartedOperationIDs),
		indeterminateOperationIDs: cloneSlice(in.IndeterminateOperationIDs),
		verifiedEffects:           cloneSlice(in.VerifiedEffects),
		warnings:                  cloneSlice(in.Warnings),
		findings:                  cloneSlice(in.Findings),
	}
	return v, v.Validate()
}

func (v GitApplyResult) Validate() error {
	if v.SchemaVersion == "" || !v.Status.Valid() || (v.Reason != "" && !v.Reason.Valid()) {
		return invalid("apply result")
	}
	if err := firstError(v.ApplyID.Validate(), validateOpaque("apply result audit status", v.AuditStatus)); err != nil {
		return err
	}
	// Only a refusal may lack an authorization; every other status reports
	// on an authorization that was bound to a plan digest.
	if v.AuthorizationID == "" {
		if v.Status != ApplyStatusRefused || v.DurableAuthorizationState != "" || v.PriorAuthorizationState != "" {
			return invalid("apply result without authorization")
		}
	} else {
		if err := v.AuthorizationID.Validate(); err != nil {
			return err
		}
		if !v.DurableAuthorizationState.Valid() || !resultStateCompatible(v.Status, v.DurableAuthorizationState) {
			return invalid("result durable state")
		}
		if v.Status != ApplyStatusRefused && v.PlanDigest == "" {
			return invalid("apply result plan digest")
		}
	}
	if v.PriorAuthorizationState != "" && !v.PriorAuthorizationState.Valid() {
		return invalid("apply result prior authorization state")
	}
	for _, check := range []struct {
		present  bool
		validate func() error
	}{
		{v.PlanID != "", v.PlanID.Validate},
		{v.PlanDigest != "", v.PlanDigest.Validate},
		{v.BeforeSnapshotDigest != "", v.BeforeSnapshotDigest.Validate},
		{v.AfterSnapshotDigest != "", v.AfterSnapshotDigest.Validate},
		{v.FailedOperationID != "", v.FailedOperationID.Validate},
		{v.CommitOID != "", v.CommitOID.Validate},
		{v.IndexTreeOID != "", v.IndexTreeOID.Validate},
	} {
		if check.present {
			if err := check.validate(); err != nil {
				return err
			}
		}
	}
	if err := firstError(
		validateEach(v.secondaryFailures),
		validateEach(v.findings),
		validateEach(v.verifiedEffects),
		validateLabels("apply result warning", v.warnings),
	); err != nil {
		return err
	}
	for i := 1; i < len(v.secondaryFailures); i++ {
		if secondaryFailureLess(v.secondaryFailures[i], v.secondaryFailures[i-1]) {
			return invalid("secondary failure order")
		}
	}
	// An operation has exactly one outcome within a result.
	seen := make(map[OperationID]bool)
	for _, ids := range [][]OperationID{v.completedOperationIDs, v.notStartedOperationIDs, v.indeterminateOperationIDs} {
		for _, id := range ids {
			if id.Validate() != nil || seen[id] {
				return invalid("apply result operation ID")
			}
			seen[id] = true
		}
	}
	if seen[v.FailedOperationID] {
		return invalid("failed operation ID outcome")
	}
	if !resultReasonCompatible(v.Status, v.Reason, len(v.secondaryFailures) > 0) {
		return invalid("result reason/status")
	}
	return validateResultExit(v.Status, v.Reason, v.ExitCode, len(v.secondaryFailures) > 0)
}

func (v GitApplyResult) SecondaryFailures() []SecondaryFailure {
	return cloneSlice(v.secondaryFailures)
}
func (v GitApplyResult) CompletedOperationIDs() []OperationID {
	return cloneSlice(v.completedOperationIDs)
}
func (v GitApplyResult) NotStartedOperationIDs() []OperationID {
	return cloneSlice(v.notStartedOperationIDs)
}
func (v GitApplyResult) IndeterminateOperationIDs() []OperationID {
	return cloneSlice(v.indeterminateOperationIDs)
}
func (v GitApplyResult) VerifiedEffects() []Condition { return cloneSlice(v.verifiedEffects) }
func (v GitApplyResult) Warnings() []string           { return cloneSlice(v.warnings) }
func (v GitApplyResult) Findings() []SecurityFinding  { return cloneSlice(v.findings) }

func resultReasonCompatible(status ApplyStatus, reason Reason, secondary bool) bool {
	if reason == "" {
		return status == ApplyStatusApplied || status == ApplyStatusAlreadyApplied
	}
	if reason == ReasonAuditFailedAfterMutation {
		return status == ApplyStatusIndeterminate
	}
	if reason == ReasonDevelopmentBranchPendingInitialHistory {
		return status == ApplyStatusApplied
	}
	if reason == ReasonOutputFailed {
		return status != ApplyStatusRefused
	}
	if reason == ReasonResourceReleaseFailed {
		return secondary && status != ApplyStatusRefused
	}
	if reason == ReasonOperationFailed {
		return status == ApplyStatusFailed || status == ApplyStatusPartiallyApplied || status == ApplyStatusIndeterminate
	}
	// An interrupt (INT-01) is a refusal before the mutation intent and,
	// after it, a cause whose effects are classified like a concurrent
	// change.
	if reason == ReasonConcurrentChange || reason == ReasonInterrupted {
		return status == ApplyStatusRefused || status == ApplyStatusFailed || status == ApplyStatusPartiallyApplied || status == ApplyStatusIndeterminate
	}
	// All remaining normative reasons describe refusal before mutation.
	return status == ApplyStatusRefused
}

func resultStateCompatible(status ApplyStatus, state AuthorizationState) bool {
	switch status {
	case ApplyStatusApplied, ApplyStatusAlreadyApplied:
		return state == AuthorizationStateApplied
	case ApplyStatusRefused:
		return state == AuthorizationStateIssued || state == AuthorizationStateClaimed || state == AuthorizationStateMutationStarted || state.terminal()
	case ApplyStatusFailed:
		return state == AuthorizationStateFailed
	case ApplyStatusPartiallyApplied:
		return state == AuthorizationStatePartiallyApplied
	case ApplyStatusIndeterminate:
		return state == AuthorizationStateIndeterminate || state == AuthorizationStateMutationStarted || state == AuthorizationStateClaimed
	default:
		return false
	}
}

func validateResultExit(status ApplyStatus, reason Reason, exit int, secondary bool) error {
	if secondary {
		if exit == 1 {
			return nil
		}
		return invalid("secondary exit")
	}
	if reason == ReasonOutputFailed {
		if exit == 2 {
			return nil
		}
		return invalid("output failure exit")
	}
	if status == ApplyStatusApplied || status == ApplyStatusAlreadyApplied {
		if reason != "" && reason != ReasonDevelopmentBranchPendingInitialHistory {
			return invalid("success reason")
		}
		if exit != 0 {
			return invalid("success exit")
		}
		return nil
	}
	if reason == "" {
		return invalid("missing failure reason")
	}
	usage := reason == ReasonInteractiveTerminalRequired || reason == ReasonInvalidStrategy || reason == ReasonMissingRequiredBranch || reason == ReasonOutputExists || reason == ReasonUnsafeOutputPath
	want := 1
	if usage {
		want = 2
	}
	if exit != want {
		return invalid("failure exit")
	}
	return nil
}

// pathKindLess orders entries by raw path bytes, then kind.
func pathKindLess(aPath ObservedPath, aKind string, bPath ObservedPath, bKind string) bool {
	if c := bytes.Compare(aPath.Bytes(), bPath.Bytes()); c != 0 {
		return c < 0
	}
	return aKind < bKind
}

func validateOpaque(name, value string) error {
	if value == "" || strings.TrimSpace(value) != value || !utf8.ValidString(value) {
		return invalid(name)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return invalid(name)
		}
	}
	return nil
}

// validateLabels applies validateOpaque to every element of a list.
func validateLabels(name string, values []string) error {
	for _, value := range values {
		if err := validateOpaque(name, value); err != nil {
			return err
		}
	}
	return nil
}

func validateEach[T interface{ Validate() error }](values []T) error {
	for _, value := range values {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func containsValue[T comparable](values []T, want T) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func validateSHA256(name, value string) error {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") || !lowerHex(value[len("sha256:"):]) {
		return invalid(name)
	}
	return nil
}

func validateOID(name, value string) error {
	if (len(value) != 40 && len(value) != 64) || !lowerHex(value) {
		return invalid(name)
	}
	return nil
}

func validateOIDFor(name, value string, format ObjectFormat) error {
	want := 40
	if format == ObjectFormatSHA256 {
		want = 64
	}
	if !format.Valid() || len(value) != want {
		return invalid(name + " object format")
	}
	return validateOID(name, value)
}

func validateUUIDv4(name, value string) error {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' || value[14] != '4' || (value[19] != '8' && value[19] != '9' && value[19] != 'a' && value[19] != 'b') {
		return invalid(name)
	}
	for i, c := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return invalid(name)
		}
	}
	return nil
}

func lowerHex(value string) bool {
	if value == "" {
		return false
	}
	for _, c := range value {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func invalid(field string) error { return fmt.Errorf("%w: %s", errInvalidModel, field) }

func cloneSlice[T any](value []T) []T { return append([]T(nil), value...) }

func cloneMap(value map[string]string) map[string]string {
	if value == nil {
		return nil
	}
	result := make(map[string]string, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}
