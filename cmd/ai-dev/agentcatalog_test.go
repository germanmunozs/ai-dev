package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// catalogRuns is contracts/catalog.md: each agent's executable and the
// arguments it must receive for the instruction "-x a b".
var catalogRuns = []struct {
	id, executable string
	args           []string
}{
	{"claude-code", "claude", []string{"--print", "--", "-x a b"}},
	{"codex-cli", "codex", []string{"exec", "--", "-x a b"}},
	{"factory-droid", "droid", []string{"exec", "--", "-x a b"}},
	{"gemini-cli", "gemini", []string{"--prompt=-x a b"}},
	{"kiro-cli", "kiro-cli", []string{"chat", "--no-interactive", "--", "-x a b"}},
	{"mistral-vibe", "vibe", []string{"--prompt=-x a b"}},
	{"opencode", "opencode", []string{"run", "--", "-x a b"}},
	{"qwen-code", "qwen", []string{"--prompt=-x a b"}},
}

// fakeAgents writes one script per catalog executable. Each prints its name
// and one ARG line per argument, then exits with $FAKE_EXIT (default 0). No
// real coding agent is ever started (SC-005).
func fakeAgents(t *testing.T, skip ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, run := range catalogRuns {
		if slices.Contains(skip, run.executable) {
			continue
		}
		body := "#!/bin/sh\necho \"FAKE:" + run.executable + "\"\nfor a in \"$@\"; do printf 'ARG:%s\\n' \"$a\"; done\nexit \"${FAKE_EXIT:-0}\"\n"
		if err := os.WriteFile(filepath.Join(dir, run.executable), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runCatalogAgent(t *testing.T, bin, pathDir string, env []string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = t.TempDir()
	// Only the fakes are on PATH: a real agent installed in a system
	// directory can never be found, not even when its fake is removed (SC-005).
	// The fakes need nothing else: /bin/sh by absolute path, builtins only.
	cmd.Env = append([]string{"PATH=" + pathDir, "HOME=" + t.TempDir()}, env...)
	return runCommand(cmd)
}

func wantFakeOutput(executable string, args []string) string {
	var b strings.Builder
	b.WriteString("FAKE:" + executable + "\n")
	for _, a := range args {
		b.WriteString("ARG:" + a + "\n")
	}
	return b.String()
}

// Every catalog agent runs its exact published invocation, with the
// instruction as one argument it cannot read as an option (F013 US2,
// SC-002, SC-003, 002 FR-010 to FR-019).
func TestAgentRunStartsEachCatalogAgentsExactInvocation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake executables are POSIX shell scripts")
	}
	bin := buildWorkflowNextBinary(t)
	fakes := fakeAgents(t)

	for _, run := range catalogRuns {
		t.Run(run.id, func(t *testing.T) {
			stdout, stderr, code := runCatalogAgent(t, bin, fakes, nil, "agent", "run", run.id, "-x a b")
			if code != 0 || stdout != wantFakeOutput(run.executable, run.args) {
				t.Fatalf("code %d\nstdout %q\nwant   %q\nstderr %q", code, stdout, wantFakeOutput(run.executable, run.args), stderr)
			}
			if !strings.Contains(stderr, "ai-dev: outcome=agent_succeeded agent="+run.id) {
				t.Fatalf("stderr %q lacks the success outcome", stderr)
			}
		})
	}

	t.Run("an instruction equal to --help is still the prompt", func(t *testing.T) {
		stdout, stderr, code := runCatalogAgent(t, bin, fakes, nil, "agent", "run", "opencode", "--help")
		if want := wantFakeOutput("opencode", []string{"run", "--", "--help"}); code != 0 || stdout != want {
			t.Fatalf("code %d stdout %q want %q stderr %q", code, stdout, want, stderr)
		}
	})

	t.Run("a non-zero exit is the agent's own failure", func(t *testing.T) {
		_, stderr, code := runCatalogAgent(t, bin, fakes, []string{"FAKE_EXIT=3"}, "agent", "run", "kiro-cli", "x")
		if code != 1 || !strings.Contains(stderr, "outcome=agent_failed agent=kiro-cli") {
			t.Fatalf("code %d stderr %q", code, stderr)
		}
	})

	t.Run("an agent missing from PATH is an execution failure", func(t *testing.T) {
		stdout, stderr, code := runCatalogAgent(t, bin, fakeAgents(t, "qwen"), nil, "agent", "run", "qwen-code", "x")
		if code != 1 || stdout != "" || !strings.Contains(stderr, "outcome=execution_failed agent=qwen-code") {
			t.Fatalf("code %d stdout %q stderr %q", code, stdout, stderr)
		}
	})

	t.Run("JSON keeps the report alone on standard output", func(t *testing.T) {
		stdout, stderr, code := runCatalogAgent(t, bin, fakes, nil, "agent", "run", "--format", "json", "gemini-cli", "-x a b")
		var report struct {
			Agent   string `json:"agent_id"`
			Outcome string `json:"outcome"`
		}
		if code != 0 || json.Unmarshal([]byte(stdout), &report) != nil || report.Outcome != "agent_succeeded" {
			t.Fatalf("code %d stdout %q stderr %q", code, stdout, stderr)
		}
		if !strings.Contains(stderr, wantFakeOutput("gemini", []string{"--prompt=-x a b"})) {
			t.Fatalf("agent output did not stream to stderr: %q", stderr)
		}
	})
}

// New catalog agents can be chosen and assigned to roles; without a project
// configuration the built-in assignment is unchanged; a configuration
// written before Feature 013 stays valid (F013-FR-010, FR-011, US3).
func TestNewCatalogAgentsCanBeConfiguredForRoles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake executables are POSIX shell scripts")
	}
	bin := buildWorkflowNextBinary(t)
	fakes := fakeAgents(t)
	run := func(project string, args ...string) (string, int) {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Dir = project
		cmd.Env = []string{"PATH=" + fakes, "HOME=" + t.TempDir()}
		stdout, _, code := runCommand(cmd)
		return stdout, code
	}

	empty := t.TempDir()
	for role, agentID := range map[string]string{"specification": "claude-code", "implementation": "codex-cli", "remediation": "codex-cli"} {
		if out, code := run(empty, "agent", "select", role); code != 0 || !strings.Contains(out, "agent="+agentID+" source=built_in") {
			t.Fatalf("built-in %s: code %d out %q", role, code, out)
		}
	}
	show, code := run(empty, "setup", "show")
	for _, r := range catalogRuns {
		if !strings.Contains(show, "agent "+r.id+" availability=available") {
			t.Fatalf("setup show lacks %s (code %d):\n%s", r.id, code, show)
		}
	}

	project := t.TempDir()
	out, code := run(project, "setup", "apply", "--agent", "gemini-cli", "--agent", "factory-droid",
		"--role", "specification=gemini-cli", "--role", "planning=gemini-cli", "--role", "implementation=factory-droid",
		"--role", "review=gemini-cli", "--role", "remediation=factory-droid")
	if code != 0 || !strings.HasPrefix(out, "ai-dev: outcome=configuration_written source=project\n") {
		t.Fatalf("setup apply: code %d out %q", code, out)
	}
	if out, code := run(project, "agent", "select", "implementation"); code != 0 || !strings.Contains(out, "agent=factory-droid source=project") {
		t.Fatalf("agent select implementation: code %d out %q", code, out)
	}

	legacy := t.TempDir()
	config := `{"schema_version":"ai-dev.agents/v1","agents":["claude-code","codex-cli"],"roles":{"specification":"claude-code","planning":"claude-code","implementation":"codex-cli","review":"claude-code","remediation":"codex-cli"}}` + "\n"
	if err := os.MkdirAll(filepath.Join(legacy, ".ai-dev"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(legacy, ".ai-dev", "agents.json")
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, code := run(legacy, "setup", "show"); code != 0 || !strings.HasPrefix(out, "ai-dev: outcome=configured source=project\n") {
		t.Fatalf("legacy configuration: code %d out %q", code, out)
	}
	if after, err := os.ReadFile(path); err != nil || string(after) != config {
		t.Fatalf("legacy configuration changed: %q %v", after, err)
	}
}
