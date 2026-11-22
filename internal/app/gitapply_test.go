package app

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func (f *gitAppFixture) run(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := RunGitInit(context.Background(), args, f.deps, nil, false, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// Apply runs through the engine with a nonmutating executor; real
// mutation dispatch is wired only in T107.
func TestApplyAdapterIssueAndConsume(t *testing.T) {
	f := newGitAppFixture(t)
	planPath := filepath.Join(f.base, "plan.json")
	code, stdout, stderr := f.run("plan", "--project-dir", f.project, "--strategy", "basic", "--initialize-repository",
		"--initial-branch", "main", "--output", planPath, "--format", "json")
	if code != 0 {
		t.Fatalf("plan exit %d %s %s", code, stdout, stderr)
	}
	var plan struct {
		Digest string `json:"plan_digest"`
	}
	if err := json.Unmarshal([]byte(stdout), &plan); err != nil {
		t.Fatal(err)
	}
	const id = "123e4567-e89b-42d3-a456-426614174088"
	code, stdout, _ = f.run("apply", "--plan", planPath, "--confirm-plan", plan.Digest, "--authorization-id", id,
		"--issue-authorization", "--authorized-by", "operator", "--format", "json")
	var output struct {
		Schema string `json:"schema_version"`
		Result struct {
			Status string `json:"status"`
			State  string `json:"durable_authorization_state"`
			Exit   int    `json:"exit_code"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(stdout), &output); err != nil || code != 0 || output.Schema != "ai-dev.git-apply-output/v1" ||
		output.Result.Status != "applied" || output.Result.State != "applied" || f.executor.executed == 0 {
		t.Fatalf("apply exit %d %s", code, stdout)
	}
	executed := f.executor.executed
	code, stdout, _ = f.run("apply", "--plan", planPath, "--confirm-plan", plan.Digest, "--authorization-id", id, "--format", "json")
	if err := json.Unmarshal([]byte(stdout), &output); err != nil || code != 1 || output.Result.Status != "refused" || f.executor.executed != executed {
		t.Fatalf("consumed apply exit %d %s", code, stdout)
	}
}

func TestApplyAdapterUsage(t *testing.T) {
	f := newGitAppFixture(t)
	for _, args := range [][]string{
		{"apply"},
		{"apply", "--plan", "p", "--confirm-plan", "sha256:x"},
		{"apply", "--plan", "p", "--confirm-plan", "sha256:x", "--authorization-id", "a", "--authorized-by", "op"},
		{"apply", "--plan", "p", "--confirm-plan", "sha256:x", "--authorization-id", "a", "--approve-file", "x"},
		{"apply", "--plan", "p", "--confirm-plan", "sha256:x", "--authorization-id", "a", "--format", "yaml"},
	} {
		if code, _, _ := f.run(args...); code != 2 {
			t.Errorf("%v exit %d, want 2", args, code)
		}
	}
	// Without an executor apply is refused before any record exists.
	f.deps.Executor = nil
	code, _, stderr := f.run("apply", "--plan", "p", "--confirm-plan", "sha256:x", "--authorization-id", "a", "--issue-authorization", "--authorized-by", "op")
	if code != 2 || f.stateExists() || stderr == "" {
		t.Fatalf("disabled apply exit %d, state %t", code, f.stateExists())
	}
}
