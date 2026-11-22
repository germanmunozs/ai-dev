package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/gitstore"
)

// productionTempDir returns a fresh temporary directory, skipping the test
// unless it is on a production filesystem: APFS on macOS, local ext4 on
// Linux. Linux temporary directories are usually tmpfs, which plan export
// and apply correctly refuse.
func productionTempDir(t *testing.T) string {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if obs, err := gitstore.ObserveFilesystem(base); err != nil || !obs.Local ||
		obs.FilesystemType != map[string]string{"darwin": "apfs", "linux": "ext4"}[runtime.GOOS] {
		t.Skip("the temporary directory is not on a production filesystem")
	}
	return base
}

func TestInitPlanSubprocess(t *testing.T) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available")
	}
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	base := productionTempDir(t)
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

	// The initial-commit group: explicit message, identities, dates and
	// literal includes; the text plan shows every exact approval argument.
	commitProject := filepath.Join(base, "commit-project")
	if err := os.MkdirAll(commitProject, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"README.md": "# p\n", ".gitignore": "*.log\n", "trace.log": "x\n",
		"key.pem": "-----BEGIN " + "EC PRIVATE KEY-----\nAAAA\n"} {
		if err := os.WriteFile(filepath.Join(commitProject, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	message := filepath.Join(out, "message.txt")
	if err := os.WriteFile(message, []byte("chore: initial commit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitBefore := treeDigest(t, commitProject)
	commit := func(extra ...string) []string {
		return append([]string{"--project-dir", commitProject, "--strategy", "basic", "--initialize-repository", "--initial-branch", "main",
			"--initial-commit", "--initial-commit-message-file", message, "--author-name", "Ada", "--author-email", "ada@example.org",
			"--committer-name", "Ada", "--committer-email", "ada@example.org", "--author-date", "2026-11-18T10:30:00+02:00",
			"--committer-date", "2026-11-18T10:30:00+02:00"}, extra...)
	}
	text, stderr, code = run(commit("--include", "README.md", "--include", ".gitignore", "--include", "trace.log", "--output", filepath.Join(out, "commit.json"))...)
	if code != 0 || !strings.Contains(text, "commit:         ") || !strings.Contains(text, "initial ref:    refs/heads/main") ||
		!strings.Contains(text, "author:         Ada <ada@example.org> 2026-11-18T10:30:00+02:00\n") ||
		!strings.Contains(text, "message:        | chore: initial commit\n") ||
		!strings.Contains(text, "approve:        --approve-file 'README.md|100644|sha256:") ||
		!strings.Contains(text, "approve:        --approve-file '.ai-dev/config.json|100644|sha256:") ||
		!strings.Contains(text, "approve:        --approve-ignored-file 'trace.log|100644|sha256:") ||
		!strings.Contains(text, "screening:      secret screening detects known high-risk patterns only") {
		t.Fatalf("commit plan exit %d:\n%s%s", code, text, stderr)
	}
	stdout, _, code = run(commit("--include", "README.md", "--include", "key.pem", "--output", filepath.Join(out, "secret.json"), "--format", "json")...)
	var refusal struct {
		Reason   string `json:"reason"`
		Findings []struct {
			Category string `json:"category"`
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(stdout), &refusal); err != nil || code != 1 || refusal.Reason != "sensitive_content_suspected" ||
		len(refusal.Findings) != 1 || refusal.Findings[0].Category != "private_key" || strings.Contains(stdout, "AAAA") {
		t.Fatalf("secret refusal exit %d: %s", code, stdout)
	}
	if _, _, code := run(append(commit("--output", filepath.Join(out, "anon.json")), "--author-name", "")...); code != 1 {
		t.Errorf("empty author name exit %d, want 1", code)
	}
	for _, args := range [][]string{
		{"--project-dir", commitProject, "--strategy", "basic", "--initialize-repository", "--initial-branch", "main", "--include", "README.md", "--output", filepath.Join(out, "h.json")},
		{"--project-dir", commitProject, "--strategy", "basic", "--initialize-repository", "--initial-branch", "main", "--initial-commit", "--output", filepath.Join(out, "i.json")},
		append(commit("--output", filepath.Join(out, "j.json")), "--author-date", "yesterday"),
	} {
		if _, _, code := run(args...); code != 2 {
			t.Errorf("%v exit %d, want 2", args, code)
		}
	}
	if treeDigest(t, commitProject) != commitBefore {
		t.Fatal("commit planning changed the project")
	}
}
