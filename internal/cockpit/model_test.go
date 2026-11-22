package cockpit

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

var areaOrder = []string{"Dashboard", "Features", "Environment", "Agents", "Repository", "Help"}

// TestTabCyclesEveryArea proves F012-FR-004 and FR-013: Tab and Shift-Tab
// visit the six areas in order and wrap.
func TestTabCyclesEveryArea(t *testing.T) {
	m, _ := newLoaded(t, defaultSource())
	for i := range areaOrder {
		if got := m.AreaName(); got != areaOrder[i] {
			t.Fatalf("step %d: area %q, want %q", i, got, areaOrder[i])
		}
		if !strings.Contains(screen(m), "ai-dev tui · "+areaOrder[i]) {
			t.Fatalf("header does not name %s:\n%s", areaOrder[i], screen(m))
		}
		m, _ = press(t, m, "tab")
	}
	if m.AreaName() != "Dashboard" {
		t.Fatalf("Tab does not wrap: %s", m.AreaName())
	}
	m, _ = press(t, m, "shift+tab")
	if m.AreaName() != "Help" {
		t.Fatalf("Shift-Tab does not wrap back: %s", m.AreaName())
	}
}

// TestDashboardSummarizesEveryArea proves US1 scenario 1.
func TestDashboardSummarizesEveryArea(t *testing.T) {
	m, _ := newLoaded(t, defaultSource())
	view := screen(m)
	for _, want := range []string{"Features", "3 features", "1 draft", "1 approved", "1 state_invalid",
		"Environment", "Overall: Ready", "Agents", "outcome=not_configured", "Repository", "project:", "Tab"} {
		if !strings.Contains(view, want) {
			t.Errorf("dashboard lacks %q:\n%s", want, view)
		}
	}
}

// TestMoveAndClamp: j, k and the arrows move the selection and clamp.
func TestMoveAndClamp(t *testing.T) {
	m, _ := newLoaded(t, defaultSource())
	m, _ = press(t, m, "tab")
	if m.SelectedFeature() != "001-alpha" {
		t.Fatalf("initial selection %q", m.SelectedFeature())
	}
	m, _ = press(t, m, "j", "down", "j", "j")
	if m.SelectedFeature() != "003-broken" {
		t.Fatalf("after moving down: %q", m.SelectedFeature())
	}
	m, _ = press(t, m, "k", "up", "k", "k")
	if m.SelectedFeature() != "001-alpha" {
		t.Fatalf("after moving up: %q", m.SelectedFeature())
	}
}

// TestFilter: / filters the current list and Esc clears it.
func TestFilter(t *testing.T) {
	m, _ := newLoaded(t, defaultSource())
	m, _ = press(t, m, "tab", "/")
	m = typeText(t, m, "brok")
	view := screen(m)
	if strings.Contains(view, "001-alpha") || !strings.Contains(view, "003-broken") {
		t.Fatalf("filter not applied:\n%s", view)
	}
	m, _ = press(t, m, "enter")
	if m.SelectedFeature() != "003-broken" {
		t.Fatalf("selection under filter: %q", m.SelectedFeature())
	}
	m, _ = press(t, m, "/", "esc")
	if !strings.Contains(screen(m), "001-alpha") {
		t.Fatalf("Esc did not clear the filter:\n%s", screen(m))
	}
	m, _ = press(t, m, "tab", "/")
	m = typeText(t, m, "Overall")
	if view := screen(m); strings.Contains(view, "[PASS] Git") || !strings.Contains(view, "Overall: Ready") {
		t.Fatalf("line filter not applied:\n%s", view)
	}
}

// TestPaletteListsAreasAndActions proves F012-FR-014 and US1 scenario 3.
func TestPaletteListsAreasAndActions(t *testing.T) {
	for _, open := range []string{":", "ctrl+p"} {
		m, _ := newLoaded(t, defaultSource())
		m, _ = press(t, m, open)
		view := screen(m)
		for _, area := range areaOrder {
			if !strings.Contains(view, "Go to "+area) {
				t.Errorf("%s: palette lacks area %s:\n%s", open, area, view)
			}
		}
		for _, action := range Actions() {
			if !strings.Contains(view, action.Title) {
				t.Errorf("%s: palette lacks action %q:\n%s", open, action.Title, view)
			}
		}
		m = typeText(t, m, "appr")
		view = screen(m)
		if !strings.Contains(view, "Approve the feature") || strings.Contains(view, "Go to Agents") {
			t.Fatalf("palette not filtered:\n%s", view)
		}
		if !strings.Contains(view, "available") {
			t.Fatalf("palette does not show availability:\n%s", view)
		}
	}
}

