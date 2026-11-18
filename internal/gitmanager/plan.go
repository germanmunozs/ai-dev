package gitmanager

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
)

// PlannerDependencies are the ports planning uses. None of them mutates:
// Git only validates ref names and the builder only builds argv.
type PlannerDependencies struct {
	Git      GitReader
	Commands GitCommandBuilder
	Random   RandomSource
}

// PlanMetadata is excluded plan metadata; it never affects the digest.
type PlanMetadata struct {
	PlanID      PlanID
	GeneratedAt time.Time
}

// PlanResult is a generated plan with its informational outcome.
type PlanResult struct {
	Plan                GitInitializationPlan
	Warnings            []string
	PendingDependencies []Reason
}

// Plan validates an explicit strategy request against an inspection,
// freezes the immutable intent and derives the canonical plan (T055).
func Plan(ctx context.Context, deps PlannerDependencies, req StrategyRequest, inspection GitInspection, environment EnvironmentProfile, meta PlanMetadata) (PlanResult, *PlanRejection) {
	decision, rejection := decideStrategy(ctx, deps.Git, req, inspection, environment)
	if rejection != nil {
		return PlanResult{}, rejection
	}
	if rejection := inspectionRejection(inspection); rejection != nil {
		return PlanResult{}, rejection
	}
	projectID := inspection.Observation.ProjectConfig.ProjectID
	if inspection.Observation.ProjectConfig.State != ProjectConfigValid {
		id, err := newProjectID(deps.Random)
		if err != nil {
			return PlanResult{}, reject(ReasonPlanInvalid, "a project ID cannot be generated: %v", err)
		}
		projectID = id
	}
	intent, rejection := buildIntent(decision, inspection, projectID)
	if rejection != nil {
		return PlanResult{}, rejection
	}
	plan, rejection := Regenerate(ctx, deps, intent, inspection, environment, meta)
	if rejection != nil {
		return PlanResult{}, rejection
	}
	result := PlanResult{Plan: plan, Warnings: append(decision.warnings, inspection.Warnings()...)}
	if decision.developmentPending {
		result.PendingDependencies = []Reason{ReasonDevelopmentBranchPendingInitialHistory}
	}
	return result, nil
}

// Regenerate derives the canonical plan from a validated immutable intent
// and an inspection. It is deterministic: the same intent and the same
// covered inspection state give byte-identical covered content and digest.
// A refusal here is the initial-phase reason; apply translates drift with
// the c-01 classifier.
func Regenerate(ctx context.Context, deps PlannerDependencies, intent ImmutableIntent, inspection GitInspection, environment EnvironmentProfile, meta PlanMetadata) (GitInitializationPlan, *PlanRejection) {
	if err := intent.Validate(); err != nil {
		return GitInitializationPlan{}, reject(ReasonPlanInvalid, "immutable intent: %v", err)
	}
	if intent.ToolIdentity.Validate() != nil || !sameTool(intent.ToolIdentity, inspection.ToolIdentity) {
		return GitInitializationPlan{}, reject(ReasonRepositoryStateIncompatible, "the Git toolchain differs from the one the intent froze")
	}
	snapshot, rejection := buildSnapshot(inspection, environment, intent.DevelopmentBase)
	if rejection != nil {
		return GitInitializationPlan{}, rejection
	}
	operations, postconditions, rejection := deriveOperations(deps.Commands, intent, snapshot)
	if rejection != nil {
		return GitInitializationPlan{}, rejection
	}
	in := GitInitializationPlanInput{
		SchemaVersion: GitInitializationPlanSchema, PlanID: meta.PlanID, GeneratedAt: meta.GeneratedAt,
		InspectionID: inspection.InspectionID, InspectedAt: inspection.InspectedAt, ImmutableIntent: intent,
		PreconditionSnapshot: snapshot, DerivedOperations: operations, ExpectedPostconditions: postconditions,
		ProhibitedOperations: ProhibitedOperations(), RecoveryPolicy: RecoveryPolicySupervisedReconciliation,
	}
	for _, operation := range operations {
		if operation.PartialEffectClassification != PartialEffectNone {
			in.PartialEffectPoints = append(in.PartialEffectPoints, operation.OperationID)
		}
	}
	digest, err := ComputePlanDigest(in)
	if err != nil {
		return GitInitializationPlan{}, limitOrInvalid(err)
	}
	in.PlanDigest = digest
	plan, err := NewGitInitializationPlan(in)
	if err != nil {
		return GitInitializationPlan{}, limitOrInvalid(err)
	}
	return plan, nil
}

