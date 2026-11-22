package cockpit

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Config composes a Model. Context must never be cancelled by an interrupt
// meant for a command in the foreground (R-04).
type Config struct {
	Context context.Context
	Source  Source
	Runner  Runner
	Now     func() time.Time
}

// areaID is one area (F012-FR-004), in Tab order.
type areaID int

const (
	dashboardArea areaID = iota
	featuresArea
	environmentArea
	agentsArea
	repositoryArea
	helpArea
	areaCount
)

var areaNames = [areaCount]string{"Dashboard", "Features", "Environment", "Agents", "Repository", "Help"}

// mode is the interaction state (data-model.md). "too small" is not a mode:
// it overlays any mode and confirms nothing.
type mode int

const (
	browseMode mode = iota
	detailMode
	filterMode
	paletteMode
	formMode
	reviewMode
	runningMode
	helpMode
)

var modeNames = map[mode]string{browseMode: "browse", detailMode: "detail", filterMode: "filter", paletteMode: "palette",
	formMode: "form", reviewMode: "review", runningMode: "running", helpMode: "help"}

const minWidth, minHeight = 80, 24

// Model is the cockpit's Bubble Tea model. It holds view state only; every
// fact comes from the Source and every action goes through the Runner.
type Model struct {
	cfg           Config
	width, height int
	area          areaID
	mode          mode
	// back is the mode Esc returns to from help, the palette, a form or
	// a review.
	back mode

	generation  int
	evaluatedAt time.Time

	featureStatus loadStatus
	features      []FeatureRow
	featureErr    Text
	lineAreas     [areaCount]area

	cursor  [areaCount]int // selected row (Features) or scroll (others)
	filters [areaCount]string
	// selectedID keeps the selected feature across a reload (F012-FR-015).
	selectedID string

	filterInput   input
	actionCursor  int
	paletteInput  input
	paletteCursor int

	form   formState
	review reviewState

	status      Text
	pendingTerm bool
	exitCode    int
}

type formState struct {
	action Action
	row    FeatureRow
	inputs []input
	field  int
	err    string
}

type reviewState struct {
	action         Action
	args           []string
	confirmFocused bool
	// fromForm: Esc and Back return to the form, not to the list.
	fromForm bool
}

// New returns a model whose Init evaluates every area.
func New(cfg Config) Model {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Context == nil {
		cfg.Context = context.Background()
	}
	return Model{cfg: cfg, area: dashboardArea}
}

// ExitCode is the process exit code once the program has quit:
// 0 for q and Ctrl-C, 130 for an interrupt signal, 143 for termination.
func (m Model) ExitCode() int { return m.exitCode }

// AreaName names the current area.
func (m Model) AreaName() string { return areaNames[m.area] }

// Mode names the current mode, for tests and diagnostics.
func (m Model) Mode() string { return modeNames[m.mode] }

// SelectedFeature is the raw identifier of the selected feature, or "".
func (m Model) SelectedFeature() string {
	if row := m.selectedRow(); row != nil {
		return row.ID
	}
	return ""
}

// Init evaluates every area concurrently (F012-FR-006).
func (m Model) Init() tea.Cmd {
	return m.loadCmds(1)
}

type featuresLoadedMsg struct {
	generation int
	rows       []FeatureRow
	err        error
}

type linesLoadedMsg struct {
	generation int
	area       areaID
	lines      []Text
	err        error
}

// execDoneMsg is the callback of a command handed to the terminal.
type execDoneMsg struct{ err error }

func (m Model) loadCmds(generation int) tea.Cmd {
	ctx, source := m.cfg.Context, m.cfg.Source
	lineCmd := func(id areaID, load func(context.Context) ([]Text, error)) tea.Cmd {
		return func() tea.Msg {
			lines, err := load(ctx)
			return linesLoadedMsg{generation: generation, area: id, lines: lines, err: err}
		}
	}
	return tea.Batch(
		func() tea.Msg {
			rows, err := source.LoadFeatures(ctx)
			return featuresLoadedMsg{generation: generation, rows: rows, err: err}
		},
		lineCmd(environmentArea, source.LoadEnvironment),
		lineCmd(agentsArea, source.LoadAgents),
		lineCmd(repositoryArea, source.LoadRepository),
	)
}

