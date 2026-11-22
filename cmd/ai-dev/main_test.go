package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCommandContractSubprocess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake executables are POSIX shell scripts")
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "ai-dev")
	build := exec.Command("go", "build", "-o", bin, "./cmd/ai-dev")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build command: %v\n%s", err, output)
	}

	fixture := makeReadyFixture(t)
	marker := filepath.Join(fixture, "executed")
	pathDir := makeFakePATH(t, marker)

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
		usage      bool
	}{
		{"default text", []string{"doctor"}, 0, "Overall: Ready", "", false},
		{"explicit text", []string{"doctor", "--format", "text"}, 0, "Overall: Ready", "", false},
		{"json", []string{"doctor", "--format=json"}, 0, `"overall":"ready"`, "", false},
		{"agents text", []string{"agents"}, 0, "claude-code", "", false},
		{"agents json", []string{"agents", "--format=json"}, 0, `"agents"`, "", false},
		{"agent run text", []string{"agent", "run", "claude-code", "hello"}, 0, "", "outcome=agent_succeeded agent=claude-code", false},
		{"agent run json", []string{"agent", "run", "--format=json", "codex-cli", "hello"}, 0, `"agent_id":"codex-cli"`, "", false},
		{"invalid flag", []string{"doctor", "--bogus"}, 2, "", "Usage:", true},
		{"agents invalid flag", []string{"agents", "--bogus"}, 2, "", "Usage:", true},
		{"invalid format", []string{"doctor", "--format", "yaml"}, 2, "", "Usage:", true},
		{"positional", []string{"doctor", "extra"}, 2, "", "Usage:", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			cmd := exec.Command(bin, tt.args...)
			cmd.Dir = fixture
			cmd.Env = append(os.Environ(), "PATH="+pathDir, "AI_DEV_TEST_MARKER="+marker)
			stdout, stderr, code := runCommand(cmd)
			if code != tt.wantCode || !strings.Contains(stdout, tt.wantStdout) || !strings.Contains(stderr, tt.wantStderr) {
				t.Fatalf("code/stdout/stderr = %d/%q/%q", code, stdout, stderr)
			}
			if tt.usage {
				if stdout != "" {
					t.Fatalf("usage wrote stdout: %q", stdout)
				}
				if _, err := os.Stat(marker); !os.IsNotExist(err) {
					t.Fatalf("usage error executed a check; marker err=%v", err)
				}
			}
			if tt.name == "json" {
				var decoded struct {
					Overall string `json:"overall"`
					Checks  []any  `json:"checks"`
				}
				if err := json.Unmarshal([]byte(stdout), &decoded); err != nil || decoded.Overall != "ready" || len(decoded.Checks) != 9 {
					t.Fatalf("JSON contract: %#v, %v", decoded, err)
				}
			}
			if tt.name == "agents json" {
				var decoded struct {
					Agents []struct {
						ID           string `json:"id"`
						Availability string `json:"availability"`
					} `json:"agents"`
				}
				if err := json.Unmarshal([]byte(stdout), &decoded); err != nil || len(decoded.Agents) != 2 || decoded.Agents[0].ID != "claude-code" || decoded.Agents[1].ID != "codex-cli" {
					t.Fatalf("agent JSON contract: %#v, %v", decoded, err)
				}
			}
		})
	}

	if err := os.Remove(filepath.Join(fixture, ".specify", "memory", "constitution.md")); err != nil {
		t.Fatal(err)
	}
	for _, formatArgs := range [][]string{nil, {"--format", "json"}} {
		cmd := exec.Command(bin, append([]string{"doctor"}, formatArgs...)...)
		cmd.Dir = fixture
		cmd.Env = append(os.Environ(), "PATH="+pathDir, "AI_DEV_TEST_MARKER="+marker)
		_, _, code := runCommand(cmd)
		if code != 1 {
			t.Fatalf("not-ready args %v exit = %d, want 1", formatArgs, code)
		}
	}
}

