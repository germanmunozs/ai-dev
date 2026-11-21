package app

import (
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/germanmunozs/ai-dev/internal/report"
	"github.com/germanmunozs/ai-dev/internal/workflow"
)

const workflowStateUsage = "Usage: ai-dev workflow state [--format text|json] <feature-id>"

// RunWorkflowState validates and executes the workflow state command. It
// receives only a read-only fs.FS — no executor, launcher, or registry — so
// it has no capability to mutate the filesystem or execute a process.
func RunWorkflowState(args []string, root fs.FS, stdout, stderr io.Writer) int {
	usage := func() { fmt.Fprintln(stderr, workflowStateUsage) }
	flags := newFlagSet("workflow state")
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

	stateReport := workflow.EvaluateState(root, featureID)
	var err error
	if *format == "json" {
		err = report.WriteWorkflowStateJSON(stdout, stateReport)
	} else {
		err = report.WriteWorkflowStateText(stdout, stateReport)
	}
	if err != nil {
		fmt.Fprintf(stderr, "write workflow state report: %v\n", err)
		return 2
	}
	if stateReport.Outcome == workflow.StateReported {
		return 0
	}
	return 1
}
