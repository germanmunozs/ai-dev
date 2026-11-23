package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

// setupExecutor finds the listed executables and records every call; it
// must only ever be asked to look executables up (F011-FR-001, FR-013).
type setupExecutor struct {
	paths   map[string]string
	lookups []string
	runs    int
}

func (e *setupExecutor) LookPath(name string) (string, error) {
	e.lookups = append(e.lookups, name)
	if path := e.paths[name]; path != "" {
		return path, nil
	}
	return "", errors.New("not found")
}

func (e *setupExecutor) Run(context.Context, string, ...string) (string, error) {
	e.runs++
	return "", errors.New("setup must not run a process")
}

func onlyClaude() *setupExecutor {
	return &setupExecutor{paths: map[string]string{"claude": "/secret/path/claude"}}
}

func runShow(t *testing.T, root fstest.MapFS, executor *setupExecutor, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := RunSetupShow(args, executor, root, &stdout, &stderr)
	if strings.Contains(stdout.String(), "/secret/path") {
		t.Fatalf("an executable path was reported: %q", stdout.String())
	}
	return code, stdout.String(), stderr.String()
}

const projectConfigImplementationClaude = `{"schema_version":"ai-dev.agents/v1","agents":["claude-code","codex-cli"],
"roles":{"specification":"claude-code","planning":"claude-code","implementation":"claude-code","review":"claude-code","remediation":"codex-cli"}}`

// setup show reports each outcome with its exit code and only looks
// executables up, exactly claude then codex (F011-FR-009, FR-002).
func TestRunSetupShowOutcomes(t *testing.T) {
	for _, test := range []struct {
		name  string
		root  fstest.MapFS
		code  int
		first string
		lines []string
	}{
		{"not configured", fstest.MapFS{}, 0, "ai-dev: outcome=not_configured source=built_in",
			[]string{"  agent claude-code availability=available chosen=no", "  role implementation agent=codex-cli"}},
		{"configured", configFS(projectConfigImplementationClaude), 0, "ai-dev: outcome=configured source=project",
			[]string{"  agent codex-cli availability=unavailable chosen=yes warning=chosen_agent_unavailable", "  role implementation agent=claude-code"}},
		{"invalid", configFS(`{"schema_version":"ai-dev.agents/v1","agents":["gemini"],"roles":{}}`), 1, "ai-dev: outcome=configuration_invalid",
			[]string{`  reason unknown_agent "gemini"`, `  reason missing_role "remediation"`}},
	} {
		t.Run(test.name, func(t *testing.T) {
			executor := onlyClaude()
			code, stdout, stderr := runShow(t, test.root, executor)
			if code != test.code || stderr != "" || !strings.HasPrefix(stdout, test.first+"\n") {
				t.Fatalf("code=%d stderr=%q stdout=%q", code, stderr, stdout)
			}
			for _, line := range test.lines {
				if !strings.Contains(stdout, line+"\n") {
					t.Errorf("missing %q in %q", line, stdout)
				}
			}
			if !reflect.DeepEqual(executor.lookups, catalogExecutables) || executor.runs != 0 {
				t.Fatalf("lookups=%v runs=%d", executor.lookups, executor.runs)
			}
		})
	}
}

