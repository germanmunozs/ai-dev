package app

import (
	"fmt"
	"io"
	"strings"
)

const helpUsage = "Usage: ai-dev help"

// helpCommand is one entry of the help: the command path as typed after
// ai-dev, and what it does. A test runs every path to prove it is routed
// (F009-FR-003).
type helpCommand struct {
	path        []string
	description string
}

var helpCommands = []helpCommand{
	{[]string{"doctor"}, "Check that the environment and the repository are ready"},
	{[]string{"agents"}, "List the supported coding agents and their local availability"},
	{[]string{"agent", "select"}, "Show which coding agent a development role maps to"},
	{[]string{"agent", "run"}, "Run a coding agent with an instruction (argument or stdin)"},
	{[]string{"workflow", "state"}, "Report a feature's Workflow State"},
	{[]string{"workflow", "transition"}, "Check whether a Workflow State transition is valid"},
	{[]string{"workflow", "next"}, "Recommend the next Spec-Driven Development step"},
	{[]string{"workflow", "guide"}, "Guide the next human actions, with command templates (read-only)"},
	{[]string{"workflow", "approve"}, "Record a human approval and apply its transition"},
	{[]string{"workflow", "abandon"}, "Record a human abandonment"},
	{[]string{"init"}, "Initialize Git interactively (needs a terminal)"},
	{[]string{"init", "inspect"}, "Inspect the project and its repository"},
	{[]string{"init", "plan"}, "Write a deterministic Git initialization plan"},
	{[]string{"init", "apply"}, "Apply a confirmed plan under a human authorization"},
	{[]string{"init", "reconcile"}, "Report the reconciliation of an interrupted apply"},
	{[]string{"git", "commit", "validate"}, "Validate a Conventional Commits message"},
	{[]string{"version"}, "Print which build of ai-dev is running"},
	{[]string{"help"}, "Print this help"},
}

// RunHelp writes the help to standard output (F009-FR-003).
func RunHelp(args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, helpUsage)
		return 2
	}
	width := 0
	for _, command := range helpCommands {
		width = max(width, len(strings.Join(command.path, " ")))
	}
	var out strings.Builder
	out.WriteString("ai-dev: spec-driven development with coding agents, under explicit human authority.\n\n")
	out.WriteString("Usage: ai-dev <command> [options]\n\nCommands:\n")
	for _, command := range helpCommands {
		fmt.Fprintf(&out, "  %-*s  %s\n", width, strings.Join(command.path, " "), command.description)
	}
	out.WriteString("\nRun a command with missing or invalid arguments to see its exact usage.\n")
	out.WriteString("Most commands accept --format text|json.\n")
	out.WriteString("Exit codes: 0 positive result, 1 negative domain result, 2 usage or internal failure.\n")
	out.WriteString("Nothing is approved, transitioned or committed without an explicit human decision.\n")
	if _, err := io.WriteString(stdout, out.String()); err != nil {
		fmt.Fprintf(stderr, "write help: %v\n", err)
		return 2
	}
	return 0
}
