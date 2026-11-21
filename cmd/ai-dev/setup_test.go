package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The setup commands, agent select and workflow guide start no agent and
// never touch Git (SC-002, F011-FR-008), and apply is reproducible (SC-006).
func TestSetupStartsNothingAndLeavesGitAlone(t *testing.T) {
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git unavailable")
	}
	bin := buildWorkflowNextBinary(t)
	project := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(realGit, append([]string{"-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "commit.gpgSign=false"}, args...)...)
		cmd.Dir = project
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
		return string(output)
	}
	git("init", "--quiet", "--initial-branch=main")
	if err := os.MkdirAll(filepath.Join(project, "specs", "f"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "specs", "f", "spec.md"), []byte("spec\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "specs")
	git("commit", "--quiet", "-m", "seed")
	head := git("rev-parse", "HEAD")

	fakes := t.TempDir()
	log := filepath.Join(t.TempDir(), "invocations.log")
	for _, name := range []string{"claude", "codex", "git"} {
		script := "#!/bin/sh\necho " + name + " \"$@\" >> '" + log + "'\nexit 1\n"
		if err := os.WriteFile(filepath.Join(fakes, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	apply := []string{"setup", "apply", "--agent", "claude-code", "--agent", "codex-cli",
		"--role", "specification=claude-code", "--role", "planning=claude-code", "--role", "implementation=claude-code",
		"--role", "review=claude-code", "--role", "remediation=codex-cli"}
	run := func(args ...string) (string, int) {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Dir = project
		cmd.Env = []string{"HOME=" + t.TempDir(), "PATH=" + fakes}
		cmd.Stdin = strings.NewReader("")
		output, err := cmd.Output()
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return string(output), exit.ExitCode()
		}
		if err != nil {
			t.Fatal(err)
		}
		return string(output), 0
	}

	for _, step := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"setup", "show"}, 0, "ai-dev: outcome=not_configured source=built_in\n"},
		{apply, 0, "ai-dev: outcome=configuration_written source=project\n"},
		{[]string{"agent", "select", "implementation"}, 0, "ai-dev: outcome=role_selected role=implementation agent=claude-code source=project\n"},
		{[]string{"workflow", "guide", "f"}, 0, "source=project"},
		{[]string{"setup", "show", "--format", "json"}, 0, `"outcome":"configured"`},
		{[]string{"setup"}, 2, ""},
	} {
		output, code := run(step.args...)
		if code != step.code || !strings.Contains(output, step.want) {
			t.Fatalf("%v: code=%d output=%q", step.args, code, output)
		}
	}
	first, err := os.ReadFile(filepath.Join(project, ".ai-dev", "agents.json"))
	if err != nil {
		t.Fatal(err)
	}
	if output, code := run(apply...); code != 0 || !strings.Contains(output, "outcome=configuration_unchanged") {
		t.Fatalf("second apply: code=%d output=%q", code, output)
	}
	if output, code := run(append([]string{apply[0], apply[1], "--replace"}, apply[2:]...)...); code != 0 || !strings.Contains(output, "configuration_unchanged") {
		t.Fatalf("replace with the same configuration: code=%d output=%q", code, output)
	}
	second, _ := os.ReadFile(filepath.Join(project, ".ai-dev", "agents.json"))
	if string(first) != string(second) {
		t.Fatal("apply is not byte-identical across runs")
	}

	if data, err := os.ReadFile(log); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an agent or git was started: %q", data)
	}
	if got := git("rev-parse", "HEAD"); got != head {
		t.Fatalf("HEAD moved: %s -> %s", head, got)
	}
	if got := git("status", "--porcelain", "--untracked-files=all"); got != "?? .ai-dev/agents.json\n" {
		t.Fatalf("status: %q", got)
	}
}
