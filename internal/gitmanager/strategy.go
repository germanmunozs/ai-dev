package gitmanager

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Branch parameter names of the immutable intent and project configuration.
const (
	BranchInitial     = "initial_branch"
	BranchPrimary     = "primary_branch"
	BranchTrunk       = "trunk_branch"
	BranchProduction  = "production_branch"
	BranchDevelopment = "development_branch"
)

// gitFlowPrefixNames are the prefix parameters of a new Git Flow
// configuration; version_tag is the release and hotfix tag prefix.
var gitFlowPrefixNames = []string{"feature", "bugfix", "release", "hotfix", "support", "version_tag"}

// gitFlowReservedNames occupy the git-flow-next topic namespace
// (F008-FR-170).
var gitFlowReservedNames = []string{"feature", "bugfix", "release", "hotfix", "support"}

// StrategyRequest is the strategy part of a plan request exactly as the
// operator gave it. Only options that were given appear in the maps; no
// value is defaulted or inferred (F008-FR-036, 037, 181).
type StrategyRequest struct {
	Strategy             string
	InitializeRepository bool
	// Branches is keyed by CLI option: initial-branch, production-branch,
	// development-branch and trunk-branch.
	Branches map[string]string
	// Prefixes is keyed by gitFlowPrefixNames.
	Prefixes           map[string]string
	GitFlowDialect     string
	DevelopmentBaseOID string
}

// PlanRejection is a planning refusal with one closed reason.
type PlanRejection struct {
	Reason   Reason
	Detail   string
	Findings []SecurityFinding
}

func (r *PlanRejection) Error() string {
	return fmt.Sprintf("plan refused (%s): %s", r.Reason, r.Detail)
}

// ExitCode follows the CLI contract: usage, strategy-input and output
// errors exit 2; every other refusal exits 1.
func (r *PlanRejection) ExitCode() int {
	switch r.Reason {
	case ReasonInvalidStrategy, ReasonMissingRequiredBranch, ReasonOutputExists, ReasonUnsafeOutputPath,
		ReasonInteractiveTerminalRequired, ReasonOutputFailed:
		return 2
	}
	return 1
}

func reject(reason Reason, format string, args ...any) *PlanRejection {
	return &PlanRejection{Reason: reason, Detail: fmt.Sprintf(format, args...)}
}

// strategyDecision is a validated strategy request against one inspection.
type strategyDecision struct {
	strategy           Strategy
	dialect            GitFlowDialect
	initialize         bool
	initialBranch      string
	branchParameters   map[string]string
	prefixParameters   map[string]string
	gitFlowWrites      []configKV // ordered keys to write
	gitFlowAdopted     []configKV // existing equal keys, verified no-ops
	developmentBase    DevelopmentBase
	developmentPending bool
	warnings           []string
}

// strategyOptions lists, per strategy, the branch options it accepts.
var strategyBranchOptions = map[Strategy][]string{
	StrategyBasic:      {"initial-branch"},
	StrategyGitHubFlow: {"initial-branch"},
	StrategyTrunkBased: {"trunk-branch"},
	StrategyGitFlow:    {"initial-branch", "production-branch", "development-branch"},
}

// decideStrategy validates the request: input errors first (exit 2), then
// repository and configuration conflicts (exit 1).
func decideStrategy(ctx context.Context, git GitReader, req StrategyRequest, inspection GitInspection, environment EnvironmentProfile) (strategyDecision, *PlanRejection) {
	strategy := Strategy(req.Strategy)
	if !strategy.Valid() {
		return strategyDecision{}, reject(ReasonInvalidStrategy, "strategy %q is not one of basic, git-flow, github-flow, trunk-based", req.Strategy)
	}
	d := strategyDecision{strategy: strategy, initialize: req.InitializeRepository, branchParameters: map[string]string{}, prefixParameters: map[string]string{}}
	observation := inspection.Observation
	absent := observation.RepositoryKind == RepositoryKindAbsent

	for option := range req.Branches {
		if !containsValue(strategyBranchOptions[strategy], option) {
			return d, reject(ReasonInvalidStrategy, "--%s is not accepted with strategy %s", option, strategy)
		}
	}
	if strategy != StrategyGitFlow && (len(req.Prefixes) > 0 || req.GitFlowDialect != "") {
		return d, reject(ReasonInvalidStrategy, "Git Flow options are not accepted with strategy %s", strategy)
	}
	if strategy != StrategyGitFlow && req.DevelopmentBaseOID != "" {
		return d, reject(ReasonInvalidStrategy, "--development-base-oid is only accepted with Git Flow")
	}
	if _, given := req.Branches["initial-branch"]; given && !(absent && req.InitializeRepository) {
		return d, reject(ReasonInvalidStrategy, "--initial-branch is only accepted when initializing a new repository")
	}
	for _, name := range req.Branches {
		if rejection := validateBranchName(ctx, git, inspection, environment, name); rejection != nil {
			return d, rejection
		}
	}
	branch := func(option string) (string, *PlanRejection) {
		value, given := req.Branches[option]
		if !given {
			return "", reject(ReasonMissingRequiredBranch, "--%s is required for strategy %s", option, strategy)
		}
		return value, nil
	}
	switch strategy {
	case StrategyBasic, StrategyGitHubFlow:
		if absent && req.InitializeRepository {
			initial, rejection := branch("initial-branch")
			if rejection != nil {
				return d, rejection
			}
			name := BranchInitial
			if strategy == StrategyGitHubFlow {
				name = BranchPrimary
			}
			d.initialBranch, d.branchParameters[name] = initial, initial
		}
	case StrategyTrunkBased:
		trunk, rejection := branch("trunk-branch")
		if rejection != nil {
			return d, rejection
		}
		d.branchParameters[BranchTrunk] = trunk
		d.initialBranch = trunk
	case StrategyGitFlow:
		if rejection := d.decideGitFlow(ctx, git, req, inspection, environment, branch); rejection != nil {
			return d, rejection
		}
	}
	if !absent || !req.InitializeRepository {
		d.initialBranch = ""
	}

	switch {
	case absent && !req.InitializeRepository:
		return d, reject(ReasonRepositoryAbsent, "the project has no repository and --initialize-repository was not given")
	case !absent && req.InitializeRepository:
		return d, reject(ReasonRepositoryStateIncompatible, "the project already has a repository; it is never reinitialized")
	}
	if strategy != StrategyGitFlow && observation.GitFlowEvidence.Classification != GitFlowAbsent {
		d.warnings = append(d.warnings, "detected_git_flow_not_selected: existing gitflow keys are left unchanged")
	}
	return d, nil
}

