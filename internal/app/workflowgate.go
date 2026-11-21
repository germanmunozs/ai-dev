package app

import (
	"fmt"
	"io"
	"io/fs"
	"strings"
	"time"

	"github.com/germanmunozs/ai-dev/internal/gate"
	"github.com/germanmunozs/ai-dev/internal/report"
)

type setOnceValue struct {
	value string
	set   bool
}

func (v *setOnceValue) String() string { return v.value }

func (v *setOnceValue) Set(value string) error {
	if v.set {
		return fmt.Errorf("flag set more than once")
	}
	v.value, v.set = value, true
	return nil
}

// workflowGateUsage is the usage line of workflow approve and workflow
// abandon; the command and the command catalog share it.
func workflowGateUsage(decision gate.Decision) string {
	if decision == gate.Abandonment {
		return "Usage: ai-dev workflow abandon [--format text|json] --approver <identity> --reason <text> <feature-id> <target-state>"
	}
	return "Usage: ai-dev workflow approve [--format text|json] --approver <identity> [--reason <text>] <feature-id> <target-state>"
}

// The usage lines of workflow approve and workflow abandon, for the
// command catalog, which does not import gate.
var (
	workflowApproveUsage = workflowGateUsage(gate.Approval)
	workflowAbandonUsage = workflowGateUsage(gate.Abandonment)
)

// RunWorkflowGate validates and executes one explicit Human Gate request.
func RunWorkflowGate(decision gate.Decision, args []string, root fs.FS, store gate.Store, stdout, stderr io.Writer) int {
	return runWorkflowGate(decision, args, root, store, time.Now, stdout, stderr)
}

func runWorkflowGate(decision gate.Decision, args []string, root fs.FS, store gate.Store, now func() time.Time, stdout, stderr io.Writer) int {
	return runWorkflowGateWithEvaluator(decision, args, root, store, now, stdout, stderr, gate.Evaluate)
}

func runWorkflowGateWithEvaluator(decision gate.Decision, args []string, root fs.FS, store gate.Store, now func() time.Time, stdout, stderr io.Writer, evaluate func(fs.FS, gate.Request, gate.Store, func() time.Time) (gate.Report, []string)) int {
	release := brokenPipeAsError()
	defer release()

	verb := "approve"
	if decision == gate.Abandonment {
		verb = "abandon"
	}
	usageText := workflowGateUsage(decision)
	usage := func() { fmt.Fprintln(stderr, usageText) }
	flags := newFlagSet("workflow " + verb)
	format := flags.String("format", "text", "output format")
	var approver, reason setOnceValue
	flags.Var(&approver, "approver", "approver identity")
	flags.Var(&reason, "reason", "decision reason")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 || (*format != "text" && *format != "json") ||
		!approver.set || strings.TrimSpace(approver.value) == "" ||
		(reason.set && strings.TrimSpace(reason.value) == "") ||
		(decision == gate.Abandonment && !reason.set) ||
		(decision != gate.Approval && decision != gate.Abandonment) {
		usage()
		return 2
	}
	featureID, target := flags.Arg(0), flags.Arg(1)
	if strings.TrimSpace(featureID) == "" || strings.TrimSpace(target) == "" {
		usage()
		return 2
	}
	if decision == gate.Abandonment && target != "abandoned" {
		fmt.Fprintln(stderr, "abandonment requires target \"abandoned\"")
		usage()
		return 2
	}
	if decision == gate.Approval && target == "abandoned" {
		fmt.Fprintln(stderr, "use \"ai-dev workflow abandon\" to abandon a feature")
		usage()
		return 2
	}

	request := gate.Request{FeatureID: featureID, Decision: decision, SuppliedTarget: target, Approver: approver.value, Reason: reason.value, ReasonSet: reason.set}
	gateReport, warnings := evaluate(root, request, store, now)
	if !gateReport.Valid() {
		fmt.Fprintf(stderr, "ai-dev: internal error: workflow gate report cannot be rendered for feature %q\n", featureID)
		return 2
	}
	var err error
	if *format == "json" {
		err = report.WriteWorkflowGateJSON(stdout, gateReport)
	} else {
		err = report.WriteWorkflowGateText(stdout, gateReport)
	}
	if err != nil {
		fmt.Fprintf(stderr, "write workflow gate report: %v\n", err)
		return 2
	}
	for _, warning := range warnings {
		fmt.Fprintln(stderr, warning)
	}
	if gateReport.Outcome == gate.TransitionApplied {
		return 0
	}
	return 1
}
