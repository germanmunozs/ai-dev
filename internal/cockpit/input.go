package cockpit

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// input is a single-line text input. The value is kept exactly as typed or
// pasted; only its rendering is sanitized.
type input struct {
	runes  []rune
	cursor int
}

func (in *input) value() string { return string(in.runes) }

func (in *input) insert(text string) {
	added := []rune(text)
	in.runes = append(in.runes[:in.cursor], append(added, in.runes[in.cursor:]...)...)
	in.cursor += len(added)
}

// handleKey applies an editing key and reports whether it was one.
func (in *input) handleKey(key tea.KeyPressMsg) bool {
	switch key.String() {
	case "backspace", "ctrl+h":
		if in.cursor > 0 {
			in.runes = append(in.runes[:in.cursor-1], in.runes[in.cursor:]...)
			in.cursor--
		}
	case "delete", "ctrl+d":
		if in.cursor < len(in.runes) {
			in.runes = append(in.runes[:in.cursor], in.runes[in.cursor+1:]...)
		}
	case "left", "ctrl+b":
		in.cursor = max(0, in.cursor-1)
	case "right", "ctrl+f":
		in.cursor = min(len(in.runes), in.cursor+1)
	case "home", "ctrl+a":
		in.cursor = 0
	case "end", "ctrl+e":
		in.cursor = len(in.runes)
	case "ctrl+u":
		in.runes, in.cursor = in.runes[in.cursor:], 0
	default:
		if key.Text == "" || key.Mod.Contains(tea.ModCtrl) || key.Mod.Contains(tea.ModAlt) {
			return false
		}
		in.insert(key.Text)
	}
	return true
}

// render shows the value sanitized, with a visible cursor mark, in at most
// width cells. A longer value scrolls so the cursor stays visible (review
// F3); the value itself is never changed.
func (in *input) render(focused bool, width int) string {
	if !focused {
		return Sanitize(in.value()).String()
	}
	before := Sanitize(string(in.runes[:in.cursor])).String()
	after := Sanitize(string(in.runes[in.cursor:])).String()
	if excess := ansi.StringWidth(before) + 1 - width; width > 1 && excess > 0 {
		before = ansi.TruncateLeft(before, excess+1, "…")
	}
	return before + "▏" + after
}
