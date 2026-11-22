package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/gitmanager"
)

// T105: the composed reconciliation inspection reads the prior
// authorization through the real store and writes nothing.
func TestInspectGitReconciliation(t *testing.T) {
	f := newGitAppFixture(t)
	planPath := filepath.Join(f.base, "plan.json")
	if code, stdout, stderr := f.run("plan", "--project-dir", f.project, "--strategy", "basic", "--initialize-repository",
		"--initial-branch", "main", "--output", planPath, "--format", "json"); code != 0 {
		t.Fatalf("plan exit %d %s %s", code, stdout, stderr)
	}
	planFile, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	var plan struct {
		Digest string `json:"plan_digest"`
	}
	if err := json.Unmarshal(planFile, &plan); err != nil {
		t.Fatal(err)
	}
	const id = "123e4567-e89b-42d3-a456-426614174105"
	if code, stdout, _ := f.run("apply", "--plan", planPath, "--confirm-plan", plan.Digest, "--authorization-id", id,
		"--issue-authorization", "--authorized-by", "operator"); code != 0 {
		t.Fatalf("apply exit %d %s", code, stdout)
	}
	before := f.authorizations()
	report, rejection, err := InspectGitReconciliation(context.Background(), f.deps, planFile, id)
	if err != nil || rejection != nil || report.Prior.State != gitmanager.AuthorizationStateApplied || report.Unresolved ||
		!report.TrailReadable || report.GrantsMutationAuthority() {
		t.Fatalf("report %+v %+v %v", report, rejection, err)
	}
	if _, rejection, _ := InspectGitReconciliation(context.Background(), f.deps, []byte("{}"), id); rejection == nil ||
		rejection.Reason != gitmanager.ReasonPlanInvalid {
		t.Fatalf("invalid plan %+v", rejection)
	}
	if f.authorizations() != before {
		t.Fatal("reconciliation inspection wrote an authorization")
	}
}
