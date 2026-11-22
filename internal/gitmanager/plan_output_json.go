package gitmanager

import "sort"

// Output schemas of the plan command.
const (
	GitPlanOutputSchema          = "ai-dev.git-plan-output/v1"
	GitPlanRejectionOutputSchema = "ai-dev.git-plan-rejection/v1"
	GitStrategyReportSchema      = "ai-dev.git-strategy-report/v1"
)

// ApprovalTuples are the exact normal and ignored CLI approval forms,
// derived solely from the intent's authorized files.
func ApprovalTuples(intent ImmutableIntent) []string {
	var tuples []string
	for _, file := range intent.AuthorizedFiles() {
		tuples = append(tuples, file.PathUTF8+"|"+file.Mode+"|"+string(file.ContentSHA256)+"|blob:"+string(file.BlobOID))
	}
	sort.Strings(tuples)
	return tuples
}

// EncodePlanOutput renders the GitPlanOutput object (data-model §15).
func EncodePlanOutput(result PlanResult, planPath string) ([]byte, error) {
	plan := result.Plan
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	e := &canonicalEncoder{}
	intent := plan.ImmutableIntent
	var tuples []any
	for _, file := range intent.AuthorizedFiles() {
		kind := "normal"
		if file.Ignored {
			kind = "ignored"
		}
		tuples = append(tuples, map[string]any{
			"approval_kind": kind,
			"tuple":         file.PathUTF8 + "|" + file.Mode + "|" + string(file.ContentSHA256) + "|blob:" + string(file.BlobOID),
		})
	}
	if tuples == nil {
		tuples = []any{}
	}
	report := e.strategyReport(StrategyReportFromPlan(result))
	if e.err != nil {
		return nil, e.err
	}
	return MarshalCanonical(map[string]any{
		"schema_version":      GitPlanOutputSchema,
		"plan_id":             string(plan.PlanID),
		"plan_digest":         string(plan.PlanDigest),
		"canonical_plan_path": e.path(NewObservedPath([]byte(planPath))),
		"strategy_report":     report,
		"ordered_operations":  canonicalList(plan.derivedOperations, e.operation),
		"approval_tuples":     tuples,
		"warnings":            canonicalStrings(result.Warnings),
	})
}

// EncodePlanRejection renders the GitPlanRejectionOutput object: one closed
// reason, no partial plan and no sensitive source data.
func EncodePlanRejection(rejection *PlanRejection, warnings []string) ([]byte, error) {
	if rejection == nil || !rejection.Reason.Valid() {
		return nil, invalid("plan rejection")
	}
	e := &canonicalEncoder{}
	return MarshalCanonical(map[string]any{
		"schema_version": GitPlanRejectionOutputSchema,
		"status":         string(ApplyStatusRefused),
		"reason":         string(rejection.Reason),
		"exit_code":      canonicalInteger(uint64(rejection.ExitCode())),
		"findings":       canonicalList(rejection.Findings, func(f SecurityFinding) any { return e.securityFinding(f) }),
		"warnings":       canonicalStrings(warnings),
		"retryable":      false,
	})
}

func (e *canonicalEncoder) strategyReport(r GitStrategyReport) map[string]any {
	report := map[string]any{
		"schema_version":            GitStrategyReportSchema,
		"project_identity_digest":   string(r.ProjectIdentityDigest),
		"selected_strategy":         string(r.SelectedStrategy),
		"source":                    r.Source,
		"branch_parameters":         canonicalMap(r.BranchParameters),
		"detected_evidence":         canonicalList(r.DetectedEvidence, e.gitFlow),
		"compatibility":             r.Compatibility,
		"pending_dependencies":      canonicalStrings(r.PendingDependencies),
		"warnings":                  canonicalStrings(r.Warnings),
		"grants_mutation_authority": r.GrantsMutationAuthority(),
	}
	optional(report, "git_flow_dialect", string(r.GitFlowDialect), r.GitFlowDialect != "")
	return report
}

// EncodeStrategyReport renders one GitStrategyReport object.
func EncodeStrategyReport(r GitStrategyReport) ([]byte, error) {
	e := &canonicalEncoder{}
	report := e.strategyReport(r)
	if e.err != nil {
		return nil, e.err
	}
	return MarshalCanonical(report)
}
