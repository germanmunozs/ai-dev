package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// T099/AC-016/AC-022: the validator runs no Git and touches no repository,
// strategy or state; validity exits 0, invalidity 1 and usage 2, in text
// and JSON.
func TestGitCommitValidateSubprocess(t *testing.T) {
	bin := buildWorkflowNextBinary(t)
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	work, home := filepath.Join(base, "work"), filepath.Join(base, "home")
	for _, dir := range []string{work, home} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	message := filepath.Join(base, "message.txt")
	if err := os.WriteFile(message, []byte("fix(core)!: stop\n\nRefs #123\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(base, "git-ran")
	env := []string{"HOME=" + home, "PATH=" + makeFakePATH(t, marker), "AI_DEV_TEST_MARKER=" + marker}
	run := func(args ...string) (string, string, int) {
		cmd := exec.Command(bin, append([]string{"git", "commit", "validate"}, args...)...)
		cmd.Dir, cmd.Env = work, env
		return runCommand(cmd)
	}
	beforeWork, beforeHome := treeDigest(t, work), treeDigest(t, home)

	stdout, _, code := run("--message", "feat: add a parser")
	if code != 0 || !strings.HasPrefix(stdout, "conventional commit: valid\n") || !strings.Contains(stdout, "breaking:       false\n") {
		t.Fatalf("valid text exit %d: %q", code, stdout)
	}
	stdout, _, code = run("--file", message, "--format", "json")
	var out struct {
		Schema   string   `json:"schema_version"`
		Valid    bool     `json:"valid"`
		Type     string   `json:"type"`
		Breaking bool     `json:"breaking_change_recognized"`
		Body     []string `json:"body_lines"`
		Footers  []struct {
			Token string `json:"token"`
		} `json:"footers"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil || code != 0 || out.Schema != "ai-dev.conventional-commit-validation/v1" ||
		!out.Valid || out.Type != "fix" || !out.Breaking || len(out.Body) != 0 || len(out.Footers) != 1 || out.Footers[0].Token != "Refs" ||
		strings.Count(stdout, "\n") != 1 {
		t.Fatalf("valid json exit %d: %q", code, stdout)
	}
	stdout, _, code = run("--message", "feat: x\n\nbreaking-change: not breaking", "--format", "json")
	if code != 0 || !strings.Contains(stdout, `"breaking_change_recognized":false`) {
		t.Fatalf("lowercase breaking-change exit %d: %q", code, stdout)
	}
	stdout, _, code = run("--message", "not a conventional commit")
	if code != 1 || !strings.HasPrefix(stdout, "conventional commit: invalid\n") || !strings.Contains(stdout, "diagnostic:     ") {
		t.Fatalf("invalid text exit %d: %q", code, stdout)
	}
	stdout, _, code = run("--message", "feat: x\r\n", "--format", "json")
	if code != 1 || !strings.Contains(stdout, `"valid":false`) {
		t.Fatalf("invalid json exit %d: %q", code, stdout)
	}
	for _, args := range [][]string{
		{}, {"--message", "feat: x", "--file", message}, {"--file", filepath.Join(base, "missing")}, {"--file", base},
		{"--message", "feat: x", "extra"}, {"--message", "feat: x", "--format", "yaml"}, {"--unknown"},
	} {
		if stdout, _, code := run(args...); code != 2 || stdout != "" {
			t.Errorf("%v: exit %d stdout %q, want usage exit 2", args, code, stdout)
		}
	}
	if _, err := os.Lstat(marker); !os.IsNotExist(err) {
		t.Fatal("the validator ran Git")
	}
	if treeDigest(t, work) != beforeWork || treeDigest(t, home) != beforeHome {
		t.Fatal("the validator wrote files")
	}
}