func (d *strategyDecision) decideGitFlow(ctx context.Context, git GitReader, req StrategyRequest, inspection GitInspection,
	environment EnvironmentProfile, branch func(string) (string, *PlanRejection)) *PlanRejection {
	production, rejection := branch("production-branch")
	if rejection != nil {
		return rejection
	}
	development, rejection := branch("development-branch")
	if rejection != nil {
		return rejection
	}
	if initial, given := req.Branches["initial-branch"]; given && initial != production {
		return reject(ReasonInvalidStrategy, "the initial branch of a new Git Flow repository must be the production branch")
	}
	if production == development {
		return reject(ReasonInvalidStrategy, "production and development branches must differ")
	}
	dialect := GitFlowDialect(req.GitFlowDialect)
	if req.GitFlowDialect != "" && !dialect.Valid() {
		return reject(ReasonInvalidStrategy, "Git Flow dialect %q is not avh or git-flow-next", req.GitFlowDialect)
	}
	if dialect == GitFlowDialectNext && (containsValue(gitFlowReservedNames, production) || containsValue(gitFlowReservedNames, development)) {
		return reject(ReasonInvalidStrategy, "git-flow-next reserves the names feature, bugfix, release, hotfix and support")
	}
	for name, prefix := range req.Prefixes {
		if !containsValue(gitFlowPrefixNames, name) {
			return reject(ReasonInvalidStrategy, "unknown Git Flow prefix %q", name)
		}
		if rejection := validateBranchName(ctx, git, inspection, environment, prefix+"example"); rejection != nil {
			return reject(ReasonInvalidStrategy, "Git Flow prefix %q cannot form a valid ref", name)
		}
	}
	d.dialect, d.initialBranch = dialect, production
	d.branchParameters[BranchProduction], d.branchParameters[BranchDevelopment] = production, development

	evidence := inspection.Observation.GitFlowEvidence
	switch evidence.Classification {
	case GitFlowAbsent:
		if dialect == "" {
			return reject(ReasonInvalidStrategy, "a new Git Flow configuration needs --git-flow-dialect")
		}
		for _, name := range gitFlowPrefixNames {
			prefix, given := req.Prefixes[name]
			if !given || prefix == "" {
				return reject(ReasonInvalidStrategy, "a new Git Flow configuration needs a non-empty --%s-prefix", strings.ReplaceAll(name, "_", "-"))
			}
			d.prefixParameters[name] = prefix
		}
		if dialect == GitFlowDialectAVH {
			d.gitFlowWrites = gitFlowAVHEntries(production, development, d.prefixParameters)
		} else {
			d.gitFlowWrites = gitFlowNextEntries(production, development, d.prefixParameters, d.prefixParameters["version_tag"])
		}
	case GitFlowCompleteAVH, GitFlowCompleteGitFlowNext:
		if rejection := d.adoptGitFlow(req, evidence, production, development); rejection != nil {
			return rejection
		}
	default:
		return reject(ReasonStrategyConflict, "existing gitflow configuration is %s and is not normalized", evidence.Classification)
	}
	return d.decideDevelopment(req, inspection, production, development)
}

