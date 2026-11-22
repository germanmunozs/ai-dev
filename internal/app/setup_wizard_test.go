package app

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

type wizardRun struct {
	code   int
	stdout string
	stderr string
	store  *fakeConfigStore
	exec   *setupExecutor
}

func runWizard(t *testing.T, ctx context.Context, root fstest.MapFS, input io.Reader, interactive bool, args ...string) wizardRun {
	t.Helper()
	store := &fakeConfigStore{}
	executor := onlyClaude()
	var stdout, stderr bytes.Buffer
	code := RunSetupWizard(ctx, args, executor, root, store, input, interactive, &stdout, &stderr)
	return wizardRun{code, stdout.String(), stderr.String(), store, executor}
}

func lines(answers ...string) io.Reader { return strings.NewReader(strings.Join(answers, "\n") + "\n") }

// The full flow writes the canonical configuration only after "yes"
// (F011-FR-010, contracts/setup-cli.md).
func TestSetupWizardWritesAfterYes(t *testing.T) {
	want := string(testConfig(t, "claude-code").Encode()) // implementation=claude-code, remediation=codex-cli
	for name, input := range map[string]io.Reader{
		"numbers": lines("1,2", "1", "1", "1", "1", "2", "yes"),
		"ids":     lines("codex-cli, claude-code", "claude-code", "claude-code", "claude-code", "claude-code", "codex-cli", "yes"),
	} {
		t.Run(name, func(t *testing.T) {
			run := runWizard(t, context.Background(), fstest.MapFS{}, input, true)
			if run.code != 0 || len(run.store.writes) != 1 || string(run.store.writes[0]) != want {
				t.Fatalf("code=%d writes=%q stderr=%q stdout=%q", run.code, run.store.writes, run.stderr, run.stdout)
			}
			for _, fragment := range []string{
				"  1. claude-code (Claude Code): available\n", "  2. codex-cli (OpenAI Codex CLI): unavailable\n",
				"warning: codex-cli is not available on this machine; it can still be configured\n",
				"Agent for specification (1 claude-code, 2 codex-cli): ",
				"This configuration will be written to .ai-dev/agents.json:\n  agents: claude-code, codex-cli\n  specification: claude-code\n",
				"Write it? [yes/no] ", "ai-dev: outcome=configuration_written source=project\n",
			} {
				if !strings.Contains(run.stdout, fragment) {
					t.Errorf("missing %q in:\n%s", fragment, run.stdout)
				}
			}
			if run.exec.runs != 0 {
				t.Fatal("the assistant ran a process")
			}
		})
	}
}

// With a valid configuration, empty answers keep it: unchanged, nothing
// written.
func TestSetupWizardKeepsCurrentAnswers(t *testing.T) {
	run := runWizard(t, context.Background(), configFS(projectConfigImplementationClaude), lines("", "", "", "", "", "", "yes"), true)
	if run.code != 0 || len(run.store.writes) != 0 || !strings.Contains(run.stdout, "ai-dev: outcome=configuration_unchanged source=project\n") {
		t.Fatalf("code=%d writes=%q stdout=%q", run.code, run.store.writes, run.stdout)
	}
	if !strings.Contains(run.stdout, "[current: claude-code, codex-cli]") || !strings.Contains(run.stdout, "Agent for implementation (1 claude-code, 2 codex-cli) [current: claude-code]: ") {
		t.Fatalf("current answers not shown:\n%s", run.stdout)
	}
}

// After "yes", a different valid configuration and an invalid one are both
// replaced: the confirmation is the explicit decision.
func TestSetupWizardReplacesAfterYes(t *testing.T) {
	for name, root := range map[string]fstest.MapFS{
		"different": configFS(projectConfigImplementationClaude),
		"invalid":   configFS(`{"schema_version":"ai-dev.agents/v1","agents":["gemini"],"roles":{}}`),
	} {
		t.Run(name, func(t *testing.T) {
			run := runWizard(t, context.Background(), root, lines("2", "1", "1", "1", "1", "1", "yes"), true)
			if run.code != 0 || len(run.store.writes) != 1 || !strings.Contains(run.stdout, "outcome=configuration_written") {
				t.Fatalf("code=%d writes=%d stdout=%q stderr=%q", run.code, len(run.store.writes), run.stdout, run.stderr)
			}
		})
	}
	run := runWizard(t, context.Background(), configFS(`{"schema_version":"ai-dev.agents/v1","agents":["gemini"],"roles":{}}`), lines(""), true)
	if !strings.Contains(run.stdout, `  reason unknown_agent "gemini"`) || run.code != 2 {
		t.Fatalf("invalid existing configuration: code=%d stdout=%q", run.code, run.stdout)
	}
}

