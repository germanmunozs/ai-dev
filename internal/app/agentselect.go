package app

import (
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/germanmunozs/ai-dev/internal/agent"
	"github.com/germanmunozs/ai-dev/internal/report"
	"github.com/germanmunozs/ai-dev/internal/selection"
)

const agentSelectUsage = "Usage: ai-dev agent select [--format text|json] <role>"

// RunAgentSelect validates and executes the role selection command. It has no
// system.Executor, no system.Launcher, and no context.Context parameter — it
// receives only a preconstructed, read-only agent.Registry, so it has no
// capability to execute a process. root is read for the Feature 011 project
// configuration, which selects the role assignment in effect.
func RunAgentSelect(args []string, registry agent.Registry, root fs.FS, stdout, stderr io.Writer) int {
	usage := func() { fmt.Fprintln(stderr, agentSelectUsage) }
	flags := newFlagSet("agent select")
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

	assignment, _, _, _ := loadAssignment(root)
	selectionReport := selection.NewSelectorFor(registry, assignment).Select(suppliedRole)
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