// TestPaletteShowsUnavailableReasonAndStartsNothing: an unavailable action
// is listed with its reason and cannot be started.
func TestPaletteShowsUnavailableReasonAndStartsNothing(t *testing.T) {
	m, runner := newLoaded(t, defaultSource())
	m, _ = press(t, m, "tab", "j") // 002-done, terminal
	m, _ = press(t, m, ":")
	m = typeText(t, m, "approve")
	if !strings.Contains(screen(m), "unavailable: the feature is in a terminal state") {
		t.Fatalf("no reason:\n%s", screen(m))
	}
	m, _ = press(t, m, "enter", "tab", "enter")
	if m.Mode() == "form" || m.Mode() == "review" || len(runner.calls) != 0 {
		t.Fatalf("unavailable action opened: mode %s, calls %q", m.Mode(), runner.calls)
	}
}

// TestPaletteGoesToArea.
func TestPaletteGoesToArea(t *testing.T) {
	m, _ := newLoaded(t, defaultSource())
	m, _ = press(t, m, ":")
	m = typeText(t, m, "Go to Repo")
	m, _ = press(t, m, "enter")
	if m.AreaName() != "Repository" || m.Mode() != "browse" {
		t.Fatalf("area %s mode %s", m.AreaName(), m.Mode())
	}
}

// TestHelpListsKeysAndReturns proves US1 scenario 2 and F012-FR-015.
func TestHelpListsKeysAndReturns(t *testing.T) {
	m, _ := newLoaded(t, defaultSource())
	m, _ = press(t, m, "tab", "j", "?")
	view := screen(m)
	for _, want := range []string{"Tab", "Shift-Tab", "j/k", "Enter", "/", "Ctrl-P", "?", "r", "Esc", "q", "Ctrl-C"} {
		if !strings.Contains(view, want) {
			t.Errorf("help lacks %q:\n%s", want, view)
		}
	}
	m, _ = press(t, m, "esc")
	if m.AreaName() != "Features" || m.SelectedFeature() != "002-done" || m.Mode() != "browse" {
		t.Fatalf("help did not return: %s %s %s", m.AreaName(), m.SelectedFeature(), m.Mode())
	}
}

// TestQuit: q quits from the top level only; Ctrl-C quits outside a
// running command; both exit 0.
func TestQuit(t *testing.T) {
	m, _ := newLoaded(t, defaultSource())
	if _, cmd := press(t, m, "q"); !quits(cmd) {
		t.Fatal("q did not quit")
	}
	inPalette, _ := press(t, m, ":")
	if _, cmd := press(t, inPalette, "q"); quits(cmd) {
		t.Fatal("q quit from the palette")
	}
	if next, cmd := press(t, inPalette, "ctrl+c"); !quits(cmd) || next.ExitCode() != 0 {
		t.Fatal("Ctrl-C did not quit with 0")
	}
}

// TestSignalsQuitWithTheirCodes (R-04, contracts/cockpit.md).
func TestSignalsQuitWithTheirCodes(t *testing.T) {
	m, _ := newLoaded(t, defaultSource())
	next, cmd := update(m, SignalMsg{})
	if !quits(cmd) || next.ExitCode() != 130 {
		t.Fatalf("SIGINT: quit=%t code=%d", quits(cmd), next.ExitCode())
	}
	next, cmd = update(m, SignalMsg{Terminate: true})
	if !quits(cmd) || next.ExitCode() != 143 {
		t.Fatalf("SIGTERM: quit=%t code=%d", quits(cmd), next.ExitCode())
	}
}

