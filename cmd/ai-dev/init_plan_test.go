package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInitPlanSubprocess(t *testing.T) {
	if runtime.GOOS != "darwin" {
		// Plan export needs a production filesystem; Linux temporary
		// directories are tmpfs, which is correctly refused.
		t.Skip("plan export acceptance runs on macOS APFS here")
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available")
	}
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(base, "ai-dev")
	build := exec.Command("go", "build", "-o", bin, "./cmd/ai-dev")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	home, project, out := filepath.Join(base, "home"), filepath.Join(base, "project"), filepath.Join(base, "out")
	for _, dir := range []string{home, project, out} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	env := []string{"HOME=" + home, "PATH=" + filepath.Dir(gitPath) + ":/usr/bin:/bin"}
	run := func(args ...string) (string, string, int) {
		cmd := exec.Command(bin, append([]string{"init", "plan", "--project-dir", project}, args...)...)
		cmd.Dir, cmd.Env = base, env
		return runCommand(cmd)
	}
	before := treeDigest(t, project)

	stdout, stderr, code := run("--strategy", "git-flow", "--initialize-repository", "--git-flow-dialect", "avh",
		"--production-branch", "main", "--development-branch", "develop", "--feature-prefix", "feature/", "--bugfix-prefix", "bugfix/",
		"--release-prefix", "release/", "--hotfix-prefix", "hotfix/", "--support-prefix", "support/", "--version-tag-prefix", "v",
		"--output", filepath.Join(out, "plan.json"), "--format", "json")
	if code != 0 {
		t.Fatalf("plan exit %d: %s %s", code, stdout, stderr)
	}
	var output struct {
		Schema     string `json:"schema_version"`
		PlanDigest string `json:"plan_digest"`
		Operations []struct {
			Kind string   `json:"kind"`
			Argv []string `json:"exact_argv"`
		} `json:"ordered_operations"`
		Report struct {
			Pending   []string `json:"pending_dependencies"`
			Authority bool     `json:"grants_mutation_authority"`
		} `json:"strategy_report"`
	}
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout)
	}
	writes := 0
	for _, operation := range output.Operations {
		if operation.Kind == "git_config_write" {
			writes++
		}
	}
	if output.Schema != "ai-dev.git-plan-output/v1" || output.Operations[0].Kind != "repository_init" || writes != 8 ||
		output.Report.Authority || len(output.Report.Pending) != 1 || output.Report.Pending[0] != "development_branch_pending_initial_history" {
		t.Fatalf("plan output %+v", output)
	}
	argv := strings.Join(output.Operations[0].Argv, " ")
	if !strings.Contains(argv, "init --template= --object-format=sha1 --ref-format=files --initial-branch=main -- "+project) {
		t.Fatalf("init argv %s", argv)
	}
	plan, err := os.ReadFile(filepath.Join(out, "plan.json"))
	info, statErr := os.Stat(filepath.Join(out, "plan.json"))
	if err != nil || statErr != nil || info.Mode().Perm() != 0o600 || !strings.Contains(string(plan), output.PlanDigest) {
		t.Fatalf("plan file %v %v", info, err)
	}
	// The text plan shows the frozen toolchain and the exact
	// initialization before any authorization.
	text, _, code := run("--strategy", "basic", "--initialize-repository", "--initial-branch", "main", "--output", filepath.Join(out, "text.json"))
	if code != 0 || !strings.Contains(text, "toolchain:      git ") ||
		!strings.Contains(text, "initialize:     git init --template= --object-format=sha1 --ref-format=files --initial-branch=main -- "+project+"\n") ||
		!strings.Contains(text, "formats:        object sha1, refs files\n") {
		t.Fatalf("text plan exit %d:\n%s", code, text)
	}
	if treeDigest(t, project) != before {
		t.Fatal("planning changed the project")
	}

	// Exit codes and refusal envelopes.
	for _, test := range []struct {
		args   []string
		code   int
		reason string
	}{
		{[]string{"--strategy", "trunk", "--output", filepath.Join(out, "a.json")}, 2, "invalid_strategy"},
		{[]string{"--strategy", "basic", "--initialize-repository", "--output", filepath.Join(out, "b.json")}, 2, "missing_required_branch"},
		{[]string{"--strategy", "basic", "--initialize-repository", "--initial-branch", "HEAD", "--output", filepath.Join(out, "c.json")}, 2, "invalid_strategy"},
		{[]string{"--strategy", "basic", "--initial-branch", "main", "--output", filepath.Join(out, "d.json")}, 2, "invalid_strategy"},
		{[]string{"--strategy", "trunk-based", "--trunk-branch", "main", "--output", filepath.Join(out, "e.json")}, 1, "repository_absent"},
		{[]string{"--strategy", "basic", "--initialize-repository", "--initial-branch", "main", "--output", filepath.Join(out, "plan.json")}, 2, "output_exists"},
		{[]string{"--strategy", "basic", "--initialize-repository", "--initial-branch", "main", "--output", filepath.Join(project, "plan.json")}, 2, "unsafe_output_path"},
	} {
		stdout, _, code := run(append(test.args, "--format", "json")...)
		var refusal struct {
			Status    string `json:"status"`
			Reason    string `json:"reason"`
			Exit      int    `json:"exit_code"`
			Retryable bool   `json:"retryable"`
		}
		if err := json.Unmarshal([]byte(stdout), &refusal); err != nil || code != test.code || refusal.Status != "refused" ||
			refusal.Reason != test.reason || refusal.Exit != test.code || refusal.Retryable {
			t.Errorf("%v: exit %d %s", test.args, code, stdout)
		}
	}
	for _, args := range [][]string{
		{"--strategy", "basic", "--output"}, {"--strategy", "basic"}, {"--path", project, "--strategy", "basic", "--output", filepath.Join(out, "f.json")},
		{"--strategy", "basic", "--initial-commit", "x", "--output", filepath.Join(out, "g.json")},
	} {
		if _, _, code := run(args...); code != 2 {
			t.Errorf("%v exit %d, want 2", args, code)
		}
	}
	if treeDigest(t, project) != before {
		t.Fatal("refusals changed the project")
	}
	if entries, _ := os.ReadDir(project); len(entries) != 0 {
		t.Fatal("planning created files in the project")
	}
}
