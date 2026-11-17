package gitmanager

import (
	"bytes"
	"fmt"
	"io/fs"
	"strconv"
	"time"
)

// Schemas of the durable records the Git store persists (STO-08).
const (
	GitAuthorizationSchema = "ai-dev.git-authorization/v1"
	GitApplyAttemptSchema  = "ai-dev.git-apply-attempt/v1"
	GitApplyResultSchema   = "ai-dev.git-apply-result/v1"
	AuditEventSchema       = "ai-dev.git-audit-event/v1"
)

// RecordDecodeLimits bound every durable record read back from the store;
// they are the plan bounds, which every record fits within.
var RecordDecodeLimits = PlanDecodeLimits

// EncodeAuthorizationRecord renders a valid authorization as the canonical
// JSON its durable record holds.
func EncodeAuthorizationRecord(v GitAuthorization) ([]byte, error) {
	if err := v.Validate(); err != nil {
		return nil, err
	}
	if v.SchemaVersion != GitAuthorizationSchema {
		return nil, invalid("authorization schema version")
	}
	e := &canonicalEncoder{}
	object := map[string]any{
		"schema_version":               v.SchemaVersion,
		"authorization_id":             string(v.AuthorizationID),
		"plan_digest":                  string(v.PlanDigest),
		"project_identity":             e.projectIdentity(v.ProjectIdentity),
		"authorized_operation_ids":     canonicalStrings(v.authorizedOperationIDs),
		"strategy":                     string(v.Strategy),
		"authorized_refs":              canonicalList(v.authorizedRefs, e.ref),
		"authorized_config_keys":       canonicalStrings(v.authorizedConfigKeys),
		"initial_commit_authorized":    v.InitialCommitAuthorized,
		"approved_files":               canonicalList(v.approvedFiles, e.authorizedFile),
		"declared_operator":            v.DeclaredOperator,
		"issued_at":                    e.timestamp(v.IssuedAt),
		"state":                        string(v.State),
		"state_version":                canonicalInteger(v.StateVersion),
		"immutable_transition_history": canonicalList(v.transitionHistory, transitionRecord),
	}
	optional(object, "git_flow_dialect", string(v.GitFlowDialect), v.GitFlowDialect != "")
	if !v.ExpiresAt.IsZero() {
		object["expires_at"] = e.timestamp(v.ExpiresAt)
	}
	optional(object, "owner_apply_id", string(v.OwnerApplyID), v.OwnerApplyID != "")
	optional(object, "terminal_result_digest", string(v.TerminalResultDigest), v.TerminalResultDigest != "")
	optional(object, "previous_authorization_id", string(v.PreviousAuthorizationID), v.PreviousAuthorizationID != "")
	optional(object, "previous_apply_id", string(v.PreviousApplyID), v.PreviousApplyID != "")
	optional(object, "previous_plan_digest", string(v.PreviousPlanDigest), v.PreviousPlanDigest != "")
	if e.err != nil {
		return nil, e.err
	}
	return MarshalCanonical(object)
}

func transitionRecord(v AuthorizationTransition) any {
	return map[string]any{
		"from":           string(v.From),
		"to":             string(v.To),
		"state_version":  canonicalInteger(v.StateVersion),
		"owner_apply_id": string(v.OwnerApplyID),
	}
}

