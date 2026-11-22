package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// T109 (after T107): the enabled public CLI through the real entrypoint.
// It covers inspect, plan, issuance and consumption, apply with real Git
// effects, already_applied, reconcile, no-TTY refusal of the assistant,
// exit codes, redacted diagnostics, the absence of implicit mutations and
// refusal of every attempted bypass of the authorized path: a tampered
// plan, a digest that is not the plan's, issuance options when consuming,
// a reused authorization ID and an authorization of another plan. The
// interactive assistant on a real terminal is covered by the T106 harness.
func TestGitManagementPublicCLI(t *testing.T) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available")
	}
	base := productionTempDir(t)
	bin := buildWorkflowNextBinary(t)
	home, project, out := filepath.Join(base, "home"), filepath.Join(base, "project"), filepath.Join(base, "out")
	for _, dir := range []string{home, project, out} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// A credential in the user's configuration must never be output or
	// recorded.
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[remote \"origin\"]\n\turl = https://u:E2E-SECRET@example.org/r.git\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"README.md": "# e2e\n", "notes.txt": "untracked\n"} {
		if err := os.WriteFile(filepath.Join(project, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	message := filepath.Join(out, "message.txt")
	if err := os.WriteFile(message, []byte("feat: start\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := []string{"HOME=" + home, "PATH=" + filepath.Dir(gitPath) + ":/usr/bin:/bin"}
	var outputs strings.Builder
	run := func(args ...string) (string, string, int) {
		cmd := exec.Command(bin, args...)
		cmd.Dir, cmd.Env, cmd.Stdin = base, env, strings.NewReader("")
		stdout, stderr, code := runCommand(cmd)
		outputs.WriteString(stdout + stderr)
		return stdout, stderr, code
	}
	runJSON := func(value any, args ...string) int {
		stdout, stderr, code := run(append(args, "--format", "json")...)
		if err := json.Unmarshal([]byte(stdout), value); err != nil {
			t.Fatalf("%v: exit %d %q %q", args, code, stdout, stderr)
		}
		return code
	}
	type result struct {
		Result struct {
			Status string `json:"status"`
			Reason string `json:"reason"`
			Commit string `json:"commit_oid"`
		} `json:"result"`
	}

	// Inspect and plan change nothing in the project.
	before := treeDigest(t, project)
	if _, _, code := run("init", "inspect", "--project-dir", project); code != 0 {
		t.Fatalf("inspect exit %d", code)
	}
	planPath := filepath.Join(out, "plan.json")
	var plan struct {
		Digest    string `json:"plan_digest"`
		Approvals []struct {
			Tuple string `json:"tuple"`
		} `json:"approval_tuples"`
	}
	if code := runJSON(&plan, "init", "plan", "--project-dir", project, "--strategy", "trunk-based", "--initialize-repository",
		"--trunk-branch", "trunk", "--initial-commit", "--initial-commit-message-file", message, "--include", "README.md",
		"--author-name", "Ada", "--author-email", "ada@example.org", "--committer-name", "Ada", "--committer-email", "ada@example.org",
		"--author-date", "2026-11-18T10:00:00+01:00", "--committer-date", "2026-11-18T10:00:00+01:00", "--output", planPath); code != 0 {
		t.Fatalf("plan exit %d", code)
	}
	if treeDigest(t, project) != before {
		t.Fatal("inspect or plan changed the project")
	}
	const id = "123e4567-e89b-42d3-a456-426614174109"
	issue := []string{"init", "apply", "--plan", planPath, "--confirm-plan", plan.Digest, "--authorization-id", id, "--issue-authorization",
		"--authorized-by", "operator", "--confirm-initial-commit"}
	for _, approval := range plan.Approvals {
		issue = append(issue, "--approve-file", approval.Tuple)
	}

	// Bypass attempts are refused before any mutation.
	tampered := filepath.Join(out, "tampered.json")
	data, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tampered, []byte(strings.Replace(string(data), "--initial-branch=trunk", "--initial-branch=other", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	var refused result
	for name, test := range map[string]struct {
		args   []string
		reason string
	}{
		"tampered plan":  {append([]string{"init", "apply", "--plan", tampered}, issue[4:]...), "plan_invalid"},
		"foreign digest": {append(append([]string{}, issue[:4]...), append([]string{"--confirm-plan", "sha256:" + strings.Repeat("1", 64)}, issue[6:]...)...), "authorization_mismatch"},
		"missing approvals": {[]string{"init", "apply", "--plan", planPath, "--confirm-plan", plan.Digest, "--authorization-id", id,
			"--issue-authorization", "--authorized-by", "operator", "--confirm-initial-commit"}, "authorization_input_incomplete"},
		"unissued consumption": {[]string{"init", "apply", "--plan", planPath, "--confirm-plan", plan.Digest, "--authorization-id", id}, "authorization_missing"},
	} {
		if code := runJSON(&refused, test.args...); code != 1 || refused.Result.Status != "refused" || refused.Result.Reason != test.reason {
			t.Errorf("%s: exit %d %+v", name, code, refused)
		}
	}
	if _, _, code := run("init", "apply", "--plan", planPath, "--confirm-plan", plan.Digest, "--authorization-id", id, "--authorized-by", "x"); code != 2 {
		t.Errorf("issuance option while consuming exit %d", code)
	}
	if treeDigest(t, project) != before {
		t.Fatal("a refused apply changed the project")
	}

	// The authorized apply, then its repetition.
	var applied result
	if code := runJSON(&applied, issue...); code != 0 || applied.Result.Status != "applied" {
		t.Fatalf("apply exit %d %+v", code, applied)
	}
	if ref, err := os.ReadFile(filepath.Join(project, ".git", "refs", "heads", "trunk")); err != nil || string(ref) != applied.Result.Commit+"\n" {
		t.Fatalf("trunk %q %v", ref, err)
	}
	// Reissuing is refused: the plan is regenerated before issuance, and
	// the applied plan is stale by its own effects.
	if code := runJSON(&refused, issue...); code != 1 || refused.Result.Reason != "plan_stale" {
		t.Errorf("reissue exit %d %+v", code, refused)
	}
	var again result
	if code := runJSON(&again, "init", "apply", "--plan", planPath, "--confirm-plan", plan.Digest, "--authorization-id", id); code != 0 ||
		again.Result.Status != "already_applied" {
		t.Fatalf("repeated apply exit %d %+v", code, again)
	}
	if stdout, _, code := run("init", "reconcile", "--plan", planPath, "--authorization-id", id); code != 0 ||
		!strings.Contains(stdout, "unresolved:     false") || !strings.Contains(stdout, "authority:      none") {
		t.Fatalf("reconcile exit %d %q", code, stdout)
	}
	if code := runJSON(&refused, "init", "apply", "--plan", planPath, "--confirm-plan", plan.Digest, "--authorization-id",
		"123e4567-e89b-42d3-a456-426614174110", "--issue-authorization", "--authorized-by", "operator"); code != 1 || refused.Result.Reason != "plan_stale" {
		t.Errorf("new authorization of an applied plan exit %d %+v", code, refused)
	}
	// The assistant refuses without a terminal.
	if _, stderr, code := run("init", "--project-dir", project); code != 2 || !strings.Contains(stderr, "interactive_terminal_required") {
		t.Errorf("assistant without a terminal exit %d %q", code, stderr)
	}

	// No output and no durable record carries the credential.
	if strings.Contains(outputs.String(), "E2E-SECRET") {
		t.Fatal("a command printed the credential")
	}
	// The state root is under HOME on both platforms; the only file there
	// allowed to hold the credential is the user's own configuration.
	_ = filepath.WalkDir(home, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && entry.Type().IsRegular() && path != filepath.Join(home, ".gitconfig") {
			if data, _ := os.ReadFile(path); strings.Contains(string(data), "E2E-SECRET") {
				t.Errorf("credential in %s", path)
			}
		}
		return nil
	})
}
