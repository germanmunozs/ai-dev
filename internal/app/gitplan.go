package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/germanmunozs/ai-dev/internal/gitmanager"
	"github.com/germanmunozs/ai-dev/internal/report"
)

const gitPlanUsage = "Usage: ai-dev init plan --project-dir PATH --strategy basic|git-flow|github-flow|trunk-based [options] --output FILE [--format text|json]"

// commitValueOptions belong to the initial-commit group and are accepted
// only with --initial-commit.
var commitValueOptions = []string{
	"initial-commit-message-file", "author-name", "author-email", "committer-name", "committer-email", "author-date", "committer-date",
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
	initialCommit := flags.Bool("initial-commit", false, "")
	commitValues := map[string]*string{}
	for _, name := range commitValueOptions {
		commitValues[name] = flags.String(name, "", "")
	}
	var includes repeated
	flags.Var(&includes, "include", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || (*format != "text" && *format != "json") {
		fmt.Fprintln(stderr, gitPlanUsage)
		return 2
	}
	given := map[string]bool{}
	flags.Visit(func(f *flag.Flag) { given[f.Name] = true })
	if *output == "" {
		fmt.Fprintln(stderr, gitPlanUsage)
		return 2
	}
	commit, usage := commitRequest(*initialCommit, given, commitValues, includes)
	if usage != "" {
		fmt.Fprintf(stderr, "ai-dev: init plan: %s\n", usage)
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
		Branches: map[string]string{}, Prefixes: map[string]string{}, InitialCommit: commit,
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
	var candidates [][]byte
	if commit != nil {
		candidates = gitmanager.CommitCandidates(commit.Includes)
	}
	inspection, environment, err := inspectWithEnvironment(ctx, deps, *projectDir, candidates)
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

// commitRequest assembles the explicit initial-commit request. Every value
// is the operator's: the message is the exact bytes of its file, and
// identities and dates are never read from Git configuration. Commit
// options without --initial-commit, a missing message file, an unreadable
// message or a malformed date are usage errors; empty identities are left
// to the domain, which refuses them as identity_invalid.
func commitRequest(requested bool, given map[string]bool, values map[string]*string, includes []string) (*gitmanager.CommitRequest, string) {
	if !requested {
		for _, name := range append([]string{"include"}, commitValueOptions...) {
			if given[name] {
				return nil, "--" + name + " is only accepted with --initial-commit"
			}
		}
		return nil, ""
	}
	path := *values["initial-commit-message-file"]
	if path == "" {
		return nil, "--initial-commit needs --initial-commit-message-file"
	}
	message, err := readBoundedFile(path, gitmanager.LimitCommitMessageBytes.Max()+1)
	if err != nil {
		return nil, "the commit message file cannot be read"
	}
	request := &gitmanager.CommitRequest{
		MessageBytes: message, Includes: append([]string(nil), includes...),
		Author:    gitmanager.GitIdentity{Name: *values["author-name"], Email: *values["author-email"]},
		Committer: gitmanager.GitIdentity{Name: *values["committer-name"], Email: *values["committer-email"]},
	}
	for name, target := range map[string]*time.Time{"author-date": &request.AuthorDate, "committer-date": &request.CommitterDate} {
		if *values[name] == "" {
			continue
		}
		at, err := time.Parse(time.RFC3339, *values[name])
		if err != nil {
			return nil, "--" + name + " must be RFC 3339"
		}
		*target = at
	}
	return request, ""
}

// readBoundedFile reads at most limit bytes of a regular file.
func readBoundedFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file")
	}
	return io.ReadAll(io.LimitReader(file, limit))
}
