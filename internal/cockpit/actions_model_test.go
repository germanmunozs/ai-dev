package cockpit

import (
	"reflect"
	"strings"
	"testing"
)

// openRunAgentReview goes from the dashboard to the review of "run agent"
// on 001-alpha with the given instruction.
func openRunAgentReview(t *testing.T, m Model, instruction string) Model {
	t.Helper()
	m, _ = press(t, m, "tab", "enter", "enter")
	if m.Mode() != "form" {
		t.Fatalf("mode %s, want form", m.Mode())
	}
	m = typeText(t, m, instruction)
	m, _ = press(t, m, "enter")
	if m.Mode() != "review" {
		t.Fatalf("mode %s, want review:\n%s", m.Mode(), screen(m))
	}
	return m
}

// TestRunAgentFormReviewAndConfirm proves US3 scenarios 1 and 5,
// F012-FR-009 and FR-012.
func TestRunAgentFormReviewAndConfirm(t *testing.T) {
	m, runner := newLoaded(t, defaultSource())
	m, _ = press(t, m, "tab", "enter", "enter")
	if !strings.Contains(screen(m), "instruction (required): ▏") {
		t.Fatalf("instruction not empty:\n%s", screen(m))
	}
	m = typeText(t, m, `-x "a b"`)
	m, _ = press(t, m, "enter")
	view := screen(m)
	for _, want := range []string{`ai-dev agent run -- claude-code -x "a b"`, reviewNotice, "[ Back ]", "[ Confirm ]"} {
		if !strings.Contains(view, want) {
			t.Errorf("review lacks %q:\n%s", want, view)
		}
	}
	m, _ = press(t, m, "enter") // Back is focused
	if m.Mode() != "form" || len(runner.calls) != 0 {
		t.Fatalf("Enter on Back: mode %s calls %q", m.Mode(), runner.calls)
	}
	m, _ = press(t, m, "enter", "tab", "enter")
	want := [][]string{{"agent", "run", "--", "claude-code", `-x "a b"`}}
	if !reflect.DeepEqual(runner.calls, want) || m.Mode() != "running" {
		t.Fatalf("calls %q mode %s", runner.calls, m.Mode())
	}
	m, _ = press(t, m, "enter", "tab", "enter", "q", "ctrl+c")
	if len(runner.calls) != 1 || m.Mode() != "running" {
		t.Fatalf("keys acted while running: calls %q mode %s", runner.calls, m.Mode())
	}
	m, _ = update(m, execDoneMsg{})
	press(t, m, "enter")
	if len(runner.calls) != 1 {
		t.Fatalf("a confirmation after the return started another command: %q", runner.calls)
	}
}

// TestEscRunsNothing: Esc at the form or the review starts nothing.
func TestEscRunsNothing(t *testing.T) {
	m, runner := newLoaded(t, defaultSource())
	review := openRunAgentReview(t, m, "x")
	review, _ = press(t, review, "tab", "esc")
	if review.Mode() != "form" {
		t.Fatalf("Esc at review: %s", review.Mode())
	}
	review, _ = press(t, review, "esc")
	if review.Mode() != "detail" || len(runner.calls) != 0 {
		t.Fatalf("Esc at form: mode %s calls %q", review.Mode(), runner.calls)
	}
}

// TestEmptyRequiredValueIsRefused.
func TestEmptyRequiredValueIsRefused(t *testing.T) {
	m, _ := newLoaded(t, defaultSource())
	m, _ = press(t, m, "tab", "enter", "enter", "enter")
	if m.Mode() != "form" || !strings.Contains(screen(m), "refused: the instruction is required") {
		t.Fatalf("empty instruction accepted: %s\n%s", m.Mode(), screen(m))
	}
}

// TestUnavailableRunAgentShowsReason proves US3 scenario 4.
func TestUnavailableRunAgentShowsReason(t *testing.T) {
	source := defaultSource()
	source.rows[0].AgentAvailable = false
	source.rows[0].Availability = Sanitize("unavailable")
	m, runner := newLoaded(t, source)
	m, _ = press(t, m, "tab", "enter")
	if !strings.Contains(screen(m), "Run the agent of the next step (unavailable: the agent of the next step is unavailable)") {
		t.Fatalf("no reason:\n%s", screen(m))
	}
	m, _ = press(t, m, "enter")
	if m.Mode() != "detail" || len(runner.calls) != 0 || !strings.Contains(screen(m), "is unavailable") {
		t.Fatalf("unavailable action opened: %s %q\n%s", m.Mode(), runner.calls, screen(m))
	}
}

