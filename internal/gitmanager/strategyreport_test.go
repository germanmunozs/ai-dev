//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"path/filepath"
	"strings"
	"testing"
)

// T096/AC-020: the strategy report is read-only and non-authoritative, from
// a plan or from the persisted configuration the inspection observed.
func TestStrategyReport(t *testing.T) {
	f := newPlanFixture(t)
	result := mustPlan(t, f, gitFlowRequest("avh"))
	fromPlan := StrategyReportFromPlan(result)
	data, err := EncodeStrategyReport(fromPlan)
	if err != nil || fromPlan.GrantsMutationAuthority() || !strings.Contains(string(data), `"grants_mutation_authority":false`) ||
		!strings.Contains(string(data), `"pending_dependencies":["development_branch_pending_initial_history"]`) {
		t.Fatalf("plan report %s %v", data, err)
	}

	config := renderedConfig(t, result.Plan)
	f = newPlanFixture(t)
	mustWrite(t, filepath.Join(f.root, ProjectConfigPath), string(config))
	inspection := f.inspect()
	report, err := StrategyReportFromProjectConfig(inspection, config)
	if err != nil || report.Source != StrategySourceProjectConfig || report.SelectedStrategy != StrategyGitFlow ||
		report.GitFlowDialect != GitFlowDialectAVH || report.BranchParameters[BranchDevelopment] != "develop" ||
		report.Compatibility != StrategyConfigurationMismatch || len(report.PendingDependencies) != 1 {
		t.Fatalf("configuration report %+v %v", report, err)
	}
	if _, err := StrategyReportFromProjectConfig(inspection, append(config, ' ')); err == nil {
		t.Fatal("bytes the inspection did not observe were reported")
	}
}