// Declining, end of input and an interrupt cancel with nothing written.
func TestSetupWizardCancels(t *testing.T) {
	for name, input := range map[string]io.Reader{
		"no":                    lines("1", "1", "1", "1", "1", "1", "no"),
		"y":                     lines("1", "1", "1", "1", "1", "1", "y"),
		"empty":                 lines("1", "1", "1", "1", "1", "1", ""),
		"end of input":          lines("1", "1"),
		"yes then end of input": strings.NewReader("1\n1\n1\n1\n1\n1\nyes"),
	} {
		t.Run(name, func(t *testing.T) {
			run := runWizard(t, context.Background(), fstest.MapFS{}, input, true)
			if run.code != 1 || len(run.store.writes) != 0 || !strings.Contains(run.stdout, "Cancelled. Nothing was changed.\n") || !strings.Contains(run.stdout, "ai-dev: outcome=setup_cancelled\n") {
				t.Fatalf("code=%d writes=%d stdout=%q", run.code, len(run.store.writes), run.stdout)
			}
		})
	}
	t.Run("interrupt", func(t *testing.T) {
		reader, writer := io.Pipe()
		defer writer.Close()
		ctx, cancel := context.WithCancel(context.Background())
		go func() { time.Sleep(50 * time.Millisecond); cancel() }()
		run := runWizard(t, ctx, fstest.MapFS{}, reader, true)
		if run.code != 1 || len(run.store.writes) != 0 || !strings.Contains(run.stdout, "outcome=setup_cancelled") {
			t.Fatalf("code=%d stdout=%q", run.code, run.stdout)
		}
	})
}

// Invalid choices are usage failures: exit 2, nothing written.
func TestSetupWizardInvalidAnswers(t *testing.T) {
	for name, input := range map[string]io.Reader{
		"unknown number":     lines("3"),
		"unknown id":         lines("gemini"),
		"no agent":           lines(""),
		"duplicate":          lines("1,1"),
		"role not chosen":    lines("1", "2"),
		"role unknown":       lines("1,2", "codex"),
		"empty role no keep": lines("1", ""),
	} {
		t.Run(name, func(t *testing.T) {
			run := runWizard(t, context.Background(), fstest.MapFS{}, input, true)
			if run.code != 2 || len(run.store.writes) != 0 || run.stderr != "ai-dev: setup: invalid input; nothing was changed\n" {
				t.Fatalf("code=%d stderr=%q writes=%d", run.code, run.stderr, len(run.store.writes))
			}
		})
	}
}

func TestSetupWizardRefusals(t *testing.T) {
	run := runWizard(t, context.Background(), fstest.MapFS{}, lines("1"), false)
	if run.code != 2 || run.stdout != "" || run.stderr != "ai-dev: setup: refused: interactive_terminal_required; use ai-dev setup apply\n" || len(run.exec.lookups) != 0 {
		t.Fatalf("not interactive: code=%d stdout=%q stderr=%q lookups=%v", run.code, run.stdout, run.stderr, run.exec.lookups)
	}
	for _, args := range [][]string{{"--format", "json"}, {"extra"}} {
		run := runWizard(t, context.Background(), fstest.MapFS{}, lines("1"), true, args...)
		if run.code != 2 || run.stderr != setupWizardUsage+"\n" || len(run.exec.lookups) != 0 {
			t.Errorf("%v: code=%d stderr=%q", args, run.code, run.stderr)
		}
	}
	store := &fakeConfigStore{fail: true}
	var stdout, stderr bytes.Buffer
	code := RunSetupWizard(context.Background(), nil, onlyClaude(), fstest.MapFS{}, store, lines("1", "1", "1", "1", "1", "1", "yes"), true, &stdout, &stderr)
	if code != 1 || !strings.Contains(stdout.String(), "ai-dev: outcome=persistence_failed\n") || !strings.Contains(stderr.String(), "injected write failure") {
		t.Fatalf("store failure: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