// reload starts a new evaluation. Every area drops its previous data, so
// nothing stale is shown as current.
func (m Model) reload() (Model, tea.Cmd) {
	if row := m.selectedRow(); row != nil {
		m.selectedID = row.ID
	}
	m.generation++
	m.evaluatedAt = m.cfg.Now()
	m.featureStatus, m.features, m.featureErr = loading, nil, Text{}
	for i := range m.lineAreas {
		m.lineAreas[i] = area{}
	}
	return m, m.loadCmds(m.generation + 1)
}

func (m Model) tooSmall() bool { return m.width < minWidth || m.height < minHeight }

// Update handles one message.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case featuresLoadedMsg:
		if msg.generation != m.generation+1 {
			return m, nil
		}
		if m.evaluatedAt.IsZero() {
			m.evaluatedAt = m.cfg.Now()
		}
		if msg.err != nil {
			m.featureStatus, m.features, m.featureErr = failed, nil, Sanitize(msg.err.Error())
		} else {
			m.featureStatus, m.features = ready, msg.rows
			m.restoreSelection()
		}
		return m, nil
	case linesLoadedMsg:
		if msg.generation != m.generation+1 {
			return m, nil
		}
		if msg.err != nil {
			m.lineAreas[msg.area] = area{status: failed, err: Sanitize(msg.err.Error())}
		} else {
			m.lineAreas[msg.area] = area{status: ready, lines: msg.lines}
		}
		return m, nil
	case SignalMsg:
		return m.handleSignal(msg)
	case execDoneMsg:
		return m.commandReturned(msg.err)
	case tea.PasteMsg:
		return m.handlePaste(msg.Content), nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// handleSignal applies R-04: while a command runs, an interrupt is the
// command's and termination waits for it to return.
func (m Model) handleSignal(msg SignalMsg) (tea.Model, tea.Cmd) {
	if m.mode == runningMode {
		if msg.Terminate {
			m.pendingTerm = true
		}
		return m, nil
	}
	if msg.Terminate {
		return m.quit(143)
	}
	return m.quit(130)
}

func (m Model) quit(code int) (tea.Model, tea.Cmd) {
	m.exitCode = code
	return m, tea.Quit
}

func (m Model) commandReturned(err error) (tea.Model, tea.Cmd) {
	if m.mode != runningMode {
		return m, nil
	}
	var exit CommandExit
	switch {
	case err == nil:
		m.status = Sanitize("The command exited with code 0.")
	case errors.As(err, &exit):
		m.status = Sanitize("The command " + strings.TrimPrefix(exit.Error(), "the command ") + ".")
	default:
		m.status = Sanitize("The command could not be run: " + err.Error())
	}
	m.mode = m.back
	if m.pendingTerm {
		return m.quit(143)
	}
	return m.reload()
}

func (m Model) handlePaste(content string) Model {
	switch m.mode {
	case formMode:
		m.form.inputs[m.form.field].insert(content)
	case filterMode:
		m.filterInput.insert(content)
		m.filters[m.area] = m.filterInput.value()
	case paletteMode:
		m.paletteInput.insert(content)
		m.paletteCursor = 0
	}
	return m
}

func (m Model) handleKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	name := k.String()
	if m.mode == runningMode {
		return m, nil // the terminal belongs to the command
	}
	if name == "ctrl+c" {
		return m.quit(0)
	}
	if m.width == 0 || m.tooSmall() {
		return m, nil
	}
	switch m.mode {
	case formMode:
		return m.formKey(k)
	case reviewMode:
		return m.reviewKey(name)
	case filterMode:
		return m.filterKey(k), nil
	case paletteMode:
		return m.paletteKey(k)
	case helpMode:
		if name == "esc" || name == "?" || name == "q" {
			m.mode = m.back
		}
		return m, nil
	case detailMode:
		return m.detailKey(name)
	}
	return m.browseKey(name)
}

