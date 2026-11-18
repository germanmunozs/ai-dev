package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The public apply option matrix and its refusals before any record: usage
// errors, issuance-only options without --issue-authorization, an unknown
// option and an unreadable plan create no state. The enabled flows are in
// gitmanagement_e2e_test.go (T109).
func TestInitApplySubprocess(t *testing.T) {
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
	home, state := filepath.Join(base, "home"), filepath.Join(base, "state")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	env := []string{"HOME=" + home, "XDG_STATE_HOME=" + state, "PATH=/usr/bin:/bin"}
	run := func(args ...string) (string, string, int) {
		cmd := exec.Command(bin, args...)
		cmd.Dir, cmd.Env, cmd.Stdin = base, env, strings.NewReader("")
		return runCommand(cmd)
	}
	const id = "123e4567-e89b-42d3-a456-426614174099"
	for _, args := range [][]string{
		{"init", "apply"},
		{"init", "apply", "--plan", "p.json", "--confirm-plan", "sha256:x"},
		{"init", "apply", "--plan", "p.json", "--confirm-plan", "sha256:x", "--authorization-id", id, "--authorized-by", "op"},
		{"init", "apply", "--plan", "p.json", "--confirm-plan", "sha256:x", "--authorization-id", id, "--approve-ignored-file", "x"},
		{"init", "apply", "--plan", "p.json", "--confirm-plan", "sha256:x", "--authorization-id", id, "--path", "x"},
		{"init", "apply", "--plan", "p.json", "--confirm-plan", "sha256:x", "--authorization-id", id, "--issue-authorization", "--authorized-by", "op"},
		{"init", "apply", "--plan", "p.json", "--confirm-plan", "sha256:x", "--authorization-id", id},
		{"init", "apply", "--plan", "p.json", "--confirm-plan", "sha256:x", "--authorization-id", id, "--reconciles-authorization", id},
		{"init", "reconcile"}, {"init", "reconcile", "--plan", "p.json"}, {"init", "reconcile", "--plan", "p.json", "--authorization-id", id},
	} {
		if _, _, code := run(args...); code != 2 {
			t.Errorf("%v exit %d, want 2", args[1:], code)
		}
	}
	_, stderr, code := run("init", "apply", "--plan", "p.json", "--confirm-plan", "sha256:x", "--authorization-id", id)
	if code != 2 || !strings.Contains(stderr, "the plan cannot be read") {
		t.Fatalf("apply of a missing plan exit %d %q", code, stderr)
	}
	_, stderr, code = run("init")
	if code != 2 || !strings.Contains(stderr, "interactive_terminal_required") {
		t.Fatalf("assistant without a terminal exit %d %q", code, stderr)
	}
	for _, dir := range []string{state, filepath.Join(home, "Library")} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("a refused command created state at %s", dir)
		}
	}
}
