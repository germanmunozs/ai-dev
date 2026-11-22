package cockpit

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// TestFeaturesTable proves US2 scenario 1: one row per feature with state,
// next activity, agent, availability and source.
func TestFeaturesTable(t *testing.T) {
	m, _ := newLoaded(t, defaultSource())
	m, _ = press(t, m, "tab")
	view := screen(m)
	for _, want := range []string{"FEATURE", "STATE", "NEXT", "AGENT", "AVAILABILITY", "SOURCE",
		"001-alpha", "draft", "step_recommended", "claude-code", "available", "built_in",
		"002-done", "approved", "workflow_terminal", "003-broken", "state_invalid"} {
		if !strings.Contains(view, want) {
			t.Errorf("features table lacks %q:\n%s", want, view)
		}
	}
}

// TestFeatureDetail proves US2 scenarios 2 and 3: Enter shows the
// commands' text and the actions; an invalid feature offers none.
func TestFeatureDetail(t *testing.T) {
	m, _ := newLoaded(t, defaultSource())
	m, _ = press(t, m, "tab", "enter")
	view := screen(m)
	for _, want := range []string{`ai-dev: outcome=state_reported feature="001-alpha" state=draft`, "# run the agent (002)",
		"Run the agent of the next step (available)", "Approve the feature (available)", "Abandon the feature (available)"} {
		if !strings.Contains(view, want) {
			t.Errorf("detail lacks %q:\n%s", want, view)
		}
	}
	m, _ = press(t, m, "esc", "j", "j", "enter")
	view = screen(m)
	if !strings.Contains(view, `outcome=state_invalid feature="003-broken"`) {
		t.Fatalf("invalid outcome not shown:\n%s", view)
	}
	if strings.Contains(view, "(available)") {
		t.Fatalf("an action is offered for an invalid feature:\n%s", view)
	}
}

// TestRefreshReloadsEveryArea proves US2 scenario 4 and F012-FR-006.
func TestRefreshReloadsEveryArea(t *testing.T) {
	source := defaultSource()
	m, _ := newLoaded(t, source)
	if !strings.Contains(screen(m), "evaluated 15:04:05") {
		t.Fatalf("evaluation time not shown:\n%s", screen(m))
	}
	press(t, m, "r")
	if source.featureLoads != 2 || source.envLoads != 2 || source.agentLoads != 2 || source.repoLoads != 2 {
		t.Fatalf("loads after r: %d %d %d %d", source.featureLoads, source.envLoads, source.agentLoads, source.repoLoads)
	}
}

// TestReturnFromActionReloadsAndKeepsSelection proves F012-FR-010 and FR-015.
func TestReturnFromActionReloadsAndKeepsSelection(t *testing.T) {
	source := defaultSource()
	m, runner := newLoaded(t, source)
	m, _ = press(t, m, "tab", "enter", "enter")
	m = typeText(t, m, "do it")
	m, _ = press(t, m, "enter", "tab", "enter")
	if len(runner.calls) != 1 || m.Mode() != "running" {
		t.Fatalf("calls %q mode %s", runner.calls, m.Mode())
	}
	m, cmd := update(m, execDoneMsg{err: CommandExit{Code: 3}})
	m = drain(t, m, cmd)
	if source.featureLoads != 2 || source.envLoads != 2 {
		t.Fatalf("not reloaded: %d %d", source.featureLoads, source.envLoads)
	}
	if m.AreaName() != "Features" || m.SelectedFeature() != "001-alpha" || m.Mode() != "detail" {
		t.Fatalf("did not return: %s %s %s", m.AreaName(), m.SelectedFeature(), m.Mode())
	}
	if !strings.Contains(screen(m), "The command exited with code 3.") {
		t.Fatalf("exit code not shown:\n%s", screen(m))
	}
}

// TestLoadingSaysSoAndKeysWork proves F012-FR-007.
func TestLoadingSaysSoAndKeysWork(t *testing.T) {
	m := New(Config{Context: context.Background(), Source: defaultSource(), Runner: &fakeRunner{}, Now: func() time.Time { return fixedNow }})
	m, _ = update(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if !strings.Contains(screen(m), "evaluating…") {
		t.Fatalf("loading not said:\n%s", screen(m))
	}
	m, _ = press(t, m, "tab", "tab")
	if m.AreaName() != "Environment" || !strings.Contains(screen(m), "evaluating…") {
		t.Fatalf("keys blocked while loading: %s\n%s", m.AreaName(), screen(m))
	}
}

// TestNoFeatures: an empty specs/ explains how features are created.
func TestNoFeatures(t *testing.T) {
	source := defaultSource()
	source.rows = nil
	m, _ := newLoaded(t, source)
	m, _ = press(t, m, "tab")
	if !strings.Contains(screen(m), "No features under specs/.") || !strings.Contains(screen(m), "/speckit-specify") {
		t.Fatalf("no explanation:\n%s", screen(m))
	}
	if _, cmd := press(t, m, "enter"); cmd != nil {
		t.Fatal("Enter did something without features")
	}
}

// TestFailedReloadDropsStaleData: an area whose reload fails shows its
// error and none of its previous lines (edge case).
func TestFailedReloadDropsStaleData(t *testing.T) {
	source := defaultSource()
	m, _ := newLoaded(t, source)
	source.featErr, source.envErr = errBoom, errBoom
	m, _ = press(t, m, "r", "tab")
	view := screen(m)
	if strings.Contains(view, "001-alpha") || !strings.Contains(view, "failed: the features cannot be evaluated") {
		t.Fatalf("stale features shown:\n%s", view)
	}
	m, _ = press(t, m, "tab")
	view = screen(m)
	if strings.Contains(view, "[PASS] Git") || !strings.Contains(view, `boom \x1b]52;c;x\x07`) {
		t.Fatalf("stale environment shown, or the error raw:\n%s", view)
	}
}
