package cockpit

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Styles only emphasize: every status and selection is also a word or a
// marker (F012-FR-019).
var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	selectedStyle = lipgloss.NewStyle().Reverse(true)
	faintStyle    = lipgloss.NewStyle().Faint(true)
)

// reviewNotice is the fixed line of every review (contracts/cockpit.md).
const reviewNotice = "This runs the command above in this terminal. Nothing happens until you confirm."

// View renders the cockpit in the alternate screen.
func (m Model) View() tea.View {
	var content string
	switch {
	case m.width == 0:
		content = ""
	case m.tooSmall():
		content = fmt.Sprintf("Terminal too small: %d×%d, need %d×%d.", m.width, m.height, minWidth, minHeight)
	default:
		content = m.render()
	}
	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "ai-dev tui"
	return view
}

func (m Model) render() string {
	header := m.header()
	var body []string
	switch m.mode {
	case helpMode:
		body = m.helpBody()
	case paletteMode:
		body = m.paletteBody()
	case formMode:
		body = m.formBody()
	case reviewMode:
		body = m.reviewBody()
	case runningMode:
		body = []string{"The command is running in this terminal."}
	default:
		body = m.areaBody()
	}
	footer := []string{faintStyle.Render(m.fit(modeKeys[m.mode])), m.fit(m.status.String())}
	height := m.height - len(header) - len(footer)
	lines := make([]string, 0, m.height)
	lines = append(lines, header...)
	for i := 0; i < height; i++ {
		if i < len(body) {
			lines = append(lines, body[i])
		} else {
			lines = append(lines, "")
		}
	}
	lines = append(lines, footer...)
	return strings.Join(lines, "\n")
}

// fit truncates a plain line to the window width.
func (m Model) fit(line string) string { return ansi.Truncate(line, m.width, "…") }

func (m Model) header() []string {
	evaluated := "evaluating…"
	if !m.evaluatedAt.IsZero() && !m.loading() {
		evaluated = "evaluated " + m.evaluatedAt.Format("15:04:05")
	}
	title := titleStyle.Render(m.fit("ai-dev tui · " + areaNames[m.area] + " · " + evaluated))
	var tabs []string
	for id := areaID(0); id < areaCount; id++ {
		if id == m.area {
			tabs = append(tabs, "["+areaNames[id]+"]")
		} else {
			tabs = append(tabs, " "+areaNames[id]+" ")
		}
	}
	return []string{title, m.fit(strings.Join(tabs, " "))}
}

func (m Model) loading() bool {
	if m.featureStatus == loading {
		return true
	}
	for _, id := range []areaID{environmentArea, agentsArea, repositoryArea} {
		if m.lineAreas[id].status == loading {
			return true
		}
	}
	return false
}

func (m Model) areaBody() []string {
	switch m.area {
	case dashboardArea:
		return m.dashboardBody()
	case featuresArea:
		return m.featuresBody()
	case helpArea:
		return m.helpBody()
	}
	return m.linesBody(m.area)
}

func (m Model) dashboardBody() []string {
	row := func(name, status, summary string) string {
		return m.fit(fmt.Sprintf("%-12s %-12s %s", name, status, summary))
	}
	var summary string
	switch m.featureStatus {
	case ready:
		summary = featureSummary(m.features)
	case failed:
		summary = m.featureErr.String()
	}
	out := []string{"", row("Features", m.featureStatus.String(), summary)}
	for _, id := range []areaID{environmentArea, agentsArea, repositoryArea} {
		a := m.lineAreas[id]
		summary = ""
		switch {
		case a.status == failed:
			summary = a.err.String()
		case len(a.lines) > 0 && id == environmentArea:
			summary = lastNonEmpty(a.lines)
		case len(a.lines) > 0:
			summary = a.lines[0].String()
		}
		out = append(out, row(areaNames[id], a.status.String(), summary))
	}
	return append(out, "", m.fit("Tab or Shift-Tab moves between areas; : or Ctrl-P opens the palette; ? shows every key."))
}

