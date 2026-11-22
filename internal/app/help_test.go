package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/germanmunozs/ai-dev/internal/system"
)

func runTopLevel(t *testing.T, args ...string) (int, string, string, *fakeExecutor, *appRunLauncher) {
	t.Helper()
	executor := &fakeExecutor{}
	launcher := &appRunLauncher{}
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), args, executor, launcher, fstest.MapFS{}, bytes.NewReader(nil), &stdout, &stderr)
	return code, stdout.String(), stderr.String(), executor, launcher
}

func TestVersionCommand(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}} {
		code, stdout, stderr, executor, launcher := runTopLevel(t, args...)
		if code != 0 || stderr != "" || !strings.HasPrefix(stdout, "ai-dev ") || strings.Count(stdout, "\n") != 1 {
			t.Fatalf("%v: exit=%d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
		if executor.lookups != 0 || executor.runs != 0 || launcher.launches != 0 {
			t.Fatalf("%v: version reached a process boundary", args)
		}
	}

	code, stdout, stderr, _, _ := runTopLevel(t, "version", "--format", "json")
	var wire map[string]any
	if code != 0 || stderr != "" || json.Unmarshal([]byte(stdout), &wire) != nil || wire["schema_version"] != "ai-dev.version/v1" || wire["version"] == "" || wire["platform"] == "" {
		t.Fatalf("json: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}

	for _, args := range [][]string{{"version", "extra"}, {"version", "--format", "yaml"}, {"version", "--unknown"}, {"--version", "x"}} {
		code, stdout, stderr, _, _ := runTopLevel(t, args...)
		if code != 2 || stdout != "" || stderr != versionUsage+"\n" {
			t.Fatalf("%v: exit=%d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
	}
}

func TestHelpCommand(t *testing.T) {
	_, want, _, _, _ := runTopLevel(t, "help")
	for _, args := range [][]string{{"help"}, {"--help"}, {"-h"}} {
		code, stdout, stderr, executor, launcher := runTopLevel(t, args...)
		if code != 0 || stderr != "" || stdout != want || !strings.HasPrefix(stdout, "ai-dev: ") {
			t.Fatalf("%v: exit=%d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
		if executor.lookups != 0 || executor.runs != 0 || launcher.launches != 0 {
			t.Fatalf("%v: help reached a process boundary", args)
		}
	}
	for _, group := range []string{"doctor", "agents", "agent", "workflow", "init", "git", "version", "help"} {
		if !strings.Contains(want, "\n  "+group+" ") && !strings.Contains(want, "\n  "+group+"\n") {
			t.Fatalf("help does not list the %q group:\n%s", group, want)
		}
	}
	code, stdout, stderr, _, _ := runTopLevel(t, "help", "extra")
	if code != 2 || stdout != "" || stderr != helpUsage+"\n" {
		t.Fatalf("help extra: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

// unroutedUsages are the texts printed when a path reaches no command: the
// top-level usage and the usage of each command group.
var unroutedUsages = []string{
	topLevelUsage,
	"Usage: ai-dev agent <run|select> [options]",
	"Usage: ai-dev workflow <state|transition|next|guide|approve|abandon> [options]",
	gitInitUsage,
}

// TestHelpCommandsAreRouted runs every command path the help lists, with no
// further arguments: none may fall through to the top-level or a group
// usage (F009-FR-003, SC-002).
func TestHelpCommandsAreRouted(t *testing.T) {
	for _, command := range catalog {
		if command.usage == "" {
			continue
		}
		_, stdout, stderr, _, launcher := runTopLevel(t, command.path...)
		if reachedNoCommand(stdout + stderr) {
			t.Fatalf("%v is listed in the help but not routed:\nstdout=%q\nstderr=%q", command.path, stdout, stderr)
		}
		if launcher.launches != 0 {
			t.Fatalf("%v launched a process with no arguments", command.path)
		}
	}
}

func TestTopLevelUsageNamesEveryGroup(t *testing.T) {
	for _, args := range [][]string{nil, {"bogus"}, {"--bogus"}} {
		code, stdout, stderr, _, _ := runTopLevel(t, args...)
		if code != 2 || stdout != "" || stderr != "Usage: ai-dev <doctor|agents|agent|workflow|init|git|setup|tui|version|help|completion> [options]\n" {
			t.Fatalf("%v: exit=%d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
	}
}

func reachedNoCommand(output string) bool {
	for _, usage := range unroutedUsages {
		if strings.Contains(output, usage) {
			return true
		}
	}
	return false
}

// TestUnroutedPathsAreDetected proves the routing check catches a listed
// command that does not exist, in every group.
func TestUnroutedPathsAreDetected(t *testing.T) {
	for _, path := range [][]string{{"bogus"}, {"agent", "bogus"}, {"workflow", "bogus"}, {"init", "bogus"}} {
		_, stdout, stderr, _, _ := runTopLevel(t, path...)
		if !reachedNoCommand(stdout + stderr) {
			t.Fatalf("%v was not detected as unrouted: stdout=%q stderr=%q", path, stdout, stderr)
		}
	}
}

// panicReader fails the test if help reads standard input (F010-FR-004).
type panicReader struct{ t *testing.T }

func (r panicReader) Read([]byte) (int, error) {
	r.t.Fatal("help read standard input")
	return 0, nil
}

// panicFS fails the test if help reads the project (F010-FR-004).
type panicFS struct{ t *testing.T }

func (f panicFS) Open(string) (fs.File, error) {
	f.t.Fatal("help read the project")
	return nil, fs.ErrNotExist
}

// runHelpIsolated runs args with dependencies that fail the test on any
// use: help must not touch them (F010-FR-004).
func runHelpIsolated(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	executor, launcher := &fakeExecutor{}, &appRunLauncher{}
	var stdout, stderr bytes.Buffer
	code := RunWithDependencies(context.Background(), args, Dependencies{
		Executor: executor, Launcher: launcher, Root: panicFS{t}, GateStore: unusedGateStore{t},
		Git: unavailableGitManagementDependencies(), Stdin: panicReader{t}, Stdout: &stdout, Stderr: &stderr,
	})
	if executor.lookups != 0 || executor.runs != 0 || launcher.launches != 0 {
		t.Fatalf("%v reached a process boundary", args)
	}
	return code, stdout.String(), stderr.String()
}

// Every catalog path answers each help form identically (F010-FR-001,
// SC-001).
func TestEveryPathAnswersHelp(t *testing.T) {
	for _, spec := range catalog {
		path := spec.path
		code, want, stderr := runHelpIsolated(t, append(slices.Clone(path), "--help")...)
		if code != 0 || stderr != "" || want == "" {
			t.Fatalf("%v --help: exit %d, stderr %q", path, code, stderr)
		}
		forms := [][]string{
			append(slices.Clone(path), "-h"),
			append([]string{"help"}, path...),
			append([]string{"--help"}, path...),
			append([]string{"-h"}, path...),
			append(slices.Clone(path), "--help", "extra"),
		}
		for _, args := range forms {
			if code, stdout, stderr := runHelpIsolated(t, args...); code != 0 || stderr != "" || stdout != want {
				t.Errorf("%v: exit %d, stderr %q, stdout differs from %v --help", args, code, stderr, path)
			}
		}
		if !strings.HasPrefix(want, "ai-dev "+strings.Join(path, " ")+": "+spec.description+"\n") {
			t.Errorf("%v help header:\n%s", path, want)
		}
		if spec.usage != "" {
			for _, part := range []string{"\n" + spec.usage + "\n", "  -h, --help", "\nChanges: " + spec.changes + "\n", "\nExit codes: " + spec.exitCodes + "\n"} {
				if !strings.Contains(want, part) {
					t.Errorf("%v help lacks %q:\n%s", path, part, want)
				}
			}
			for _, option := range spec.options {
				if !strings.Contains(want, "  --"+option.name) {
					t.Errorf("%v help lacks --%s", path, option.name)
				}
				for _, value := range option.values {
					if !strings.Contains(want, value) {
						t.Errorf("%v help lacks value %q of --%s", path, value, option.name)
					}
				}
			}
		}
		for _, child := range children(path) {
			if !strings.Contains(want, "\n  "+strings.Join(child.path, " ")+" ") {
				t.Errorf("%v help does not list %v:\n%s", path, child.path, want)
			}
		}
	}
}

func TestCommandAndGroupHelpLayout(t *testing.T) {
	_, state, _ := runHelpIsolated(t, "workflow", "state", "--help")
	wantState := "ai-dev workflow state: Report a feature's Workflow State\n\n" +
		"Usage: ai-dev workflow state [--format text|json] <feature-id>\n\n" +
		"Arguments:\n" +
		"  <feature-id>     The feature, as named under specs/\n" +
		"Options:\n" +
		"  --format FORMAT  Output format (default text) [values: text|json]\n" +
		"  -h, --help       Show this help\n" +
		"Changes: nothing\n" +
		"Exit codes: 0 positive result, 1 negative domain result, 2 usage or internal failure.\n"
	if state != wantState {
		t.Errorf("workflow state --help:\n%s\nwant:\n%s", state, wantState)
	}
	_, group, _ := runHelpIsolated(t, "workflow", "--help")
	wantGroup := "ai-dev workflow: Report, evaluate and decide a feature's Workflow State\n\n" +
		"Commands:\n" +
		"  workflow state       Report a feature's Workflow State\n" +
		"  workflow transition  Check whether a Workflow State transition is valid\n" +
		"  workflow next        Recommend the next Spec-Driven Development step\n" +
		"  workflow guide       Guide the next human actions, with command templates (read-only)\n" +
		"  workflow approve     Record a human approval and apply its transition\n" +
		"  workflow abandon     Record a human abandonment\n\n" +
		"Run 'ai-dev help <command>' for a command's options.\n"
	if group != wantGroup {
		t.Errorf("workflow --help:\n%s\nwant:\n%s", group, wantGroup)
	}
	_, init, _ := runHelpIsolated(t, "init", "--help")
	if !strings.Contains(init, "\n"+gitWizardUsage+"\n") || !strings.Contains(init, "\nCommands:\n  init inspect ") ||
		!strings.Contains(init, "\n  init reconcile ") {
		t.Errorf("init --help:\n%s", init)
	}
	_, gitHelp, _ := runHelpIsolated(t, "git", "--help")
	if !strings.Contains(gitHelp, "\n  git commit ") || strings.Contains(gitHelp, "validate") {
		t.Errorf("git --help:\n%s", gitHelp)
	}
}

// --help elsewhere than right after the path keeps the command's
// behavior (F010-FR-003).
func TestHelpIsRecognizedOnlyAfterThePath(t *testing.T) {
	launcher := &appRunLauncher{status: system.ExitStatus{Exited: true, ExitCode: 0}}
	var stdout, stderr bytes.Buffer
	code := RunAgentRun(context.Background(), []string{"claude-code", "--help"}, availableAgents(), launcher, bytes.NewReader(nil), false, "/project", &stdout, &stderr)
	if code != 0 || len(launcher.specs) != 1 || launcher.specs[0].Args[len(launcher.specs[0].Args)-1] != "--help" {
		t.Fatalf("agent run claude-code --help: exit %d, specs %#v", code, launcher.specs)
	}
	stdout.Reset()
	stderr.Reset()
	routed := RunWithDependencies(context.Background(), []string{"agent", "run", "claude-code", "--help"}, Dependencies{
		Executor: availableAgents(), Launcher: launcher, Root: fstest.MapFS{}, Git: unavailableGitManagementDependencies(),
		Stdin: bytes.NewReader(nil), Stdout: &stdout, Stderr: &stderr,
	})
	if routed != 0 || len(launcher.specs) != 2 || strings.Contains(stdout.String(), "Show this help") {
		t.Fatalf("routed agent run claude-code --help: exit %d, stdout %q", routed, stdout.String())
	}

	for _, args := range [][]string{
		{"workflow", "state", "f", "--help"},
		{"workflow", "state", "--", "--help"},
		{"workflow", "approve", "--approver", "--help"},
		{"version", "--format", "--help"},
	} {
		code, stdout, stderr, _, _ := runTopLevel(t, args...)
		if strings.Contains(stdout, "Show this help") || strings.Contains(stderr, "Show this help") {
			t.Errorf("%v was treated as a help request: exit %d", args, code)
		}
	}
	// Today's behavior is kept: a feature id and an extra argument is a
	// usage failure, and "--help" after "--" is a feature id.
	if code, _, stderr := runHelpIsolated(t, "workflow", "state", "f", "--help"); code != 2 || stderr != workflowStateUsage+"\n" {
		t.Errorf("workflow state f --help: exit %d, stderr %q", code, stderr)
	}
}

func TestHelpUsageFailures(t *testing.T) {
	for _, args := range [][]string{{"help", "nope"}, {"help", "workflow", "state", "extra"}, {"help", ""}, {"--help", "nope"}, {"help", "--zz"}} {
		code, stdout, stderr := runHelpIsolated(t, args...)
		if code != 2 || stdout != "" || stderr != helpUsage+"\n" {
			t.Errorf("%v: exit %d, stdout %q, stderr %q", args, code, stdout, stderr)
		}
	}
}
