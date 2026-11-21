package app

import (
	"fmt"
	"io"

	"github.com/germanmunozs/ai-dev/internal/agent"
	"github.com/germanmunozs/ai-dev/internal/report"
	"github.com/germanmunozs/ai-dev/internal/system"
)

const agentsUsage = "Usage: ai-dev agents [--format text|json]"

// RunAgentsList validates and executes the supported-agent listing command.
func RunAgentsList(args []string, executor system.Executor, stdout, stderr io.Writer) int {
	usage := func() { fmt.Fprintln(stderr, agentsUsage) }
	flags := newFlagSet("agents")
	format := flags.String("format", "text", "output format: text or json")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || (*format != "text" && *format != "json") {
		usage()
		return 2
	}

	statuses := agent.NewRegistry(executor).List()
	var err error
	if *format == "json" {
		err = report.WriteAgentListJSON(stdout, statuses)
	} else {
		err = report.WriteAgentListText(stdout, statuses)
	}
	if err != nil {
		fmt.Fprintf(stderr, "write agent listing: %v\n", err)
		return 2
	}
	return 0
}