// featureSummary counts features by their reported state word.
func featureSummary(rows []FeatureRow) string {
	if len(rows) == 0 {
		return "no features under specs/"
	}
	counts := map[string]int{}
	var order []string
	for _, row := range rows {
		state := row.State.String()
		if counts[state] == 0 {
			order = append(order, state)
		}
		counts[state]++
	}
	slices.Sort(order)
	parts := make([]string, 0, len(order))
	for _, state := range order {
		parts = append(parts, fmt.Sprintf("%d %s", counts[state], state))
	}
	noun := "features"
	if len(rows) == 1 {
		noun = "feature"
	}
	return fmt.Sprintf("%d %s: %s", len(rows), noun, strings.Join(parts, ", "))
}

func lastNonEmpty(lines []Text) string {
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i].String()) != "" {
			return lines[i].String()
		}
	}
	return ""
}

func (m Model) featuresBody() []string {
	switch m.featureStatus {
	case loading:
		return []string{"evaluating…"}
	case failed:
		return []string{"failed: the features cannot be evaluated", m.fit(m.featureErr.String())}
	}
	if len(m.features) == 0 {
		return []string{"No features under specs/.",
			m.fit("A feature is created with Spec Kit (/speckit-specify), which writes specs/NNN-name/spec.md.")}
	}
	visible := m.visibleRows()
	out := []string{m.filterLine(featuresArea)}
	columns := [][]string{{"FEATURE", "STATE", "NEXT", "AGENT", "AVAILABILITY", "SOURCE"}}
	for _, index := range visible {
		row := m.features[index]
		next := row.Next.String()
		columns = append(columns, []string{row.Name.String(), row.State.String(), next,
			row.Agent.String(), row.Availability.String(), row.Source.String()})
	}
	table := alignColumns(columns)
	out = append(out, faintStyle.Render(m.fit("  "+table[0])))
	selected := min(m.cursor[featuresArea], len(visible)-1)
	listHeight := m.height - 4 - 2
	if m.mode == detailMode {
		listHeight = 3
	}
	start := max(0, selected-listHeight+1)
	for i := start; i < len(visible) && i-start < listHeight; i++ {
		line := "  " + table[i+1]
		if i == selected {
			out = append(out, selectedStyle.Render(m.fit("> "+table[i+1])))
			continue
		}
		out = append(out, m.fit(line))
	}
	if len(visible) == 0 {
		out = append(out, "No feature matches the filter.")
	}
	if m.mode == detailMode {
		if row := m.selectedRow(); row != nil {
			out = append(out, "")
			for _, line := range row.Detail {
				out = append(out, m.fit(line.String()))
			}
			out = append(out, m.actionsBody(row)...)
		}
	}
	return out
}

// alignColumns pads each column to its widest cell.
func alignColumns(rows [][]string) []string {
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			widths[i] = max(widths[i], ansi.StringWidth(cell))
		}
	}
	out := make([]string, len(rows))
	for r, row := range rows {
		var line strings.Builder
		for i, cell := range row {
			line.WriteString(cell)
			if i < len(row)-1 {
				line.WriteString(strings.Repeat(" ", widths[i]-ansi.StringWidth(cell)+2))
			}
		}
		out[r] = strings.TrimRight(line.String(), " ")
	}
	return out
}

func (m Model) filterLine(id areaID) string {
	if m.mode == filterMode {
		return m.fit("filter: " + m.filterInput.render(true, m.width-len("filter: ")))
	}
	if m.filters[id] != "" {
		return m.fit("filter: " + Sanitize(m.filters[id]).String() + "  (/ to change, Esc in the filter clears)")
	}
	return ""
}

// actionsBody lists the area's actions with their availability.
func (m Model) actionsBody(row *FeatureRow) []string {
	out := []string{"", titleStyle.Render("Actions")}
	for i, action := range m.areaActions() {
		ok, reason := action.Available(row)
		state := "available"
		if !ok {
			state = "unavailable: " + reason
		}
		line := fmt.Sprintf("%s (%s)", action.Title, state)
		if i == m.actionCursor {
			out = append(out, selectedStyle.Render(m.fit("> "+line)))
		} else {
			out = append(out, m.fit("  "+line))
		}
	}
	return out
}

