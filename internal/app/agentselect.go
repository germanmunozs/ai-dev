package app

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/germanmunozs/ai-dev/internal/agent"
	"github.com/germanmunozs/ai-dev/internal/report"
	"github.com/germanmunozs/ai-dev/internal/selection"
)

// RunAgentSelect validates and executes the role selection command. It has no
// system.Executor, no system.Launcher, and no context.Context parameter — it
// receives only a preconstructed, read-only agent.Registry, so it has no
// capability to execute a process.
func RunAgentSelect(args []string, registry agent.Registry, stdout, stderr io.Writer) int {
	usage := func() { fmt.Fprintln(stderr, "Usage: ai-dev agent select [--format text|json] <role>") }
	flags := flag.NewFlagSet("agent select", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	format := flags.String("format", "text", "output format: text or json")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 || (*format != "text" && *format != "json") {
		usage()
		return 2
	}

	suppliedRole := flags.Arg(0)
	if strings.TrimSpace(suppliedRole) == "" {
		usage()
		return 2
	}

	selectionReport := selection.NewSelector(registry).Select(suppliedRole)
	var err error
	if *format == "json" {
		err = report.WriteRoleSelectionJSON(stdout, selectionReport)
	} else {
		err = report.WriteRoleSelectionText(stdout, selectionReport)
	}
	if err != nil {
		fmt.Fprintf(stderr, "write role selection report: %v\n", err)
		return 2
	}
	if selectionReport.Outcome == selection.RoleSelected {
		return 0
	}
	return 1
}
