package app

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"ai-dev/internal/report"
	"ai-dev/internal/workflow"
)

// RunWorkflowTransition validates and executes the workflow transition
// command. Usage validation (FR-029 step 1) completes here before
// workflow.EvaluateTransition is called. It receives only a read-only fs.FS —
// no executor, launcher, or registry — and never applies a transition.
func RunWorkflowTransition(args []string, root fs.FS, stdout, stderr io.Writer) int {
	usage := func() {
		fmt.Fprintln(stderr, "Usage: ai-dev workflow transition [--format text|json] <feature-id> <target-state>")
	}
	flags := flag.NewFlagSet("workflow transition", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	format := flags.String("format", "text", "output format: text or json")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 || (*format != "text" && *format != "json") {
		usage()
		return 2
	}

	featureID, suppliedTarget := flags.Arg(0), flags.Arg(1)
	if strings.TrimSpace(featureID) == "" || strings.TrimSpace(suppliedTarget) == "" {
		usage()
		return 2
	}

	transitionReport := workflow.EvaluateTransition(root, featureID, suppliedTarget)
	var err error
	if *format == "json" {
		err = report.WriteWorkflowTransitionJSON(stdout, transitionReport)
	} else {
		err = report.WriteWorkflowTransitionText(stdout, transitionReport)
	}
	if err != nil {
		fmt.Fprintf(stderr, "write workflow transition report: %v\n", err)
		return 2
	}
	if transitionReport.Outcome == workflow.TransitionValid {
		return 0
	}
	return 1
}