// TestCommandFailureIsReported: a command that cannot be run is reported
// after the return, and nothing else happens.
func TestCommandFailureIsReported(t *testing.T) {
	m, runner := newLoaded(t, defaultSource())
	m = openRunAgentReview(t, m, "x")
	m, _ = press(t, m, "tab", "enter")
	m, cmd := update(m, execDoneMsg{err: errBoom})
	m = drain(t, m, cmd)
	if !strings.Contains(screen(m), `The command could not be run: boom \x1b]52;c;x\x07`) || len(runner.calls) != 1 {
		t.Fatalf("failure not reported:\n%s", screen(m))
	}
	if strings.Contains(m.View().Content, "\x1b]52") {
		t.Fatal("error rendered raw")
	}
}

// TestTerminationWaitsForTheCommand proves R-04: SIGTERM while a command
// runs is honored when it returns; SIGINT is the command's.
func TestTerminationWaitsForTheCommand(t *testing.T) {
	m, _ := newLoaded(t, defaultSource())
	m = openRunAgentReview(t, m, "x")
	m, _ = press(t, m, "tab", "enter")
	m, cmd := update(m, SignalMsg{})
	if cmd != nil || m.Mode() != "running" {
		t.Fatal("SIGINT acted while a command ran")
	}
	m, cmd = update(m, SignalMsg{Terminate: true})
	if cmd != nil || m.Mode() != "running" {
		t.Fatal("SIGTERM acted before the command returned")
	}
	m, cmd = update(m, execDoneMsg{})
	if !quits(cmd) || m.ExitCode() != 143 {
		t.Fatalf("deferred SIGTERM: quit=%t code=%d", quits(cmd), m.ExitCode())
	}
}

// keymap is every key of the default keymap plus common confirmation keys.
var keymap = []string{"j", "k", "up", "down", "enter", "tab", "shift+tab", "/", ":", "ctrl+p", "?", "r", "esc", "q", "ctrl+c", "y", "space"}

// TestNoKeySequenceActsWithoutConfirmation proves SC-006 and F012-FR-013:
// from every area and mode, every key and every pair of keys starts a
// command only through Enter on a focused Confirm of a review.
func TestNoKeySequenceActsWithoutConfirmation(t *testing.T) {
	base, _ := newLoaded(t, defaultSource())
	starts := map[string][]string{}
	for i, name := range areaOrder {
		path := []string{}
		for range i {
			path = append(path, "tab")
		}
		starts["browse "+name] = path
		starts["detail "+name] = append(append([]string{}, path...), "enter")
		starts["filter "+name] = append(append([]string{}, path...), "/")
		starts["help "+name] = append(append([]string{}, path...), "?")
	}
	starts["palette"] = []string{":"}
	starts["form run-agent"] = []string{"tab", "enter", "enter"}
	starts["form approve"] = []string{"tab", "enter", "j", "enter"}
	starts["form abandon"] = []string{"tab", "enter", "j", "j", "enter"}
	starts["review setup"] = []string{"tab", "tab", "tab", "enter", "enter"}
	starts["review run-agent"] = []string{"tab", "enter", "enter", "x", "enter"}
	starts["review run-agent focused"] = []string{"tab", "enter", "enter", "x", "enter", "tab"}
	starts["form commit message"] = []string{"tab", "tab", "tab", "tab", "enter", "j", "enter"}

	sequences := 0
	for name, path := range starts {
		start, runner := withRunner(base)
		start, _ = press(t, start, path...)
		for _, want := range []string{"form", "review", "palette"} {
			if strings.HasPrefix(name, want) && start.Mode() != want {
				t.Fatalf("start %q reached mode %s", name, start.Mode())
			}
		}
		before := len(runner.calls)
		check := func(m Model, keys ...string) {
			for _, k := range keys {
				confirmable := m.mode == reviewMode && m.review.confirmFocused
				calls := len(runner.calls)
				m, _ = press(t, m, k)
				if len(runner.calls) > calls && !(confirmable && k == "enter") {
					t.Fatalf("%s then %q: a command started without the review's confirmation: %q", name, keys, runner.calls)
				}
			}
		}
		for _, k1 := range keymap {
			check(start, k1)
			for _, k2 := range keymap {
				check(start, k1, k2)
				sequences++
			}
		}
		if name == "review run-agent focused" && len(runner.calls) == before {
			t.Fatal("the confirmation path itself never ran: the test proves nothing")
		}
	}
	t.Logf("%d states, %d key pairs each, %d sequences", len(starts), len(keymap)*len(keymap), sequences)
}

// withRunner gives a copy of m a fresh recording runner.
func withRunner(m Model) (Model, *fakeRunner) {
	runner := &fakeRunner{}
	m.cfg.Runner = runner
	return m, runner
}

