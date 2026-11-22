package cockpit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// fakeSource returns fixed data; a non-nil error fails that area.
type fakeSource struct {
	rows                   []FeatureRow
	env, agents, repo      []Text
	featErr, envErr        error
	agentsErr, repoErr     error
	featureLoads, envLoads int
	agentLoads, repoLoads  int
}

func (s *fakeSource) LoadFeatures(context.Context) ([]FeatureRow, error) {
	s.featureLoads++
	return s.rows, s.featErr
}
func (s *fakeSource) LoadEnvironment(context.Context) ([]Text, error) {
	s.envLoads++
	return s.env, s.envErr
}
func (s *fakeSource) LoadAgents(context.Context) ([]Text, error) {
	s.agentLoads++
	return s.agents, s.agentsErr
}
func (s *fakeSource) LoadRepository(context.Context) ([]Text, error) {
	s.repoLoads++
	return s.repo, s.repoErr
}

// fakeRunner records every command the model asks for. It never runs one.
type fakeRunner struct{ calls [][]string }

func (r *fakeRunner) Command(args []string) tea.ExecCommand {
	r.calls = append(r.calls, append([]string(nil), args...))
	return fakeExec{}
}

func (r *fakeRunner) Preview(args []string) string { return "ai-dev " + strings.Join(args, " ") }

type fakeExec struct{}

func (fakeExec) Run() error          { return nil }
func (fakeExec) SetStdin(io.Reader)  {}
func (fakeExec) SetStdout(io.Writer) {}
func (fakeExec) SetStderr(io.Writer) {}

func lines(texts ...string) []Text {
	var out []Text
	for _, text := range texts {
		out = append(out, Sanitize(text))
	}
	return out
}

func featureRow(id, state string) FeatureRow {
	return FeatureRow{Name: Sanitize(id), ID: id, State: Sanitize(state), Next: Sanitize("step_recommended"),
		Role: Sanitize("specification"), Agent: Sanitize("claude-code"), Availability: Sanitize("available"), Source: Sanitize("built_in"),
		AgentID: "claude-code", Recognized: true, StepRecommended: true, AgentAvailable: true,
		Detail: lines("ai-dev: outcome=state_reported feature=\""+id+"\" state="+state+" terminal=false", "  # run the agent (002)")}
}

func defaultSource() *fakeSource {
	terminal := featureRow("002-done", "approved")
	terminal.Terminal, terminal.StepRecommended, terminal.Next = true, false, Sanitize("workflow_terminal")
	invalid := FeatureRow{Name: Sanitize("003-broken"), ID: "003-broken", State: Sanitize("state_invalid"), Next: Sanitize("state_invalid"),
		Detail: lines("ai-dev: outcome=state_invalid feature=\"003-broken\"")}
	return &fakeSource{
		rows:   []FeatureRow{featureRow("001-alpha", "draft"), terminal, invalid},
		env:    lines("Environment", "  [PASS] Git", "", "Overall: Ready"),
		agents: lines("ai-dev: outcome=not_configured source=built_in", "  agent claude-code availability=available chosen=no"),
		repo:   lines("project:        /tmp/p", "repository:     normal"),
	}
}

var fixedNow = time.Date(2026, 11, 21, 15, 4, 5, 0, time.UTC)

// newLoaded returns a model of size 100×30 with every area loaded.
func newLoaded(t testing.TB, source *fakeSource) (Model, *fakeRunner) {
	t.Helper()
	runner := &fakeRunner{}
	m := New(Config{Context: context.Background(), Source: source, Runner: runner, Now: func() time.Time { return fixedNow }})
	m = drain(t, m, m.Init())
	m, _ = update(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	return m, runner
}

func update(m Model, msg tea.Msg) (Model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

// drain runs cmd and feeds its messages back, except a command handed to
// the terminal, which is never run.
func drain(t testing.TB, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	switch msg := msg.(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			m = drain(t, m, c)
		}
		return m
	case nil, tea.QuitMsg:
		return m
	}
	if isExec(msg) {
		return m
	}
	m, next := update(m, msg)
	return drain(t, m, next)
}

// isExec reports Bubble Tea's internal exec message.
func isExec(msg tea.Msg) bool {
	return fmt.Sprintf("%T", msg) == "tea.execMsg"
}

// key builds a key press from its keystroke name.
func key(name string) tea.KeyPressMsg {
	var mod tea.KeyMod
	base := name
	for {
		switch {
		case strings.HasPrefix(base, "ctrl+"):
			mod |= tea.ModCtrl
			base = strings.TrimPrefix(base, "ctrl+")
			continue
		case strings.HasPrefix(base, "shift+"):
			mod |= tea.ModShift
			base = strings.TrimPrefix(base, "shift+")
			continue
		}
		break
	}
	codes := map[string]rune{"enter": tea.KeyEnter, "tab": tea.KeyTab, "esc": tea.KeyEscape, "up": tea.KeyUp, "down": tea.KeyDown,
		"backspace": tea.KeyBackspace, "left": tea.KeyLeft, "right": tea.KeyRight, "space": tea.KeySpace}
	if code, ok := codes[base]; ok {
		k := tea.KeyPressMsg{Code: code, Mod: mod}
		if base == "space" {
			k.Text = " "
		}
		return k
	}
	r := []rune(base)[0]
	if mod != 0 {
		return tea.KeyPressMsg{Code: r, Mod: mod}
	}
	return tea.KeyPressMsg{Code: r, Text: base}
}

// press sends keys in order and returns the model and the last command.
func press(t testing.TB, m Model, names ...string) (Model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, name := range names {
		m, cmd = update(m, key(name))
		if cmd != nil {
			if msg := cmd(); msg != nil {
				if _, quit := msg.(tea.QuitMsg); !quit && !isExec(msg) {
					m = drain(t, m, func() tea.Msg { return msg })
				}
			}
		}
	}
	return m, cmd
}

// typeText sends each rune as a key press.
func typeText(t testing.TB, m Model, text string) Model {
	t.Helper()
	for _, r := range text {
		m, _ = update(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

func screen(m Model) string { return ansi.Strip(m.View().Content) }

func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

var errBoom = errors.New("boom \x1b]52;c;x\x07")