func TestRunSetupShowJSON(t *testing.T) {
	code, stdout, stderr := runShow(t, configFS(projectConfigImplementationClaude), onlyClaude(), "--format", "json")
	var object map[string]any
	if code != 0 || stderr != "" || strings.Count(stdout, "\n") != 1 || json.Unmarshal([]byte(stdout), &object) != nil {
		t.Fatalf("code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	if object["schema_version"] != "ai-dev.setup/v1" || object["outcome"] != "configured" || len(object["warnings"].([]any)) != 1 {
		t.Fatalf("object: %v", object)
	}
}

func TestRunSetupShowUsage(t *testing.T) {
	for _, args := range [][]string{{"extra"}, {"--nope"}, {"--format", "yaml"}} {
		executor := onlyClaude()
		code, stdout, stderr := runShow(t, fstest.MapFS{}, executor, args...)
		if code != 2 || stdout != "" || stderr != setupShowUsage+"\n" || len(executor.lookups) != 0 {
			t.Errorf("%v: code=%d stdout=%q stderr=%q lookups=%v", args, code, stdout, stderr, executor.lookups)
		}
	}
}

// The router sends setup show and setup apply to their commands and an
// unknown subcommand to the setup group usage.
func TestSetupRouting(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunWithDependencies(context.Background(), []string{"setup", "show"}, Dependencies{Executor: onlyClaude(), Root: fstest.MapFS{}, Stdout: &stdout, Stderr: &stderr})
	if code != 0 || !strings.HasPrefix(stdout.String(), "ai-dev: outcome=not_configured") {
		t.Fatalf("setup show: %d %q %q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = RunWithDependencies(context.Background(), []string{"setup", "bogus"}, Dependencies{Executor: onlyClaude(), Root: fstest.MapFS{}, Stdout: &stdout, Stderr: &stderr})
	if code != 2 || stdout.Len() != 0 || stderr.String() != setupGroupUsage+"\n" {
		t.Fatalf("setup bogus: %d %q %q", code, stdout.String(), stderr.String())
	}
}

var applyArgs = []string{"--agent", "claude-code", "--agent", "codex-cli",
	"--role", "specification=claude-code", "--role", "planning=claude-code", "--role", "implementation=claude-code",
	"--role", "review=claude-code", "--role", "remediation=codex-cli"}

func runApply(t *testing.T, root fstest.MapFS, store *fakeConfigStore, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := RunSetupApply(args, onlyClaude(), root, store, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// setup apply writes, recognizes an equal configuration, and needs
// --replace for a different or invalid one (F011-FR-011).
func TestRunSetupApplyOutcomes(t *testing.T) {
	store := &fakeConfigStore{}
	code, stdout, stderr := runApply(t, fstest.MapFS{}, store, applyArgs...)
	if code != 0 || len(store.writes) != 1 || !strings.HasPrefix(stdout, "ai-dev: outcome=configuration_written source=project\n") ||
		!strings.Contains(stdout, "agent codex-cli availability=unavailable chosen=yes warning=chosen_agent_unavailable") {
		t.Fatalf("written: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	written := configFS(string(store.writes[0]))

	code, stdout, _ = runApply(t, written, store, applyArgs...)
	if code != 0 || len(store.writes) != 1 || !strings.HasPrefix(stdout, "ai-dev: outcome=configuration_unchanged source=project\n") {
		t.Fatalf("unchanged: code=%d stdout=%q", code, stdout)
	}

	different := append(append([]string{}, applyArgs[:len(applyArgs)-1]...), "remediation=claude-code")
	wantExists := "ai-dev: outcome=configuration_exists\n  agent claude-code availability=available chosen=no\n"
	for _, id := range catalogIDs[1:] {
		wantExists += "  agent " + id + " availability=unavailable chosen=no\n"
	}
	for name, root := range map[string]fstest.MapFS{"different": written, "invalid": configFS("{}")} {
		store := &fakeConfigStore{}
		code, stdout, _ := runApply(t, root, store, different...)
		if code != 1 || len(store.writes) != 0 || stdout != wantExists {
			t.Errorf("%s without --replace: code=%d stdout=%q", name, code, stdout)
		}
		code, stdout, _ = runApply(t, root, store, append([]string{"--replace"}, different...)...)
		if code != 0 || len(store.writes) != 1 || !strings.HasPrefix(stdout, "ai-dev: outcome=configuration_written") {
			t.Errorf("%s with --replace: code=%d stdout=%q", name, code, stdout)
		}
	}

	code, stdout, stderr = runApply(t, fstest.MapFS{}, &fakeConfigStore{fail: true}, applyArgs...)
	if code != 1 || !strings.HasPrefix(stdout, "ai-dev: outcome=persistence_failed\n") || !strings.Contains(stderr, "injected write failure") {
		t.Fatalf("persistence failed: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

// configuration_rejected lists every reason and writes nothing.
func TestRunSetupApplyRejected(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want []string
	}{
		{"missing role", applyArgs[:len(applyArgs)-2], []string{`reason missing_role "remediation"`}},
		{"duplicate role", append(append([]string{}, applyArgs...), "--role", "review=codex-cli"), []string{`reason duplicate_role "review"`}},
		{"agent not chosen", []string{"--agent", "claude-code", "--role", "specification=claude-code", "--role", "planning=claude-code", "--role", "implementation=codex-cli", "--role", "review=claude-code", "--role", "remediation=claude-code"}, []string{`reason agent_not_chosen "implementation=codex-cli"`}},
		{"unknown agent and role", []string{"--agent", "gemini", "--role", "deploy=gemini"}, []string{`reason unknown_agent "gemini"`, `reason unknown_role "deploy"`, `reason missing_role "specification"`}},
		{"no agents", applyArgs[4:], []string{"reason no_agents"}},
		{"duplicate agent", append([]string{"--agent", "codex-cli"}, applyArgs...), []string{`reason duplicate_agent "codex-cli"`}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeConfigStore{}
			code, stdout, _ := runApply(t, fstest.MapFS{}, store, test.args...)
			if code != 1 || len(store.writes) != 0 || !strings.HasPrefix(stdout, "ai-dev: outcome=configuration_rejected\n") {
				t.Fatalf("code=%d stdout=%q", code, stdout)
			}
			for _, want := range test.want {
				if !strings.Contains(stdout, "  "+want+"\n") {
					t.Errorf("missing %q in %q", want, stdout)
				}
			}
		})
	}
}

func TestRunSetupApplyUsageAndJSON(t *testing.T) {
	for _, args := range [][]string{{"--role", "implementation"}, append([]string{"extra"}, applyArgs...), append([]string{"--format", "yaml"}, applyArgs...)} {
		store := &fakeConfigStore{}
		code, stdout, stderr := runApply(t, fstest.MapFS{}, store, args...)
		if code != 2 || stdout != "" || stderr != setupApplyUsage+"\n" || len(store.writes) != 0 {
			t.Errorf("%v: code=%d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
	}
	code, stdout, _ := runApply(t, fstest.MapFS{}, &fakeConfigStore{}, append([]string{"--format", "json"}, applyArgs...)...)
	var object map[string]any
	if code != 0 || json.Unmarshal([]byte(stdout), &object) != nil || object["command"] != "apply" || object["outcome"] != "configuration_written" {
		t.Fatalf("json: code=%d stdout=%q", code, stdout)
	}
}

// Feature 008's .ai-dev/config.json is not the 011 configuration: a project
// initialized with ai-dev init is simply not configured for agents.
func TestSetupIgnoresFeature008Config(t *testing.T) {
	root := fstest.MapFS{".ai-dev/config.json": &fstest.MapFile{Data: []byte(`{"schema_version":"ai-dev.project-config/v1","project_id":"x","strategy":"basic","strategy_parameters":{}}`)}}
	code, stdout, _ := runShow(t, root, onlyClaude())
	if code != 0 || !strings.HasPrefix(stdout, "ai-dev: outcome=not_configured source=built_in\n") {
		t.Fatalf("code=%d stdout=%q", code, stdout)
	}
}
