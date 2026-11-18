package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"ai-dev/internal/gitmanager"
	"ai-dev/internal/report"
)

const gitPlanUsage = "Usage: ai-dev init plan --project-dir PATH --strategy basic|git-flow|github-flow|trunk-based [options] --output FILE [--format text|json]"

// initialCommitOptions belong to the initial-commit group, which is planned
// together with its allowlist and preflight (T083–T089); until then they
// are refused explicitly rather than ignored.
var initialCommitOptions = []string{
	"initial-commit", "initial-commit-message-file", "include", "author-name", "author-email", "committer-name",
	"committer-email", "author-date", "committer-date",
}

// RunGitPlan implements "ai-dev init plan". It inspects, plans and exports
// the canonical plan as one explicit external write; it never mutates the
// project and never grants authorization.
func RunGitPlan(ctx context.Context, args []string, deps GitManagementDependencies, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("init plan", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	projectDir := flags.String("project-dir", ".", "")
	strategy := flags.String("strategy", "", "")
	initialize := flags.Bool("initialize-repository", false, "")
	dialect := flags.String("git-flow-dialect", "", "")
	base := flags.String("development-base-oid", "", "")
	output := flags.String("output", "", "")
	format := flags.String("format", "text", "")
	branches := map[string]*string{}
	for _, name := range []string{"initial-branch", "production-branch", "development-branch", "trunk-branch"} {
		branches[name] = flags.String(name, "", "")
	}
	prefixes := map[string]*string{}
	for _, name := range []string{"feature", "bugfix", "release", "hotfix", "support", "version-tag"} {
		prefixes[name] = flags.String(name+"-prefix", "", "")
	}
	for _, name := range initialCommitOptions {
		flags.String(name, "", "")
	}
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || (*format != "text" && *format != "json") {
		fmt.Fprintln(stderr, gitPlanUsage)
		return 2
	}
	given := map[string]bool{}
	flags.Visit(func(f *flag.Flag) { given[f.Name] = true })
	for _, name := range initialCommitOptions {
		if given[name] {
			fmt.Fprintf(stderr, "ai-dev: init plan: --%s is not available yet: the initial-commit group is planned with its allowlist and preflight\n", name)
			return 2
		}
	}
	if *output == "" {
		fmt.Fprintln(stderr, gitPlanUsage)
		return 2
	}
	refuse := func(rejection *gitmanager.PlanRejection, warnings []string) int {
		if err := report.WriteGitPlanRejection(stdout, *format, rejection, warnings); err != nil {
			fmt.Fprintf(stderr, "ai-dev: init plan: output failed: %v\n", err)
			return 2
		}
		return rejection.ExitCode()
	}

	req := gitmanager.StrategyRequest{
		Strategy: *strategy, InitializeRepository: *initialize, GitFlowDialect: *dialect, DevelopmentBaseOID: *base,
		Branches: map[string]string{}, Prefixes: map[string]string{},
	}
	for name, value := range branches {
		if given[name] {
			req.Branches[name] = *value
		}
	}
	for name, value := range prefixes {
		if given[name+"-prefix"] {
			key := name
			if name == "version-tag" {
				key = "version_tag"
			}
			req.Prefixes[key] = *value
		}
	}
	if deps.Commands == nil || deps.RandomSource == nil {
		fmt.Fprintln(stderr, "ai-dev: init plan: Git planning is unavailable in this build")
		return 1
	}
	inspection, environment, err := inspectWithEnvironment(ctx, deps, *projectDir)
	if err != nil {
		var inspectErr *gitmanager.InspectError
		if errors.As(err, &inspectErr) {
			return refuse(&gitmanager.PlanRejection{Reason: inspectErr.Reason, Detail: inspectErr.Detail}, nil)
		}
		fmt.Fprintf(stderr, "ai-dev: init plan: %v\n", err)
		return 1
	}
	planID, err := newInspectionID()
	if err != nil {
		fmt.Fprintf(stderr, "ai-dev: init plan: %v\n", err)
		return 1
	}
	now := time.Now
	if deps.Clock != nil {
		now = deps.Clock.Now
	}
	planner := gitmanager.PlannerDependencies{Git: deps.Reader, Commands: deps.Commands, Random: deps.RandomSource}
	result, rejection := gitmanager.Plan(ctx, planner, req, inspection, environment,
		gitmanager.PlanMetadata{PlanID: gitmanager.PlanID(planID), GeneratedAt: now().UTC().Truncate(time.Second)})
	if rejection != nil {
		return refuse(rejection, inspection.Warnings())
	}
	data, err := gitmanager.EncodePlanFile(result.Plan)
	if err != nil {
		return refuse(&gitmanager.PlanRejection{Reason: gitmanager.ReasonPlanInvalid, Detail: err.Error()}, nil)
	}
	workingDir, err := deps.WorkingDir()
	if err != nil {
		fmt.Fprintf(stderr, "ai-dev: init plan: %v\n", err)
		return 1
	}
	written, rejection := gitmanager.WritePlanOutput(gitmanager.PlanOutputRequest{
		Path: *output, WorkingDir: workingDir, Inspection: inspection, ObserveFilesystem: deps.ObserveFilesystem,
	}, data)
	if rejection != nil {
		return refuse(rejection, nil)
	}
	if *format == "json" {
		err = report.WriteGitPlanJSON(stdout, result, written)
	} else {
		err = report.WriteGitPlanText(stdout, result, written)
	}
	if err != nil {
		// The plan file is durable; only its report could not be delivered.
		fmt.Fprintf(stderr, "ai-dev: init plan: output failed after writing %s: %v\n", written, err)
		return 2
	}
	return 0
}