// TestApproveForm proves US4 scenario 1: approver, target state and an
// optional reason, none prefilled, a hint with the canonical states, and
// the exact vector in the review.
func TestApproveForm(t *testing.T) {
	m, runner := newLoaded(t, defaultSource())
	m, _ = press(t, m, "tab", "enter", "j", "enter")
	view := screen(m)
	for _, want := range []string{"Approve the feature", "approver (required): ▏", "target state (required): ", "reason: ",
		"draft, specified, planned, tasked, implemented, approved or abandoned"} {
		if !strings.Contains(view, want) {
			t.Errorf("approve form lacks %q:\n%s", want, view)
		}
	}
	m = typeText(t, m, "Germán")
	m, _ = press(t, m, "enter")
	m = typeText(t, m, "specified")
	m, _ = press(t, m, "enter", "enter") // empty optional reason
	if m.Mode() != "review" || !strings.Contains(screen(m), "ai-dev workflow approve --approver=Germán -- 001-alpha specified") {
		t.Fatalf("review: %s\n%s", m.Mode(), screen(m))
	}
	press(t, m, "tab", "enter")
	want := [][]string{{"workflow", "approve", "--approver=Germán", "--", "001-alpha", "specified"}}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls %q", runner.calls)
	}
}

// TestAbandonRequiresReason proves US4 scenario 2.
func TestAbandonRequiresReason(t *testing.T) {
	m, runner := newLoaded(t, defaultSource())
	m, _ = press(t, m, "tab", "enter", "j", "j", "enter")
	m = typeText(t, m, "me")
	m, _ = press(t, m, "enter", "enter")
	if m.Mode() != "form" || !strings.Contains(screen(m), "refused: the reason is required") {
		t.Fatalf("abandon without reason: %s\n%s", m.Mode(), screen(m))
	}
	m = typeText(t, m, "out of scope")
	press(t, m, "enter", "tab", "enter")
	want := [][]string{{"workflow", "abandon", "--approver=me", "--reason=out of scope", "--", "001-alpha", "abandoned"}}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls %q", runner.calls)
	}
}

// TestTerminalFeatureOffersNoDecision proves F012-FR-011 in the view.
func TestTerminalFeatureOffersNoDecision(t *testing.T) {
	m, runner := newLoaded(t, defaultSource())
	m, _ = press(t, m, "tab", "j", "enter")
	view := screen(m)
	for _, want := range []string{"Approve the feature (unavailable: the feature is in a terminal state)",
		"Abandon the feature (unavailable: the feature is in a terminal state)"} {
		if !strings.Contains(view, want) {
			t.Errorf("lacks %q:\n%s", want, view)
		}
	}
	m, _ = press(t, m, "j", "enter", "j", "enter")
	if m.Mode() != "detail" || len(runner.calls) != 0 {
		t.Fatalf("a decision opened on a terminal feature: %s %q", m.Mode(), runner.calls)
	}
}

// TestProjectActions proves US5 scenario 4: configure agents, initialize
// the repository and validate a commit message go through the same review.
func TestProjectActions(t *testing.T) {
	cases := []struct {
		path []string
		text string
		want []string
	}{
		{[]string{"tab", "tab", "tab", "enter", "enter"}, "", []string{"setup"}},
		{[]string{"tab", "tab", "tab", "tab", "enter", "enter"}, "", []string{"init"}},
		{[]string{"tab", "tab", "tab", "tab", "enter", "j", "enter"}, "feat: x", []string{"git", "commit", "validate", "--message=feat: x"}},
	}
	for _, c := range cases {
		m, runner := newLoaded(t, defaultSource())
		m, _ = press(t, m, c.path...)
		if c.text != "" {
			m = typeText(t, m, c.text)
			m, _ = press(t, m, "enter")
		}
		if m.Mode() != "review" {
			t.Fatalf("%q: mode %s\n%s", c.want, m.Mode(), screen(m))
		}
		press(t, m, "tab", "enter")
		if !reflect.DeepEqual(runner.calls, [][]string{c.want}) {
			t.Fatalf("calls %q, want %q", runner.calls, c.want)
		}
	}
}

// TestLineAreasShowTheReports: Environment, Agents and Repository show
// their reports and scroll.
func TestLineAreasShowTheReports(t *testing.T) {
	m, _ := newLoaded(t, defaultSource())
	m, _ = press(t, m, "tab", "tab")
	if view := screen(m); !strings.Contains(view, "[PASS] Git") || !strings.Contains(view, "Overall: Ready") {
		t.Fatalf("environment:\n%s", view)
	}
	m, _ = press(t, m, "j")
	if strings.Contains(screen(m), "Environment\n") {
		t.Fatalf("j did not scroll:\n%s", screen(m))
	}
	m, _ = press(t, m, "tab")
	if !strings.Contains(screen(m), "agent claude-code availability=available") || !strings.Contains(screen(m), "Enter: actions for this area") {
		t.Fatalf("agents:\n%s", screen(m))
	}
	m, _ = press(t, m, "tab")
	if !strings.Contains(screen(m), "repository:     normal") {
		t.Fatalf("repository:\n%s", screen(m))
	}
}