func TestCommandHandledInterruptStopsChildBeforeExitAndReportsExecutionFailed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake executables and interrupt assertions are POSIX-specific")
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "ai-dev")
	build := exec.Command("go", "build", "-o", bin, "./cmd/ai-dev")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build command: %v\n%s", err, output)
	}

	temp := t.TempDir()
	pidFile := filepath.Join(temp, "child.pid")
	releaseFile := filepath.Join(temp, "never-release")
	pathDir := t.TempDir()
	fixture, err := filepath.Abs(filepath.Join(repoRoot, "internal", "system", "testdata", "streaming-agent.sh"))
	if err != nil {
		t.Fatal(err)
	}
	claude := filepath.Join(pathDir, "claude")
	launcher := "#!/bin/sh\nexec \"" + fixture + "\" \"$AI_DEV_RELEASE_FILE\" \"$AI_DEV_PID_FILE\" 0\n"
	if err := os.WriteFile(claude, []byte(launcher), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, "agent", "run", "claude-code", "wait for cancellation")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"PATH="+pathDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"AI_DEV_RELEASE_FILE="+releaseFile,
		"AI_DEV_PID_FILE="+pidFile,
	)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start ai-dev: %v", err)
	}
	pid := waitForFixturePID(t, pidFile)
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("interrupt ai-dev: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		exitErr, ok := err.(*exec.ExitError)
		if !ok || exitErr.ExitCode() != 1 {
			t.Fatalf("ai-dev wait error = %v, want exit 1", err)
		}
	case <-time.After(7 * time.Second):
		t.Fatal("ai-dev did not exit after handled interrupt")
	}
	if err := syscall.Kill(pid, 0); err == nil || !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("child pid %d still exists after ai-dev exit; signal check error = %v", pid, err)
	}
	if stdout.String() != "stdout-before-exit\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if got := stderr.String(); !strings.Contains(got, "stderr-before-exit\n") ||
		!strings.Contains(got, "outcome=execution_failed agent=claude-code") ||
		strings.Contains(got, "outcome=agent_succeeded") {
		t.Fatalf("stderr = %q", got)
	}
}

func waitForFixturePID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		contents, err := os.ReadFile(path)
		if err == nil {
			pid, err := strconv.Atoi(strings.TrimSpace(string(contents)))
			if err != nil {
				t.Fatalf("parse fixture pid: %v", err)
			}
			return pid
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("fixture did not write pid file %s", path)
	return 0
}

func runCommand(cmd *exec.Cmd) (stdout, stderr string, code int) {
	var outBuilder, errBuilder strings.Builder
	cmd.Stdout = &outBuilder
	cmd.Stderr = &errBuilder
	err := cmd.Run()
	code = 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			code = -1
		}
	}
	return outBuilder.String(), errBuilder.String(), code
}

func makeReadyFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		".specify/integrations/speckit.manifest.json": `{"files":{".specify/template.md":"ignored"}}`,
		".specify/template.md":                        "template",
		".specify/memory/constitution.md":             "**Version**: 1.0.0 | **Ratified**: 2026-09-27",
		".specify/integration.json":                   `{"installed_integrations":["claude","codex"]}`,
		".specify/integrations/claude.manifest.json":  `{"files":{".claude/skill.md":"ignored"}}`,
		".claude/skill.md":                            "claude",
		".specify/integrations/codex.manifest.json":   `{"files":{".agents/skill.md":"ignored"}}`,
		".agents/skill.md":                            "codex",
	}
	for path, contents := range files {
		fullPath := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func makeFakePATH(t *testing.T, marker string) string {
	t.Helper()
	dir := t.TempDir()
	for _, tool := range []string{"git", "specify", "claude", "codex"} {
		body := "#!/bin/sh\n"
		if tool == "git" {
			body += "printf marker > \"$AI_DEV_TEST_MARKER\"\nprintf 'true\\n'\n"
		}
		path := filepath.Join(dir, tool)
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	_ = marker
	return dir
}
