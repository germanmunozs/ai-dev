package cockpit

// keyHelp is one key and what it does (F012-FR-013).
type keyHelp struct{ keys, does string }

// globalKeys are the default keymap; none approves, abandons or starts an
// agent by itself.
var globalKeys = []keyHelp{
	{"Tab / Shift-Tab", "next or previous area"},
	{"j/k or ↑/↓", "move"},
	{"Enter", "open the selected item or choose the selected action"},
	{"/", "filter the current list (Esc clears)"},
	{": or Ctrl-P", "palette: every area and action"},
	{"?", "this help"},
	{"r", "refresh every view"},
	{"Esc", "back or close"},
	{"q", "quit (from the top level)"},
	{"Ctrl-C", "quit (anywhere except while a command runs)"},
}

// modeKeys are the keys the footer shows for each mode.
var modeKeys = map[mode]string{
	browseMode:  "Tab area · j/k move · Enter open · / filter · : palette · ? help · r refresh · q quit",
	detailMode:  "j/k choose · Enter start · Esc back · : palette · ? help",
	filterMode:  "type to filter · Enter keep · Esc clear",
	paletteMode: "type to search · ↑/↓ choose · Enter open · Esc close",
	formMode:    "type the value · Tab/↑/↓ field · Enter next or review · Esc back",
	reviewMode:  "Tab focuses Confirm · Enter presses the focused button · Esc back",
	runningMode: "the command owns the terminal",
	helpMode:    "Esc close",
}

// reviewKeys explain the review in the help.
var reviewKeys = []keyHelp{
	{"Tab", "focus the Confirm button"},
	{"Shift-Tab", "focus the Back button"},
	{"Enter", "press the focused button; only Confirm runs the command"},
}