// adoptGitFlow accepts a complete existing profile only when every
// requested value equals it; its keys become verified no-ops.
func (d *strategyDecision) adoptGitFlow(req StrategyRequest, evidence GitFlowEvidence, production, development string) *PlanRejection {
	if req.GitFlowDialect != "" && GitFlowDialect(req.GitFlowDialect) != evidence.Dialect {
		return reject(ReasonStrategyConflict, "the requested dialect differs from the existing %s profile", evidence.Dialect)
	}
	if evidence.ProductionBranch != production || evidence.DevelopmentBranch != development {
		return reject(ReasonStrategyConflict, "the requested branches differ from the existing Git Flow profile")
	}
	existing := map[string]string{}
	for _, entry := range evidence.Entries() {
		existing[entry.Key] = entry.Value
	}
	prefixKey := func(name string) string {
		if evidence.Dialect == GitFlowDialectAVH {
			return "gitflow.prefix." + strings.ReplaceAll(name, "_", "")
		}
		if name == "version_tag" {
			return "gitflow.branch.release.tagprefix"
		}
		return "gitflow.branch." + name + ".prefix"
	}
	for _, name := range gitFlowPrefixNames {
		value := existing[prefixKey(name)]
		if requested, given := req.Prefixes[name]; given && requested != value {
			return reject(ReasonStrategyConflict, "the requested %s prefix differs from the existing profile", name)
		}
		d.prefixParameters[name] = value
	}
	d.dialect = evidence.Dialect
	keys := make([]string, 0, len(existing))
	for key := range existing {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		d.gitFlowAdopted = append(d.gitFlowAdopted, configKV{key, existing[key]})
	}
	return nil
}

// decideDevelopment applies F008-FR-043–046: without history the
// development branch is pending; with history and an absent development
// ref, only an explicit base equal to the frozen production tip creates it.
func (d *strategyDecision) decideDevelopment(req StrategyRequest, inspection GitInspection, production, development string) *PlanRejection {
	observation := inspection.Observation
	refs := map[string]ObjectOID{}
	for _, ref := range observation.Refs() {
		refs[string(ref.Name.Bytes())] = ref.TargetOID
	}
	_, developmentExists := refs["refs/heads/"+development]
	history := observation.Head.HistoryPresent
	baseAllowed := observation.RepositoryKind != RepositoryKindAbsent && history && !developmentExists
	if req.DevelopmentBaseOID != "" && !baseAllowed {
		return reject(ReasonInvalidStrategy, "--development-base-oid is only accepted for existing history without the development branch")
	}
	switch {
	case developmentExists:
		return nil
	case !history || observation.RepositoryKind == RepositoryKindAbsent:
		d.developmentPending = true
		return nil
	case req.DevelopmentBaseOID == "":
		return reject(ReasonStrategyConflict, "existing history without the development branch needs --development-base-oid")
	}
	algorithm, hex, found := strings.Cut(req.DevelopmentBaseOID, ":")
	format := ObjectFormat(algorithm)
	if !found || !format.Valid() || ObjectOID(hex).ValidateFor(format) != nil {
		return reject(ReasonInvalidStrategy, "--development-base-oid must be ALGORITHM:HEX")
	}
	if format != observation.ObjectFormat {
		return reject(ReasonInvalidStrategy, "--development-base-oid uses %s but the repository uses %s", format, observation.ObjectFormat)
	}
	productionRef := "refs/heads/" + production
	tip, exists := refs[productionRef]
	if !exists || tip != ObjectOID(hex) {
		return reject(ReasonStrategyConflict, "the development base is not the frozen production tip")
	}
	d.developmentBase = DevelopmentBase{
		BaseOID: ObjectOID(hex), ObjectFormat: format, ProductionRef: NewObservedRef([]byte(productionRef)), ProductionTipOID: tip,
	}
	return nil
}

// validateBranchName rejects what must never reach Git, then asks Git to
// validate the full ref (plan, Exact Git operation matrix): NUL, C0/C1
// controls, line breaks, a leading dash, the @{ syntax, non-UTF-8 bytes and
// the literal HEAD are refused before Git runs.
func validateBranchName(ctx context.Context, git GitReader, inspection GitInspection, environment EnvironmentProfile, name string) *PlanRejection {
	invalidName := reject(ReasonInvalidStrategy, "branch name %q is not a valid explicit branch", name)
	if name == "" || name == "HEAD" || strings.HasPrefix(name, "-") || strings.Contains(name, "@{") || !utf8.ValidString(name) {
		return invalidName
	}
	for _, r := range name {
		if unicode.IsControl(r) || r == ' ' || r == ' ' {
			return invalidName
		}
	}
	if git == nil {
		return reject(ReasonGitCapabilityMissing, "branch names cannot be validated without Git")
	}
	result, err := git.Read(ctx, GitReadRequest{
		Row: GitReadCheckRefFormat, Tool: inspection.ToolIdentity, Environment: environment, Directory: []byte("/"),
		BranchName: []byte(name),
	})
	if err != nil || result.ExitCode != 0 {
		return invalidName
	}
	return nil
}