// TestTooSmall proves F012-FR-003: below 80×24 only the notice renders, and
// no input confirms; growing restores the previous view.
func TestTooSmall(t *testing.T) {
	m, runner := newLoaded(t, defaultSource())
	m, _ = press(t, m, "tab", "enter", "enter") // run agent form
	m = typeText(t, m, "go")
	m, _ = press(t, m, "enter", "tab") // review, confirm focused
	if m.Mode() != "review" {
		t.Fatalf("mode %s", m.Mode())
	}
	small, _ := update(m, tea.WindowSizeMsg{Width: 79, Height: 30})
	if got := strings.TrimSpace(screen(small)); got != "Terminal too small: 79×30, need 80×24." {
		t.Fatalf("notice %q", got)
	}
	small, _ = press(t, small, "enter", "enter", "y", "space")
	if len(runner.calls) != 0 {
		t.Fatalf("a key confirmed below the minimum size: %q", runner.calls)
	}
	short, _ := update(small, tea.WindowSizeMsg{Width: 120, Height: 23})
	if !strings.Contains(screen(short), "Terminal too small: 120×23") {
		t.Fatalf("height not checked: %q", screen(short))
	}
	grown, _ := update(short, tea.WindowSizeMsg{Width: 80, Height: 24})
	if grown.Mode() != "review" || !strings.Contains(screen(grown), "Confirm") {
		t.Fatalf("view not restored: %s\n%s", grown.Mode(), screen(grown))
	}
}

// TestStatusesAreWords proves F012-FR-019 for the loading and failed states.
func TestStatusesAreWords(t *testing.T) {
	source := defaultSource()
	source.envErr = errBoom
	m, _ := newLoaded(t, source)
	view := screen(m)
	if !strings.Contains(view, "failed") || !strings.Contains(view, "ready") {
		t.Fatalf("statuses not given as words:\n%s", view)
	}
	if strings.Contains(m.View().Content, "\x1b]52") {
		t.Fatal("error rendered raw")
	}
}

// TestViewFitsTheWindow: no rendered line is wider than the window.
func TestViewFitsTheWindow(t *testing.T) {
	source := defaultSource()
	source.repo = lines(strings.Repeat("x", 300))
	m, _ := newLoaded(t, source)
	m, _ = update(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	for range areaOrder {
		for _, line := range strings.Split(screen(m), "\n") {
			if w := len([]rune(line)); w > 80 {
				t.Fatalf("%s: line of width %d: %q", m.AreaName(), w, line)
			}
		}
		if n := strings.Count(screen(m), "\n") + 1; n > 24 {
			t.Fatalf("%s: %d lines", m.AreaName(), n)
		}
		m, _ = press(t, m, "tab")
	}
}

// TestPaletteWithoutMatches is the regression test of review F2: moving and
// choosing in an empty palette does nothing and does not panic.
func TestPaletteWithoutMatches(t *testing.T) {
	m, runner := newLoaded(t, defaultSource())
	m, _ = press(t, m, ":")
	m = typeText(t, m, "zzz")
	m, _ = press(t, m, "down", "ctrl+n", "up", "enter")
	if m.Mode() != "palette" || len(runner.calls) != 0 {
		t.Fatalf("mode %s calls %q", m.Mode(), runner.calls)
	}
	_ = m.View()
}

// TestReviewShowsTheWholeCommand is the regression test of review F3: a
// command wider than the window is wrapped, never cut, and a long input
// keeps its cursor and typed end visible.
func TestReviewShowsTheWholeCommand(t *testing.T) {
	m, _ := newLoaded(t, defaultSource())
	m, _ = update(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = press(t, m, "tab", "enter", "j", "enter")
	m = typeText(t, m, "Germán")
	m, _ = press(t, m, "enter")
	m = typeText(t, m, "specified")
	m, _ = press(t, m, "enter")
	reason := strings.Repeat("reviewed the plan and everything in it ", 3) + "END"
	m = typeText(t, m, reason)
	if !strings.Contains(screen(m), "END▏") {
		t.Fatalf("the typed end and cursor are hidden:\n%s", screen(m))
	}
	m, _ = press(t, m, "enter")
	joined := strings.ReplaceAll(strings.ReplaceAll(screen(m), "\n  ", ""), "\n", "")
	if !strings.Contains(joined, "--reason="+reason+" -- 001-alpha specified") {
		t.Fatalf("the review does not show the whole command:\n%s", screen(m))
	}
	for _, line := range strings.Split(screen(m), "\n") {
		if w := len([]rune(line)); w > 80 {
			t.Fatalf("line of width %d: %q", w, line)
		}
	}
}