// limitOrInvalid maps a construction failure to its closed reason: an
// exceeded limit keeps its catalogue reason, anything else is plan_invalid.
func limitOrInvalid(err error) *PlanRejection {
	var limit *LimitError
	if errors.As(err, &limit) {
		return reject(limit.Reason, "%v", err)
	}
	return reject(ReasonPlanInvalid, "%v", err)
}

// sameTool compares frozen toolchain identities through their canonical
// encoding.
func sameTool(a, b GitToolIdentity) bool {
	e := &canonicalEncoder{}
	left, errLeft := MarshalCanonical(e.tool(a))
	right, errRight := MarshalCanonical(e.tool(b))
	return errLeft == nil && errRight == nil && string(left) == string(right)
}

func pathSubject(path string) string { return EncodeReversiblePath([]byte(path)) }
func refSubject(ref string) string   { return EncodeReversiblePath([]byte(ref)) }

// deriveOperations enumerates every mutation of the plan in the definitive
// order (plan, Definitive global apply ordering step 6), each with its
// complete preconditions, effects, postconditions, failure outcomes,
// partial-effect classification and recovery instruction (F008-FR-140).
func deriveOperations(commands GitCommandBuilder, intent ImmutableIntent, snapshot PreconditionSnapshot) ([]DerivedOperation, []Condition, *PlanRejection) {
	if commands == nil {
		return nil, nil, reject(ReasonGitCapabilityMissing, "no Git command builder")
	}
	observation := snapshot.Observation
	root := snapshot.ProjectIdentity.ReversiblePathBytes()
	target := "project:" + string(snapshot.ProjectIdentity.Digest())
	var drafts []DerivedOperationInput
	mutation := func(in DerivedOperationInput) {
		if in.PartialEffectClassification == "" {
			in.PartialEffectClassification = PartialEffectAtomic
		}
		in.RecoveryInstruction = RecoverySupervisedReconciliation
		drafts = append(drafts, in)
	}
	verification := func(in DerivedOperationInput) {
		in.Kind, in.PartialEffectClassification, in.RecoveryInstruction = OperationKindVerification, PartialEffectNone, RecoveryNotApplicable
		drafts = append(drafts, in)
	}
	build := func(command GitCommand) ([]string, *PlanRejection) {
		invocation, rejection := buildCommand(commands, intent, snapshot, command)
		return invocation.Args, rejection
	}
	repositoryNormal := Condition{Kind: ConditionRepositoryKind, Expected: string(RepositoryKindNormal)}

	// 1. Deterministic initialization of a new repository.
	branches := intent.BranchParameters()
	if intent.InitializeRepository {
		initial := initialBranchOf(intent.Strategy, branches)
		argv, rejection := build(GitCommand{Kind: GitCommandInit, BranchName: []byte(initial), PhysicalRoot: root})
		if rejection != nil {
			return nil, nil, rejection
		}
		if rejection := checkInitArgv(intent, root, argv); rejection != nil {
			return nil, nil, rejection
		}
		headRef := Condition{Kind: ConditionHeadSymbolicRef, Subject: refSubject("refs/heads/" + initial)}
		mutation(DerivedOperationInput{
			Kind: OperationKindRepositoryInit, ExactArgv: argv, EnvironmentProfile: snapshot.EnvironmentProfile.Version,
			TargetIdentity: target, AffectedPaths: []ObservedPath{NewObservedPath([]byte(".git"))},
			AffectedRefs:    []ObservedRef{NewObservedRef([]byte("refs/heads/" + initial))},
			Preconditions:   []Condition{{Kind: ConditionRepositoryAbsent}, {Kind: ConditionPathAbsent, Subject: pathSubject(".git")}},
			ExpectedEffects: []Condition{repositoryNormal, headRef},
			ExpectedPostconditions: []Condition{repositoryNormal, {Kind: ConditionHeadState, Expected: string(HeadStateUnborn)}, headRef,
				{Kind: ConditionConfigValue, Subject: "core.bare", Expected: "false"}},
			FailureOutcomes:             []Reason{ReasonOperationFailed, ReasonGitCapabilityMissing, ReasonConcurrentChange},
			PartialEffectClassification: PartialEffectPartialPossible,
		})
	}

	// 2. Git Flow configuration: one ordered write per key, or verified
	// no-ops for an adopted equal profile (F008-FR-041).
	if intent.Strategy == StrategyGitFlow {
		writes, adopted, rejection := gitFlowPlan(intent, observation)
		if rejection != nil {
			return nil, nil, rejection
		}
		for _, kv := range writes {
			argv, rejection := build(GitCommand{Kind: GitCommandConfigWrite, ConfigKey: kv.key, ConfigValue: kv.value})
			if rejection != nil {
				return nil, nil, rejection
			}
			value := Condition{Kind: ConditionConfigValue, Subject: kv.key, Expected: kv.value}
			mutation(DerivedOperationInput{
				Kind: OperationKindGitConfigWrite, ExactArgv: argv, EnvironmentProfile: snapshot.EnvironmentProfile.Version,
				TargetIdentity: "git_config:local:" + kv.key, AffectedConfigKeys: []string{kv.key},
				Preconditions:   []Condition{repositoryNormal, {Kind: ConditionConfigAbsent, Subject: kv.key}},
				ExpectedEffects: []Condition{value}, ExpectedPostconditions: []Condition{value},
				FailureOutcomes: []Reason{ReasonOperationFailed, ReasonConcurrentChange},
			})
		}
		if len(adopted) > 0 {
			var preconditions []Condition
			for _, kv := range adopted {
				preconditions = append(preconditions, Condition{Kind: ConditionConfigValue, Subject: kv.key, Expected: kv.value})
			}
			verification(DerivedOperationInput{TargetIdentity: "git_config:local:gitflow", Preconditions: preconditions,
				FailureOutcomes: []Reason{ReasonConcurrentChange}})
		}
	}

	// 3–4. The project configuration: a no-op when the existing file holds
	// exactly the rendered bytes, otherwise a guarded directory creation,
	// its barrier, and one atomic write (F008-FR-072–079).
	config, err := renderProjectConfig(intent.ProjectID, intent.Strategy, intent.GitFlowDialect, branches, intent.PrefixParameters())
	if err != nil || contentDigest(config) != intent.ProjectConfigBytesDigest {
		return nil, nil, reject(ReasonPlanInvalid, "the project configuration does not reproduce the frozen digest")
	}
	configContent := Condition{Kind: ConditionPathContent, Subject: pathSubject(ProjectConfigPath), Expected: string(intent.ProjectConfigBytesDigest)}
	configMode := Condition{Kind: ConditionPathMode, Subject: pathSubject(ProjectConfigPath), Expected: "100644"}
	switch existing := observation.ProjectConfig; existing.State {
	case ProjectConfigValid, ProjectConfigInvalid:
		if existing.ContentDigest != intent.ProjectConfigBytesDigest {
			return nil, nil, reject(ReasonConfigurationConflict, ".ai-dev/config.json exists with different content")
		}
		verification(DerivedOperationInput{TargetIdentity: "path:" + ProjectConfigPath, Preconditions: []Condition{configContent}})
	case ProjectConfigAbsent:
		configPreconditions := []Condition{{Kind: ConditionPathAbsent, Subject: pathSubject(ProjectConfigPath)}}
		directory, exists := pathIdentity(observation, ".ai-dev")
		directoryMode := Condition{Kind: ConditionPathMode, Subject: pathSubject(".ai-dev"), Expected: "040755"}
		switch {
		case exists && !directory.Identity.Mode.IsDir():
			return nil, nil, reject(ReasonConfigurationConflict, ".ai-dev exists and is not a directory")
		case exists:
			configPreconditions = append(configPreconditions, Condition{Kind: ConditionPathIdentity, Subject: pathSubject(".ai-dev"),
				Expected: directory.Identity.DeviceDecimal + ":" + directory.Identity.InodeDecimal})
		default:
			mutation(DerivedOperationInput{
				Kind: OperationKindCreateProjectConfigDirectory, TargetIdentity: "path:.ai-dev",
				AffectedPaths:   []ObservedPath{NewObservedPath([]byte(".ai-dev"))},
				Preconditions:   []Condition{{Kind: ConditionPathAbsent, Subject: pathSubject(".ai-dev")}},
				ExpectedEffects: []Condition{directoryMode}, ExpectedPostconditions: []Condition{directoryMode},
				FailureOutcomes: []Reason{ReasonOperationFailed, ReasonConcurrentChange},
			})
			// The barrier revalidates the new directory and the still-absent
			// target before any configuration temporary is created.
			verification(DerivedOperationInput{TargetIdentity: "path:.ai-dev", Preconditions: []Condition{directoryMode,
				{Kind: ConditionPathAbsent, Subject: pathSubject(ProjectConfigPath)}}, FailureOutcomes: []Reason{ReasonConcurrentChange}})
		}
		mutation(DerivedOperationInput{
			Kind: OperationKindProjectConfigWrite, TargetIdentity: "path:" + ProjectConfigPath,
			AffectedPaths:   []ObservedPath{NewObservedPath([]byte(ProjectConfigPath))},
			Preconditions:   configPreconditions,
			ExpectedEffects: []Condition{configContent}, ExpectedPostconditions: []Condition{configContent, configMode},
			FailureOutcomes:        []Reason{ReasonOperationFailed, ReasonConcurrentChange},
			TemporaryResourceRoles: []ResourceRole{ResourceRoleConfigTemporary},
		})
	default:
		return nil, nil, reject(ReasonRepositoryStateIncompatible, ".ai-dev/config.json cannot be read")
	}

	// 5. The development ref of existing history, created only from the
	// explicit base equal to the frozen production tip (F008-FR-046).
	if !intent.DevelopmentBase.IsZero() {
		ref := "refs/heads/" + branches[BranchDevelopment]
		base := intent.DevelopmentBase
		argv, rejection := build(GitCommand{Kind: GitCommandUpdateRef, RefName: []byte(ref), ObjectOID: base.BaseOID})
		if rejection != nil {
			return nil, nil, rejection
		}
		created := Condition{Kind: ConditionRefTarget, Subject: refSubject(ref), Expected: string(base.BaseOID)}
		mutation(DerivedOperationInput{
			Kind: OperationKindDevelopmentRefCAS, ExactArgv: argv, EnvironmentProfile: snapshot.EnvironmentProfile.Version,
			TargetIdentity: "ref:" + ref, AffectedRefs: []ObservedRef{NewObservedRef([]byte(ref))},
			Preconditions: []Condition{repositoryNormal, {Kind: ConditionRefAbsent, Subject: refSubject(ref)},
				{Kind: ConditionRefTarget, Subject: EncodeReversiblePath(base.ProductionRef.Bytes()), Expected: string(base.ProductionTipOID)},
				{Kind: ConditionObjectType, Subject: string(base.BaseOID), Expected: "commit"}},
			ExpectedEffects:        []Condition{created},
			ExpectedPostconditions: []Condition{created, {Kind: ConditionReflogAbsent, Subject: refSubject(ref)}},
			FailureOutcomes:        []Reason{ReasonOperationFailed, ReasonConcurrentChange},
		})
	}

	var operations []DerivedOperation
	var postconditions []Condition
	seen := map[Condition]bool{}
	for ordinal, draft := range drafts {
		draft.Ordinal = uint64(ordinal)
		id, err := DeriveOperationID(draft, snapshot.EnvironmentProfile)
		if err != nil {
			return nil, nil, limitOrInvalid(err)
		}
		draft.OperationID = id
		operation, err := NewDerivedOperation(draft)
		if err != nil {
			return nil, nil, limitOrInvalid(err)
		}
		operations = append(operations, operation)
		conditions := draft.ExpectedPostconditions
		if draft.Kind == OperationKindVerification {
			conditions = draft.Preconditions
		}
		for _, condition := range conditions {
			if !seen[condition] {
				seen[condition] = true
				postconditions = append(postconditions, condition)
			}
		}
	}
	return operations, postconditions, nil
}

