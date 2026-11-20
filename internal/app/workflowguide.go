package app

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"ai-dev/internal/agent"
	"ai-dev/internal/guide"
	"ai-dev/internal/report"
	"ai-dev/internal/sdd"
	"ai-dev/internal/selection"
	"ai-dev/internal/system"
)

// RunWorkflowGuide evaluates and reports one read-only guidance request. The
// executor is reached only through the 003 selector's registry, which uses
// executable lookup alone; guidance starts no process and reads no stdin.
func RunWorkflowGuide(args []string, root fs.FS, executor system.Executor, stdout, stderr io.Writer) int {
	return runWorkflowGuide(args, root, stdout, stderr, func(root fs.FS, featureID string) guide.Report {
		return guide.Evaluate(root, featureID, selection.NewSelector(agent.NewRegistry(executor)))
	})
}

func runWorkflowGuide(args []string, root fs.FS, stdout, stderr io.Writer, evaluate func(fs.FS, string) guide.Report) int {
	release := brokenPipeAsError()
	defer release()

	usage := func() { fmt.Fprintln(stderr, "Usage: ai-dev workflow guide [--format text|json] <feature-id>") }
	flags := flag.NewFlagSet("workflow guide", flag.ContinueOnError)
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

	guideReport := evaluate(root, featureID)
	if !guideReport.Valid() {
		fmt.Fprintf(stderr, "ai-dev: internal error: workflow guide report cannot be rendered for feature %q\n", featureID)
		return 2
	}
	var err error
	if *format == "json" {
		err = report.WriteGuideJSON(stdout, guideReport)
	} else {
		err = report.WriteGuideText(stdout, guideReport)
	}
	if err != nil {
		fmt.Fprintf(stderr, "write workflow guide report: %v\n", err)
		return 2
	}
	if guideReport.Outcome == sdd.StepRecommended || guideReport.Outcome == sdd.WorkflowTerminal {
		return 0
	}
	return 1
}
