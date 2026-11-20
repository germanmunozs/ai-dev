package app

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"
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
	"Usage: ai-dev workflow <state|transition> [options]",
	gitInitUsage,
}

// TestHelpCommandsAreRouted runs every command path the help lists, with no
// further arguments: none may fall through to the top-level or a group
// usage (F009-FR-003, SC-002).
func TestHelpCommandsAreRouted(t *testing.T) {
	for _, command := range helpCommands {
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
		if code != 2 || stdout != "" || stderr != "Usage: ai-dev <doctor|agents|agent|workflow|init|git|version|help> [options]\n" {
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