// buildCommand builds one mutating row under the frozen toolchain, the
// snapshot's environment and the project root. Planning records its argv;
// the dispatcher rebuilds it and runs it only when the argv is unchanged.
func buildCommand(commands GitCommandBuilder, intent ImmutableIntent, snapshot PreconditionSnapshot, command GitCommand) (GitInvocation, *PlanRejection) {
	if commands == nil {
		return GitInvocation{}, reject(ReasonGitCapabilityMissing, "no Git command builder")
	}
	command.Tool, command.Environment = intent.ToolIdentity, snapshot.EnvironmentProfile
	command.Directory = snapshot.ProjectIdentity.ReversiblePathBytes()
	invocation, err := commands.BuildCommand(command)
	if err != nil {
		type reasoned interface{ ClosedReason() Reason }
		if r, ok := err.(reasoned); ok {
			return GitInvocation{}, reject(r.ClosedReason(), "%v", err)
		}
		return GitInvocation{}, reject(ReasonPlanInvalid, "%v", err)
	}
	return invocation, nil
}

func initialBranchOf(strategy Strategy, branches map[string]string) string {
	switch strategy {
	case StrategyGitHubFlow:
		return branches[BranchPrimary]
	case StrategyTrunkBased:
		return branches[BranchTrunk]
	case StrategyGitFlow:
		return branches[BranchProduction]
	default:
		return branches[BranchInitial]
	}
}

