package app

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/germanmunozs/ai-dev/internal/gitmanager"
	"github.com/germanmunozs/ai-dev/internal/report"
)

// InspectGitReconciliation composes the read-only reconciliation
// inspection (T105): it opens the store at the canonical state root,
// reads the prior authorization and its attempt trail, and classifies the
// prior plan's postconditions on a fresh inspection. It writes nothing.
//
// Continuing is a new plan applied with --issue-authorization
// --reconciles-authorization naming the prior authorization (REC-04).
func InspectGitReconciliation(ctx context.Context, deps GitManagementDependencies, planFile []byte, authorizationID string) (gitmanager.ReconciliationReport, *gitmanager.PlanRejection, error) {
	if deps.OpenStore == nil || deps.LookupEnv == nil {
		//lint:ignore ST1005 "Git" is a proper noun, and the text is the CLI's existing output.
		return gitmanager.ReconciliationReport{}, nil, fmt.Errorf("Git reconciliation is unavailable in this build")
	}
	stateRoot, err := gitmanager.ResolveStateRoot(deps.GOOS, deps.LookupEnv)
	if err != nil {
		return gitmanager.ReconciliationReport{}, nil, err
	}
	store, closeStore, err := deps.OpenStore(stateRoot)
	if err != nil {
		return gitmanager.ReconciliationReport{}, nil, fmt.Errorf("the Git state store is unavailable: %v", err)
	}
	defer closeStore()
	report, rejection := gitmanager.InspectReconciliation(ctx, gitmanager.ReconciliationDependencies{
		Store: store,
		Inspect: func(ctx context.Context, projectPath []byte, candidates [][]byte) (gitmanager.GitInspection, error) {
			inspection, _, err := inspectWithEnvironment(ctx, deps, string(projectPath), candidates)
			return inspection, err
		},
	}, gitmanager.ReconciliationRequest{PlanFile: planFile, AuthorizationID: authorizationID})
	return report, rejection, nil
}

const gitReconcileUsage = "Usage: ai-dev init reconcile --plan FILE --authorization-id UUID [--format text|json]"

// RunGitReconcile implements "ai-dev init reconcile" (REC-04): the
// read-only steps 1–2 of supervised reconciliation. It creates no
// authorization, transitions nothing and runs no mutation.
func RunGitReconcile(ctx context.Context, args []string, deps GitManagementDependencies, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("init reconcile", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	planPath := flags.String("plan", "", "")
	authorizationID := flags.String("authorization-id", "", "")
	format := flags.String("format", "text", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || (*format != "text" && *format != "json") || *planPath == "" || *authorizationID == "" {
		fmt.Fprintln(stderr, gitReconcileUsage)
		return 2
	}
	planFile, err := readPlanFile(*planPath)
	if err != nil {
		fmt.Fprintf(stderr, "ai-dev: init reconcile: %v\n", err)
		return 2
	}
	result, rejection, err := InspectGitReconciliation(ctx, deps, planFile, *authorizationID)
	switch {
	case err != nil:
		fmt.Fprintf(stderr, "ai-dev: init reconcile: %v\n", err)
		return 1
	case rejection != nil:
		if err := report.WriteGitPlanRejection(stdout, *format, rejection, nil); err != nil {
			fmt.Fprintf(stderr, "ai-dev: init reconcile: output failed: %v\n", err)
			return 2
		}
		return rejection.ExitCode()
	}
	if *format == "json" {
		err = report.WriteGitReconciliationJSON(stdout, result)
	} else {
		err = report.WriteGitReconciliationText(stdout, result)
	}
	if err != nil {
		fmt.Fprintf(stderr, "ai-dev: init reconcile: output failed: %v\n", err)
		return 2
	}
	return 0
}