// DecodeAuthorizationRecord strictly decodes a durable authorization record.
// The result must re-encode to exactly the same bytes, so a record the
// encoder could not have produced is rejected rather than reinterpreted.
func DecodeAuthorizationRecord(data []byte) (GitAuthorization, error) {
	value, err := UnmarshalCanonical(data, RecordDecodeLimits)
	if err != nil {
		return GitAuthorization{}, err
	}
	object, err := canonicalFields(value, []string{
		"schema_version", "authorization_id", "plan_digest", "project_identity", "authorized_operation_ids", "strategy",
		"authorized_refs", "authorized_config_keys", "initial_commit_authorized", "approved_files", "declared_operator",
		"issued_at", "state", "state_version", "immutable_transition_history",
	}, []string{
		"git_flow_dialect", "expires_at", "owner_apply_id", "terminal_result_digest", "previous_authorization_id",
		"previous_apply_id", "previous_plan_digest",
	})
	if err != nil {
		return GitAuthorization{}, err
	}
	d := &recordDecoder{}
	in := GitAuthorizationInput{
		SchemaVersion:           d.text(object, "schema_version"),
		AuthorizationID:         AuthorizationID(d.text(object, "authorization_id")),
		PlanDigest:              PlanDigest(d.text(object, "plan_digest")),
		ProjectIdentity:         d.projectIdentity(object["project_identity"]),
		AuthorizedOperationIDs:  decodeStrings[OperationID](d, object["authorized_operation_ids"]),
		Strategy:                Strategy(d.text(object, "strategy")),
		GitFlowDialect:          GitFlowDialect(d.optionalText(object, "git_flow_dialect")),
		AuthorizedRefs:          decodeList(d, object["authorized_refs"], d.ref),
		AuthorizedConfigKeys:    decodeStrings[string](d, object["authorized_config_keys"]),
		InitialCommitAuthorized: d.boolean(object, "initial_commit_authorized"),
		ApprovedFiles:           decodeList(d, object["approved_files"], d.authorizedFile),
		DeclaredOperator:        d.text(object, "declared_operator"),
		IssuedAt:                d.timestamp(object["issued_at"]),
		State:                   AuthorizationState(d.text(object, "state")),
		StateVersion:            d.integer(object["state_version"]),
		OwnerApplyID:            ApplyID(d.optionalText(object, "owner_apply_id")),
		TerminalResultDigest:    ContentDigest(d.optionalText(object, "terminal_result_digest")),
		PreviousAuthorizationID: AuthorizationID(d.optionalText(object, "previous_authorization_id")),
		PreviousApplyID:         ApplyID(d.optionalText(object, "previous_apply_id")),
		PreviousPlanDigest:      PlanDigest(d.optionalText(object, "previous_plan_digest")),
		TransitionHistory:       decodeList(d, object["immutable_transition_history"], d.transition),
	}
	if raw, present := object["expires_at"]; present {
		in.ExpiresAt = d.timestamp(raw)
	}
	if d.err != nil {
		return GitAuthorization{}, d.err
	}
	authorization, err := NewGitAuthorization(in)
	if err != nil {
		return GitAuthorization{}, err
	}
	again, err := EncodeAuthorizationRecord(authorization)
	if err != nil {
		return GitAuthorization{}, err
	}
	if !bytes.Equal(again, data) {
		return GitAuthorization{}, fmt.Errorf("%w: authorization record does not round-trip", errCanonicalSyntax)
	}
	return authorization, nil
}

// EncodeAttemptRecord renders a valid apply attempt as the canonical JSON of
// its durable attempt record.
func EncodeAttemptRecord(v GitApplyAttempt) ([]byte, error) {
	if err := v.Validate(); err != nil {
		return nil, err
	}
	if v.SchemaVersion != GitApplyAttemptSchema {
		return nil, invalid("apply attempt schema version")
	}
	e := &canonicalEncoder{}
	object := map[string]any{
		"schema_version":  v.SchemaVersion,
		"apply_id":        string(v.ApplyID),
		"started_at":      e.timestamp(v.StartedAt),
		"invocation_mode": string(v.InvocationMode),
	}
	optional(object, "authorization_id", string(v.AuthorizationID), v.AuthorizationID != "")
	optional(object, "declared_plan_digest", string(v.DeclaredPlanDigest), v.DeclaredPlanDigest != "")
	if !v.ProjectIdentity.IsZero() {
		object["project_identity"] = e.projectIdentity(v.ProjectIdentity)
	}
	optional(object, "result_status", string(v.ResultStatus), v.ResultStatus != "")
	optional(object, "result_reason", string(v.ResultReason), v.ResultReason != "")
	if e.err != nil {
		return nil, e.err
	}
	return MarshalCanonical(object)
}

// encodeApplyResult renders a valid result as canonical JSON. It is the
// byte form of the attempt result and, embedded unchanged, of the prepared
// terminal result.
func encodeApplyResult(v GitApplyResult) ([]byte, error) {
	value, err := applyResultValue(v)
	if err != nil {
		return nil, err
	}
	return MarshalCanonical(value)
}

