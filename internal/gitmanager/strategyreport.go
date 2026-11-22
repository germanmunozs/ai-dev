package gitmanager

import (
	"errors"
	"sort"
)

// Strategy report sources and compatibility values.
const (
	StrategySourcePlan          = "plan"
	StrategySourceProjectConfig = "project_config"

	StrategyCompatible            = "compatible"
	StrategyConfigurationMismatch = "configuration_mismatch"
)

// GitStrategyReport is the read-only, non-authoritative strategy
// description Feature 007 may consume (T096, data-model §10). It carries
// no credential, authorization, plan or mutation capability: building one
// requires none, and GrantsMutationAuthority is always false.
type GitStrategyReport struct {
	ProjectIdentityDigest ProjectIdentityDigest
	SelectedStrategy      Strategy
	Source                string
	GitFlowDialect        GitFlowDialect
	BranchParameters      map[string]string
	DetectedEvidence      []GitFlowEvidence
	Compatibility         string
	PendingDependencies   []Reason
	Warnings              []string
}

// GrantsMutationAuthority is false for every report.
func (GitStrategyReport) GrantsMutationAuthority() bool { return false }

// StrategyReportFromPlan describes the strategy a generated plan selects.
func StrategyReportFromPlan(result PlanResult) GitStrategyReport {
	plan := result.Plan
	intent := plan.ImmutableIntent
	return GitStrategyReport{
		ProjectIdentityDigest: plan.PreconditionSnapshot.ProjectIdentity.Digest(), SelectedStrategy: intent.Strategy,
		Source: StrategySourcePlan, GitFlowDialect: intent.GitFlowDialect, BranchParameters: intent.BranchParameters(),
		DetectedEvidence: []GitFlowEvidence{plan.PreconditionSnapshot.Observation.GitFlowEvidence}, Compatibility: StrategyCompatible,
		PendingDependencies: append([]Reason(nil), result.PendingDependencies...), Warnings: append([]string(nil), result.Warnings...),
	}
}

var errStrategyUnreported = errors.New("the project has no valid configuration matching its inspection")

// StrategyReportFromProjectConfig describes the strategy persisted in
// .ai-dev/config.json. data must be the bytes the inspection observed:
// a valid configuration with the frozen content digest. The report only
// compares that strategy with the detected Git Flow evidence; it decides
// nothing and authorizes nothing.
func StrategyReportFromProjectConfig(inspection GitInspection, data []byte) (GitStrategyReport, error) {
	observed := inspection.Observation.ProjectConfig
	config, valid := parseProjectConfig(data)
	if observed.State != ProjectConfigValid || !valid || contentDigest(data) != observed.ContentDigest {
		return GitStrategyReport{}, errStrategyUnreported
	}
	observation := inspection.Observation
	evidence := observation.GitFlowEvidence
	report := GitStrategyReport{
		ProjectIdentityDigest: inspection.ProjectIdentity.Digest(), SelectedStrategy: config.strategy, Source: StrategySourceProjectConfig,
		GitFlowDialect: config.dialect, BranchParameters: config.branches, DetectedEvidence: []GitFlowEvidence{evidence},
		Compatibility: StrategyCompatible, Warnings: append([]string(nil), inspection.Warnings()...),
	}
	switch {
	case config.strategy != StrategyGitFlow && evidence.Classification != GitFlowAbsent:
		report.Warnings = append(report.Warnings, "detected_git_flow_not_selected: existing gitflow keys are left unchanged")
	case config.strategy == StrategyGitFlow:
		complete := evidence.Classification == GitFlowCompleteAVH || evidence.Classification == GitFlowCompleteGitFlowNext
		if !complete || evidence.Dialect != config.dialect || evidence.ProductionBranch != config.branches[BranchProduction] ||
			evidence.DevelopmentBranch != config.branches[BranchDevelopment] {
			report.Compatibility = StrategyConfigurationMismatch
		}
		development := "refs/heads/" + config.branches[BranchDevelopment]
		present := false
		for _, ref := range observation.Refs() {
			present = present || string(ref.Name.Bytes()) == development
		}
		if !present && !observation.Head.HistoryPresent {
			report.PendingDependencies = []Reason{ReasonDevelopmentBranchPendingInitialHistory}
		}
	}
	sort.Strings(report.Warnings)
	return report, nil
}
