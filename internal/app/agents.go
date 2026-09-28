package app

import (
	"flag"
	"fmt"
	"io"

	"ai-dev/internal/agent"
	"ai-dev/internal/report"
	"ai-dev/internal/system"
)

// RunAgentsList validates and executes the supported-agent listing command.
func RunAgentsList(args []string, executor system.Executor, stdout, stderr io.Writer) int {
	usage := func() { fmt.Fprintln(stderr, "Usage: ai-dev agents [--format text|json]") }
	flags := flag.NewFlagSet("agents", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
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