// applyResultValue maps a valid result onto its canonical JSON value.
func applyResultValue(v GitApplyResult) (map[string]any, error) {
	if err := v.Validate(); err != nil {
		return nil, err
	}
	e := &canonicalEncoder{}
	object := map[string]any{
		"schema_version":              v.SchemaVersion,
		"apply_id":                    string(v.ApplyID),
		"status":                      string(v.Status),
		"exit_code":                   canonicalInteger(uint64(v.ExitCode)),
		"secondary_failures":          canonicalList(v.secondaryFailures, secondaryFailureRecord),
		"completed_operation_ids":     canonicalStrings(v.completedOperationIDs),
		"not_started_operation_ids":   canonicalStrings(v.notStartedOperationIDs),
		"indeterminate_operation_ids": canonicalStrings(v.indeterminateOperationIDs),
		"verified_effects":            canonicalList(v.verifiedEffects, e.condition),
		"audit_status":                v.AuditStatus,
		"warnings":                    canonicalStrings(v.warnings),
		"findings":                    canonicalList(v.findings, func(f SecurityFinding) any { return e.securityFinding(f) }),
		"recovery_required":           v.RecoveryRequired,
	}
	optional(object, "authorization_id", string(v.AuthorizationID), v.AuthorizationID != "")
	optional(object, "plan_id", string(v.PlanID), v.PlanID != "")
	optional(object, "plan_digest", string(v.PlanDigest), v.PlanDigest != "")
	optional(object, "reason", string(v.Reason), v.Reason != "")
	optional(object, "durable_authorization_state", string(v.DurableAuthorizationState), v.DurableAuthorizationState != "")
	optional(object, "prior_authorization_state", string(v.PriorAuthorizationState), v.PriorAuthorizationState != "")
	optional(object, "before_snapshot_digest", string(v.BeforeSnapshotDigest), v.BeforeSnapshotDigest != "")
	optional(object, "after_snapshot_digest", string(v.AfterSnapshotDigest), v.AfterSnapshotDigest != "")
	optional(object, "failed_operation_id", string(v.FailedOperationID), v.FailedOperationID != "")
	optional(object, "commit_oid", string(v.CommitOID), v.CommitOID != "")
	optional(object, "index_tree_oid", string(v.IndexTreeOID), v.IndexTreeOID != "")
	optional(object, "development_branch_created", v.DevelopmentBranchCreated, v.DevelopmentBranchCreated)
	if e.err != nil {
		return nil, e.err
	}
	return object, nil
}

func secondaryFailureRecord(v SecondaryFailure) any {
	object := map[string]any{
		"code":               string(v.Code),
		"phase":              v.Phase,
		"resource_role":      string(v.ResourceRole),
		"failure_class":      v.FailureClass,
		"ownership_verified": v.OwnershipVerified,
		"release_ordinal":    canonicalInteger(v.ReleaseOrdinal),
	}
	optional(object, "operation_id", string(v.OperationID), v.OperationID != "")
	optional(object, "audit_record_digest", string(v.AuditRecordDigest), v.AuditRecordDigest != "")
	return object
}

func (e *canonicalEncoder) securityFinding(v SecurityFinding) any {
	object := e.path(v.Path).(map[string]any)
	object["category"] = v.Category
	return object
}

// recordDecoder maps decoded canonical values back onto model inputs. The
// first type error is kept; callers check err once and then rely on the
// model constructors and the round-trip check for every semantic rule.
type recordDecoder struct{ err error }

func (d *recordDecoder) fail(what string) {
	if d.err == nil {
		d.err = fmt.Errorf("%w: record member %s", errCanonicalSyntax, what)
	}
}

func (d *recordDecoder) object(v any, what string, required, optional []string) map[string]any {
	object, err := canonicalFields(v, required, optional)
	if err != nil {
		d.fail(what)
		return map[string]any{}
	}
	return object
}

func (d *recordDecoder) text(object map[string]any, name string) string {
	value, ok := object[name].(string)
	if !ok {
		d.fail(name)
	}
	return value
}

func (d *recordDecoder) optionalText(object map[string]any, name string) string {
	if _, present := object[name]; !present {
		return ""
	}
	return d.text(object, name)
}

func (d *recordDecoder) boolean(object map[string]any, name string) bool {
	value, ok := object[name].(bool)
	if !ok {
		d.fail(name)
	}
	return value
}

// integer accepts the F008-FR-171 forms: a safe JSON number or, above the
// safe range, a decimal string.
func (d *recordDecoder) integer(v any) uint64 {
	switch value := v.(type) {
	case int64:
		if value >= 0 {
			return uint64(value)
		}
	case string:
		if parsed, err := strconv.ParseUint(value, 10, 64); err == nil && parsed > maxSafeInteger {
			return parsed
		}
	}
	d.fail("integer")
	return 0
}