func (m Model) browseKey(name string) (tea.Model, tea.Cmd) {
	switch name {
	case "q":
		return m.quit(0)
	case "tab":
		m.area = (m.area + 1) % areaCount
	case "shift+tab":
		m.area = (m.area + areaCount - 1) % areaCount
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "enter":
		if len(m.areaActions()) > 0 && (m.area != featuresArea || m.selectedRow() != nil) {
			m.mode, m.actionCursor = detailMode, 0
		}
	case "/":
		if m.area != dashboardArea && m.area != helpArea {
			m.mode = filterMode
			m.filterInput = input{runes: []rune(m.filters[m.area]), cursor: len([]rune(m.filters[m.area]))}
		}
	case ":", "ctrl+p":
		m.openPalette()
	case "?":
		m.back, m.mode = m.mode, helpMode
	case "r":
		return m.reload()
	}
	return m, nil
}

func (m Model) detailKey(name string) (tea.Model, tea.Cmd) {
	actions := m.areaActions()
	switch name {
	case "esc":
		m.mode = browseMode
	case "j", "down":
		m.actionCursor = min(m.actionCursor+1, len(actions)-1)
	case "k", "up":
		m.actionCursor = max(m.actionCursor-1, 0)
	case "enter":
		if m.actionCursor < len(actions) {
			return m.startAction(actions[m.actionCursor], detailMode), nil
		}
	case ":", "ctrl+p":
		m.openPalette()
	case "?":
		m.back, m.mode = m.mode, helpMode
	case "r":
		return m.reload()
	}
	return m, nil
}

func (m Model) filterKey(k tea.KeyPressMsg) Model {
	switch k.String() {
	case "esc":
		m.filters[m.area] = ""
		m.mode = browseMode
	case "enter":
		m.mode = browseMode
	default:
		if m.filterInput.handleKey(k) {
			m.filters[m.area] = m.filterInput.value()
			m.cursor[m.area] = 0
		}
	}
	return m
}

func (m *Model) openPalette() {
	m.back, m.mode = m.mode, paletteMode
	m.paletteInput, m.paletteCursor = input{}, 0
}

// paletteEntry is one palette line: an area or an action.
type paletteEntry struct {
	title     string
	area      areaID
	action    *Action
	available bool
	reason    string
}

func (m Model) paletteEntries() []paletteEntry {
	query := strings.ToLower(m.paletteInput.value())
	var entries []paletteEntry
	for id := areaID(0); id < areaCount; id++ {
		entries = append(entries, paletteEntry{title: "Go to " + areaNames[id], area: id, available: true})
	}
	row := m.selectedRow()
	for _, action := range Actions() {
		ok, reason := action.Available(row)
		entries = append(entries, paletteEntry{title: action.Title, action: &action, available: ok, reason: reason})
	}
	var matches []paletteEntry
	for _, entry := range entries {
		if strings.Contains(strings.ToLower(entry.title), query) {
			matches = append(matches, entry)
		}
	}
	return matches
}

func (m Model) paletteKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	entries := m.paletteEntries()
	switch k.String() {
	case "esc":
		m.mode = m.back
	case "down", "ctrl+n":
		m.paletteCursor = max(0, min(m.paletteCursor+1, len(entries)-1))
	case "up":
		m.paletteCursor = max(m.paletteCursor-1, 0)
	case "enter":
		if m.paletteCursor < 0 || m.paletteCursor >= len(entries) {
			return m, nil
		}
		entry := entries[m.paletteCursor]
		if entry.action == nil {
			m.area, m.mode = entry.area, browseMode
			return m, nil
		}
		if !entry.available {
			m.status = Sanitize(entry.title + " is unavailable: " + entry.reason)
			return m, nil
		}
		return m.startAction(*entry.action, browseMode), nil
	default:
		if m.paletteInput.handleKey(k) {
			m.paletteCursor = 0
		}
	}
	return m, nil
}

// startAction opens the form, or the review when the action needs no
// value. It re-checks availability, so no path starts an unavailable one.
func (m Model) startAction(action Action, back mode) Model {
	var row FeatureRow
	selected := m.selectedRow()
	if action.Feature {
		if ok, reason := action.Available(selected); !ok {
			m.status = Sanitize(action.Title + " is unavailable: " + reason)
			return m
		}
		row = *selected
	}
	m.back = back
	if len(action.Fields) == 0 {
		m.review = reviewState{action: action, args: action.Argv(row, nil)}
		m.mode = reviewMode
		return m
	}
	m.form = formState{action: action, row: row, inputs: make([]input, len(action.Fields))}
	m.mode = formMode
	return m
}

