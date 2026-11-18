package gitmanager

import (
	"bytes"
	"errors"
	"fmt"
)

// planCoveredMembers are exactly the F008-FR-172 covered members.
var planCoveredMembers = []string{
	"schema_version", "immutable_intent", "precondition_snapshot", "derived_operations", "expected_postconditions",
	"prohibited_operations", "partial_effect_points", "recovery_policy",
}

// EncodePlanFile renders the complete external plan file: the covered
// members, the excluded metadata and the plan digest.
func EncodePlanFile(plan GitInitializationPlan) ([]byte, error) {
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	covered, err := coveredPlan(plan.SchemaVersion, plan.ImmutableIntent, plan.PreconditionSnapshot, plan.derivedOperations,
		plan.expectedPostconditions, plan.prohibitedOperations, plan.partialEffectPoints, plan.RecoveryPolicy)
	if err != nil {
		return nil, err
	}
	object := covered.(map[string]any)
	e := &canonicalEncoder{}
	object["plan_id"] = string(plan.PlanID)
	object["generated_at"] = e.timestamp(plan.GeneratedAt)
	if !plan.ExpiresAt.IsZero() {
		object["expires_at"] = e.timestamp(plan.ExpiresAt)
	}
	object["inspection_id"] = string(plan.InspectionID)
	object["inspected_at"] = e.timestamp(plan.InspectedAt)
	object["plan_digest"] = string(plan.PlanDigest)
	if e.err != nil {
		return nil, e.err
	}
	return MarshalCanonical(object)
}

// PlanFile is what apply may take from an untrusted plan file: only the
// immutable intent and identifiers, never its snapshot or operations
// (F008-FR-050–054).
type PlanFile struct {
	PlanID     PlanID
	PlanDigest PlanDigest
	Intent     ImmutableIntent
}

var errPlanFile = errors.New("invalid plan file")

// DecodePlanFile validates an untrusted plan file and extracts its intent.
// The file must be exactly canonical within the F008-FR-051 limits, have
// the closed member set, carry a digest equal to the digest of its own
// covered members, and hold an intent that re-encodes to the same bytes.
// Its operations are never decoded for dispatch.
func DecodePlanFile(data []byte) (PlanFile, error) {
	value, err := UnmarshalCanonical(data, PlanDecodeLimits)
	if err != nil {
		return PlanFile{}, err
	}
	object, err := canonicalFields(value, append(append([]string(nil), planCoveredMembers...),
		"plan_id", "generated_at", "inspection_id", "inspected_at", "plan_digest"), []string{"expires_at"})
	if err != nil {
		return PlanFile{}, fmt.Errorf("%w: %v", errPlanFile, err)
	}
	if object["schema_version"] != GitInitializationPlanSchema {
		return PlanFile{}, fmt.Errorf("%w: schema", errPlanFile)
	}
	covered := make(map[string]any, len(planCoveredMembers))
	for _, name := range planCoveredMembers {
		covered[name] = object[name]
	}
	digest, err := canonicalSHA256(covered)
	if err != nil || object["plan_digest"] != digest {
		return PlanFile{}, fmt.Errorf("%w: plan digest does not match its covered members", errPlanFile)
	}
	d := &recordDecoder{}
	for _, name := range []string{"generated_at", "inspected_at"} {
		d.timestamp(object[name])
	}
	if raw, present := object["expires_at"]; present {
		d.timestamp(raw)
	}
	intent := d.intent(object["immutable_intent"])
	planID, _ := object["plan_id"].(string)
	if d.err != nil {
		return PlanFile{}, fmt.Errorf("%w: %v", errPlanFile, d.err)
	}
	if err := firstError(intent.Validate(), PlanID(planID).Validate()); err != nil {
		return PlanFile{}, fmt.Errorf("%w: %v", errPlanFile, err)
	}
	e := &canonicalEncoder{}
	again, err := MarshalCanonical(e.intent(intent))
	original, originalErr := MarshalCanonical(object["immutable_intent"])
	if err != nil || originalErr != nil || e.err != nil || !bytes.Equal(again, original) {
		return PlanFile{}, fmt.Errorf("%w: immutable intent does not round-trip", errPlanFile)
	}
	return PlanFile{PlanID: PlanID(planID), PlanDigest: PlanDigest(digest), Intent: intent}, nil
}