func (d *recordDecoder) timestamp(v any) time.Time {
	text, ok := v.(string)
	parsed, err := time.Parse(time.RFC3339, text)
	if !ok || err != nil {
		d.fail("timestamp")
	}
	return parsed
}

func decodeList[T any](d *recordDecoder, v any, decode func(any) T) []T {
	items, ok := v.([]any)
	if !ok {
		d.fail("list")
		return nil
	}
	values := make([]T, 0, len(items))
	for _, item := range items {
		values = append(values, decode(item))
	}
	return values
}

func decodeStrings[T ~string](d *recordDecoder, v any) []T {
	return decodeList(d, v, func(item any) T {
		text, ok := item.(string)
		if !ok {
			d.fail("string list item")
		}
		return T(text)
	})
}

func (d *recordDecoder) projectIdentity(v any) PhysicalProjectIdentity {
	object := d.object(v, "project_identity", []string{"reversible_path_bytes_base64url", "device_decimal", "inode_decimal", "digest"},
		[]string{"physical_path_utf8"})
	path, err := DecodeReversiblePath(d.text(object, "reversible_path_bytes_base64url"))
	if err != nil {
		d.fail("project path")
		return PhysicalProjectIdentity{}
	}
	identity, err := NewPhysicalProjectIdentity(d.optionalText(object, "physical_path_utf8"), path,
		d.text(object, "device_decimal"), d.text(object, "inode_decimal"))
	if err != nil || identity.Digest() != ProjectIdentityDigest(d.text(object, "digest")) {
		d.fail("project identity")
	}
	return identity
}

func (d *recordDecoder) ref(v any) ObservedRef {
	object := d.object(v, "ref", []string{"reversible_path_bytes_base64url"}, []string{"display_path_utf8"})
	return ObservedRef{ReversibleBytesBase64URL: d.text(object, "reversible_path_bytes_base64url"), DisplayUTF8: d.optionalText(object, "display_path_utf8")}
}

func (d *recordDecoder) fileIdentity(v any) FileIdentity {
	object := d.object(v, "file identity", []string{"device_decimal", "inode_decimal", "file_type", "permissions_octal", "size"}, nil)
	permissions, err := strconv.ParseUint(d.text(object, "permissions_octal"), 8, 32)
	if err != nil {
		d.fail("permissions_octal")
	}
	mode := fs.FileMode(permissions) & fs.ModePerm
	switch d.text(object, "file_type") {
	case "regular":
	case "directory":
		mode |= fs.ModeDir
	case "symlink":
		mode |= fs.ModeSymlink
	default:
		d.fail("file_type")
	}
	return FileIdentity{
		DeviceDecimal: d.text(object, "device_decimal"), InodeDecimal: d.text(object, "inode_decimal"),
		Mode: mode, Size: int64(d.integer(object["size"])),
	}
}

func (d *recordDecoder) authorizedFile(v any) AuthorizedFile {
	object := d.object(v, "authorized file", []string{
		"path_utf8", "reversible_path_bytes_base64url", "mode", "content_sha256", "blob_oid", "ignored", "approval_kind",
		"individually_approved", "observed_identity",
	}, nil)
	return AuthorizedFile{
		PathUTF8:                     d.text(object, "path_utf8"),
		ReversiblePathBytesBase64URL: d.text(object, "reversible_path_bytes_base64url"),
		Mode:                         d.text(object, "mode"),
		ContentSHA256:                ContentDigest(d.text(object, "content_sha256")),
		BlobOID:                      BlobOID(d.text(object, "blob_oid")),
		Ignored:                      d.boolean(object, "ignored"),
		ApprovalKind:                 ApprovalKind(d.text(object, "approval_kind")),
		IndividuallyApproved:         d.boolean(object, "individually_approved"),
		ObservedIdentity:             d.fileIdentity(object["observed_identity"]),
	}
}

func (d *recordDecoder) transition(v any) AuthorizationTransition {
	object := d.object(v, "transition", []string{"from", "to", "state_version", "owner_apply_id"}, nil)
	return AuthorizationTransition{
		From: AuthorizationState(d.text(object, "from")), To: AuthorizationState(d.text(object, "to")),
		StateVersion: d.integer(object["state_version"]), OwnerApplyID: ApplyID(d.text(object, "owner_apply_id")),
	}
}