func (m Model) linesBody(id areaID) []string {
	a := m.lineAreas[id]
	switch a.status {
	case loading:
		return []string{"evaluating…"}
	case failed:
		return []string{"failed: this view cannot be evaluated", m.fit(a.err.String())}
	}
	out := []string{m.filterLine(id)}
	lines := m.visibleLines(id)
	height := m.height - 4 - 1
	if m.mode == detailMode {
		height -= len(m.areaActions()) + 2
	}
	start := min(m.cursor[id], max(len(lines)-1, 0))
	for i := start; i < len(lines) && i-start < height; i++ {
		out = append(out, m.fit(lines[i].String()))
	}
	if m.mode == detailMode {
		out = append(out, m.actionsBody(nil)...)
	} else if len(m.areaActions()) > 0 {
		out = append(out, "", faintStyle.Render(m.fit("Enter: actions for this area")))
	}
	return out
}

func (m Model) helpBody() []string {
	out := []string{titleStyle.Render("Keys")}
	for _, k := range globalKeys {
		out = append(out, m.fit(fmt.Sprintf("  %-16s %s", k.keys, k.does)))
	}
	out = append(out, "", titleStyle.Render("In a review"))
	for _, k := range reviewKeys {
		out = append(out, m.fit(fmt.Sprintf("  %-16s %s", k.keys, k.does)))
	}
	if m.mode == helpMode {
		out = append(out, "", m.fit("Keys of the "+modeNames[m.back]+" mode: "+modeKeys[m.back]))
	}
	return append(out, "", m.fit("Actions run the ai-dev command itself, after you review and confirm it."))
}

func (m Model) paletteBody() []string {
	out := []string{m.fit("> " + m.paletteInput.render(true, m.width-2)), ""}
	for i, entry := range m.paletteEntries() {
		line := entry.title
		if entry.action != nil {
			if entry.available {
				line += " (available)"
			} else {
				line += " (unavailable: " + entry.reason + ")"
			}
		}
		if i == m.paletteCursor {
			out = append(out, selectedStyle.Render(m.fit("> "+line)))
		} else {
			out = append(out, m.fit("  "+line))
		}
	}
	return out
}

func (m Model) formBody() []string {
	out := []string{titleStyle.Render(m.fit(m.form.action.Title))}
	if m.form.action.Feature {
		out = append(out, m.fit("feature: "+m.form.row.Name.String()))
	}
	out = append(out, "")
	for i, field := range m.form.action.Fields {
		label := field.Label
		if field.Required {
			label += " (required)"
		}
		marker := "  "
		if i == m.form.field {
			marker = "> "
		}
		prefix := marker + label + ": "
		out = append(out, m.fit(prefix+m.form.inputs[i].render(i == m.form.field, m.width-ansi.StringWidth(prefix))))
		if hint := m.form.action.hintFor(m.form.row, i); hint != "" {
			out = append(out, faintStyle.Render(m.fit("    "+hint)))
		}
	}
	if m.form.err != "" {
		out = append(out, "", m.fit("refused: "+m.form.err))
	}
	return out
}

func (m Model) reviewBody() []string {
	back, confirm := "[ Back ]", "[ Confirm ]"
	if m.review.confirmFocused {
		confirm = selectedStyle.Render("> [ Confirm ] <")
	} else {
		back = selectedStyle.Render("> [ Back ] <")
	}
	// The command is wrapped, never truncated: the review shows all of it
	// (F012-FR-009, review F3).
	preview := Sanitize(m.cfg.Runner.Preview(m.review.args)).String()
	out := []string{titleStyle.Render(m.fit("Review: " + m.review.action.Title)), "", m.fit("Command:")}
	for _, line := range strings.Split(ansi.Hardwrap(preview, m.width-2, true), "\n") {
		out = append(out, "  "+line)
	}
	return append(out,
		"",
		m.fit("Changes: "+m.review.action.Changes),
		"",
		m.fit(reviewNotice),
		"",
		back+"   "+confirm,
	)
}