func (d *recordDecoder) stringMap(v any) map[string]string {
	object, ok := v.(map[string]any)
	if !ok {
		d.fail("map")
		return nil
	}
	values := make(map[string]string, len(object))
	for key, item := range object {
		text, ok := item.(string)
		if !ok {
			d.fail("map value")
		}
		values[key] = text
	}
	return values
}

func (d *recordDecoder) tool(v any) GitToolIdentity {
	object := d.object(v, "tool identity", []string{
		"invocation_mode", "invocation_path", "invocation_path_digest", "invocation_device", "invocation_inode",
		"effective_executable_path", "effective_executable_digest", "version", "exec_path", "capabilities", "environment_selectors",
	}, nil)
	tool, err := NewGitToolIdentity(GitToolIdentityInput{
		InvocationMode: InvocationMode(d.text(object, "invocation_mode")), InvocationPath: d.text(object, "invocation_path"),
		InvocationPathDigest: ContentDigest(d.text(object, "invocation_path_digest")), InvocationDevice: d.text(object, "invocation_device"),
		InvocationInode: d.text(object, "invocation_inode"), EffectiveExecutablePath: d.text(object, "effective_executable_path"),
		EffectiveExecutableDigest: ContentDigest(d.text(object, "effective_executable_digest")), Version: d.text(object, "version"),
		ExecPath: d.text(object, "exec_path"), Capabilities: decodeStrings[string](d, object["capabilities"]),
		EnvironmentSelectors: d.stringMap(object["environment_selectors"]),
	})
	if err != nil {
		d.fail("tool identity")
	}
	return tool
}

func (d *recordDecoder) gitIdentity(v any) GitIdentity {
	object := d.object(v, "Git identity", []string{"name", "email"}, nil)
	return GitIdentity{Name: d.text(object, "name"), Email: d.text(object, "email")}
}

func (d *recordDecoder) intent(v any) ImmutableIntent {
	object := d.object(v, "immutable intent", []string{
		"project_id", "strategy", "branch_parameters", "prefix_parameters", "initialize_repository", "object_format",
		"ref_format", "tool_identity", "project_config_bytes_digest", "initial_commit", "authorized_files", "required_capabilities",
	}, []string{
		"git_flow_dialect", "development_base", "author", "committer", "author_timestamp", "committer_timestamp",
		"commit_message_bytes_base64url",
	})
	in := ImmutableIntentInput{
		ProjectID: ProjectID(d.text(object, "project_id")), Strategy: Strategy(d.text(object, "strategy")),
		GitFlowDialect: GitFlowDialect(d.optionalText(object, "git_flow_dialect")), BranchParameters: d.stringMap(object["branch_parameters"]),
		PrefixParameters: d.stringMap(object["prefix_parameters"]), InitializeRepository: d.boolean(object, "initialize_repository"),
		ObjectFormat: ObjectFormat(d.text(object, "object_format")), RefFormat: RefFormat(d.text(object, "ref_format")),
		ToolIdentity: d.tool(object["tool_identity"]), ProjectConfigBytesDigest: ContentDigest(d.text(object, "project_config_bytes_digest")),
		InitialCommit: d.boolean(object, "initial_commit"), AuthorizedFiles: decodeList(d, object["authorized_files"], d.authorizedFile),
		RequiredCapabilities: decodeStrings[string](d, object["required_capabilities"]),
	}
	if raw, present := object["development_base"]; present {
		base := d.object(raw, "development base", []string{"base_oid", "object_format", "production_ref", "production_tip_oid"}, nil)
		in.DevelopmentBase = DevelopmentBase{
			BaseOID: ObjectOID(d.text(base, "base_oid")), ObjectFormat: ObjectFormat(d.text(base, "object_format")),
			ProductionRef: d.ref(base["production_ref"]), ProductionTipOID: ObjectOID(d.text(base, "production_tip_oid")),
		}
	}
	if in.InitialCommit {
		in.Author, in.Committer = d.gitIdentity(object["author"]), d.gitIdentity(object["committer"])
		in.AuthorTimestamp, in.CommitterTimestamp = d.timestamp(object["author_timestamp"]), d.timestamp(object["committer_timestamp"])
		message, err := DecodeReversiblePath(d.text(object, "commit_message_bytes_base64url"))
		if err != nil {
			d.fail("commit message")
		}
		in.CommitMessageBytes = message
	}
	intent, err := NewImmutableIntent(in)
	if err != nil {
		d.fail("immutable intent")
	}
	return intent
}