func (m Model) formKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	last := len(m.form.inputs) - 1
	switch k.String() {
	case "esc":
		m.mode = m.back
	case "tab", "down":
		m.form.field = min(m.form.field+1, last)
	case "shift+tab", "up":
		m.form.field = max(m.form.field-1, 0)
	case "enter":
		if m.form.field < last {
			m.form.field++
			return m, nil
		}
		values := make([]string, len(m.form.inputs))
		for i := range m.form.inputs {
			values[i] = m.form.inputs[i].value()
		}
		if refusal := m.form.action.Validate(values); refusal != "" {
			m.form.err = refusal
			return m, nil
		}
		m.form.err = ""
		m.review = reviewState{action: m.form.action, args: m.form.action.Argv(m.form.row, values), fromForm: true}
		m.mode = reviewMode
	default:
		m.form.inputs[m.form.field].handleKey(k)
	}
	return m, nil
}

// reviewKey: only Tab focuses Confirm and only Enter on it runs the
// command, once (F012-FR-009, FR-012, R-06).
func (m Model) reviewKey(name string) (tea.Model, tea.Cmd) {
	switch name {
	case "tab":
		m.review.confirmFocused = true
	case "shift+tab":
		m.review.confirmFocused = false
	case "esc":
		m.leaveReview()
	case "enter":
		if !m.review.confirmFocused {
			m.leaveReview()
			return m, nil
		}
		args := m.review.args
		m.review = reviewState{}
		m.mode = runningMode
		m.status = Text{}
		return m, tea.Exec(m.cfg.Runner.Command(args), func(err error) tea.Msg { return execDoneMsg{err: err} })
	}
	return m, nil
}

func (m *Model) leaveReview() {
	if m.review.fromForm {
		m.mode = formMode
	} else {
		m.mode = m.back
	}
	m.review = reviewState{}
}

// areaActions are the actions the current area offers in its detail.
func (m Model) areaActions() []Action {
	var ids []ActionID
	switch m.area {
	case featuresArea:
		ids = []ActionID{RunAgent, Approve, Abandon}
	case agentsArea:
		ids = []ActionID{Setup}
	case repositoryArea:
		ids = []ActionID{Init, CommitValidate}
	}
	var out []Action
	for _, id := range ids {
		out = append(out, ActionByID(id))
	}
	return out
}

// visibleRows are the indexes of the features matching the filter.
func (m Model) visibleRows() []int {
	query := strings.ToLower(m.filters[featuresArea])
	var out []int
	for i, row := range m.features {
		if strings.Contains(strings.ToLower(row.Name.String()), query) {
			out = append(out, i)
		}
	}
	return out
}

func (m Model) selectedRow() *FeatureRow {
	visible := m.visibleRows()
	if len(visible) == 0 {
		return nil
	}
	i := min(m.cursor[featuresArea], len(visible)-1)
	return &m.features[visible[i]]
}

func (m *Model) restoreSelection() {
	visible := m.visibleRows()
	m.cursor[featuresArea] = min(m.cursor[featuresArea], max(len(visible)-1, 0))
	for i, index := range visible {
		if m.features[index].ID == m.selectedID {
			m.cursor[featuresArea] = i
		}
	}
}

// visibleLines are the lines of a line area matching its filter.
func (m Model) visibleLines(id areaID) []Text {
	query := strings.ToLower(m.filters[id])
	if query == "" {
		return m.lineAreas[id].lines
	}
	var out []Text
	for _, line := range m.lineAreas[id].lines {
		if strings.Contains(strings.ToLower(line.String()), query) {
			out = append(out, line)
		}
	}
	return out
}

func (m *Model) move(delta int) {
	var count int
	switch m.area {
	case featuresArea:
		count = len(m.visibleRows())
	case environmentArea, agentsArea, repositoryArea:
		count = len(m.visibleLines(m.area))
	default:
		return
	}
	m.cursor[m.area] = max(0, min(m.cursor[m.area]+delta, count-1))
	if row := m.selectedRow(); row != nil && m.area == featuresArea {
		m.selectedID = row.ID
	}
}
