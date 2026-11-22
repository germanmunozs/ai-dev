package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/gitmanager"
)

func TestGitStrategyReportText(t *testing.T) {
	r := gitmanager.GitStrategyReport{
		ProjectIdentityDigest: gitmanager.ProjectIdentityDigest("sha256:" + strings.Repeat("a", 64)), SelectedStrategy: gitmanager.StrategyTrunkBased,
		Source: gitmanager.StrategySourceProjectConfig, BranchParameters: map[string]string{"trunk_branch": "main"},
		Compatibility: gitmanager.StrategyCompatible,
	}
	var text, json bytes.Buffer
	if err := WriteGitStrategyReportText(&text, r); err != nil {
		t.Fatal(err)
	}
	if err := WriteGitStrategyReportJSON(&json, r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text.String(), "strategy:       trunk-based (from project_config)\n") ||
		!strings.Contains(text.String(), "branch:         trunk_branch = main\n") ||
		!strings.HasSuffix(text.String(), "authority:      none; this report grants no mutation authority\n") {
		t.Fatalf("text %q", text.String())
	}
	if !strings.Contains(json.String(), `"grants_mutation_authority":false`) || strings.Count(json.String(), "\n") != 1 {
		t.Fatalf("json %q", json.String())
	}
}