// gitFlowPlan derives the Git Flow writes from the intent against the
// snapshot's evidence: an absent profile is written key by key, a complete
// equal profile is adopted, anything else is a conflict.
func gitFlowPlan(intent ImmutableIntent, observation RepositoryObservation) ([]configKV, []configKV, *PlanRejection) {
	branches, prefixes := intent.BranchParameters(), intent.PrefixParameters()
	production, development := branches[BranchProduction], branches[BranchDevelopment]
	var profile []configKV
	if intent.GitFlowDialect == GitFlowDialectAVH {
		profile = gitFlowAVHEntries(production, development, prefixes)
	} else {
		profile = gitFlowNextEntries(production, development, prefixes, prefixes["version_tag"])
	}
	evidence := observation.GitFlowEvidence
	switch evidence.Classification {
	case GitFlowAbsent:
		return profile, nil, nil
	case GitFlowCompleteAVH, GitFlowCompleteGitFlowNext:
		if evidence.Dialect != intent.GitFlowDialect {
			return nil, nil, reject(ReasonStrategyConflict, "the existing Git Flow dialect differs")
		}
		existing := map[string]string{}
		for _, entry := range evidence.Entries() {
			existing[entry.Key] = entry.Value
		}
		for _, kv := range profile {
			if value, ok := existing[kv.key]; !ok || value != kv.value {
				return nil, nil, reject(ReasonStrategyConflict, "the existing Git Flow profile differs at %s", kv.key)
			}
		}
		adopted := make([]configKV, len(profile))
		copy(adopted, profile)
		sort.Slice(adopted, func(i, j int) bool { return adopted[i].key < adopted[j].key })
		return nil, adopted, nil
	default:
		return nil, nil, reject(ReasonStrategyConflict, "existing gitflow configuration is %s", evidence.Classification)
	}
}

// pathIdentity finds a frozen worktree path identity by repository-relative
// path.
func pathIdentity(observation RepositoryObservation, path string) (WorktreePathIdentity, bool) {
	for _, identity := range observation.WorktreePathIdentities() {
		if strings.TrimSuffix(string(identity.Path.Bytes()), "/") == path {
			return identity, true
		}
	}
	return WorktreePathIdentity{}, false
}
