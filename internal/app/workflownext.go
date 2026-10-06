package app

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"ai-dev/internal/report"
	"ai-dev/internal/sdd"
)

// RunWorkflowNext evaluates and reports one read-only SDD next-step request.
func RunWorkflowNext(args []string, root fs.FS, stdout, stderr io.Writer) int {
	return runWorkflowNext(args, root, stdout, stderr, sdd.Evaluate)
}

func runWorkflowNext(args []string, root fs.FS, stdout, stderr io.Writer, evaluate func(fs.FS, string) sdd.Report) int {
	release := brokenPipeAsError()
	defer release()

	usage := func() { fmt.Fprintln(stderr, "Usage: ai-dev workflow next [--format text|json] <feature-id>") }
	flags := flag.NewFlagSet("workflow next", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	format := flags.String("format", "text", "output format: text or json")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 || (*format != "text" && *format != "json") {
		usage()
		return 2
	}
	featureID := flags.Arg(0)
	if strings.TrimSpace(featureID) == "" {
		usage()
		return 2
	}

	nextReport := evaluate(root, featureID)
	if !nextReport.Valid() {
		fmt.Fprintf(stderr, "ai-dev: internal error: workflow next report cannot be rendered for feature %q\n", featureID)
		return 2
	}
	var err error
	if *format == "json" {
		err = report.WriteWorkflowNextJSON(stdout, nextReport)
	} else {
		err = report.WriteWorkflowNextText(stdout, nextReport)
	}
	if err != nil {
		fmt.Fprintf(stderr, "write workflow next report: %v\n", err)
		return 2
	}
	if nextReport.Outcome == sdd.StepRecommended || nextReport.Outcome == sdd.WorkflowTerminal {
		return 0
	}
	return 1
}
