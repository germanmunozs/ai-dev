package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"ai-dev/internal/gitmanager"
	"ai-dev/internal/report"
)

const gitApplyUsage = "Usage: ai-dev init apply --plan FILE --confirm-plan sha256:DIGEST --authorization-id UUID [--issue-authorization --authorized-by TEXT [--authorization-expires-at RFC3339] [--confirm-initial-commit] [--approve-file TUPLE]... [--approve-ignored-file TUPLE]...] [--format text|json]"

// repeated collects a repeatable string option.
type repeated []string

func (r *repeated) String() string     { return strings.Join(*r, ",") }
func (r *repeated) Set(v string) error { *r = append(*r, v); return nil }

// issuanceOptions are accepted only with --issue-authorization; the
// consume form cannot reissue, widen or replace authority.
var issuanceOptions = []string{"authorized-by", "authorization-expires-at", "confirm-initial-commit", "approve-file", "approve-ignored-file"}

// RunGitApply implements "ai-dev init apply" in its issue-and-consume and
// consume-existing forms (CLI contract §4–5).
func RunGitApply(ctx context.Context, args []string, deps GitManagementDependencies, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("init apply", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	planPath := flags.String("plan", "", "")
	confirm := flags.String("confirm-plan", "", "")
	authorizationID := flags.String("authorization-id", "", "")
	issue := flags.Bool("issue-authorization", false, "")
	authorizedBy := flags.String("authorized-by", "", "")
	expires := flags.String("authorization-expires-at", "", "")
	confirmCommit := flags.Bool("confirm-initial-commit", false, "")
	format := flags.String("format", "text", "")
	var approvals, ignored repeated
	flags.Var(&approvals, "approve-file", "")
	flags.Var(&ignored, "approve-ignored-file", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || (*format != "text" && *format != "json") ||
		*planPath == "" || *confirm == "" || *authorizationID == "" {
		fmt.Fprintln(stderr, gitApplyUsage)
		return 2
	}
	given := map[string]bool{}
	flags.Visit(func(f *flag.Flag) { given[f.Name] = true })
	if !*issue {
		for _, name := range issuanceOptions {
			if given[name] {
				fmt.Fprintf(stderr, "ai-dev: init apply: --%s is only accepted with --issue-authorization\n", name)
				return 2
			}
		}
	}
	if deps.Executor == nil {
		fmt.Fprintln(stderr, "ai-dev: init apply: apply is not enabled yet: the mutation operations are not available in this build")
		return 2
	}
	planFile, err := readPlanFile(*planPath)
	if err != nil {
		fmt.Fprintf(stderr, "ai-dev: init apply: %v\n", err)
		return 2
	}
	request := gitmanager.ApplyRequest{PlanFile: planFile, ConfirmPlan: *confirm, AuthorizationID: *authorizationID}
	if *issue {
		request.Issue = &gitmanager.IssueRequest{
			AuthorizedBy: *authorizedBy, ExpiresAt: *expires, ConfirmInitialCommit: *confirmCommit, Approvals: approvals, IgnoredApprovals: ignored,
		}
	}
	result, err := applyPlan(ctx, deps, request)
	if err != nil {
		fmt.Fprintf(stderr, "ai-dev: init apply: %v\n", err)
		return 1
	}
	return writeApplyResult(stdout, stderr, *format, result)
}

func writeApplyResult(stdout, stderr io.Writer, format string, result gitmanager.GitApplyResult) int {
	var err error
	if format == "json" {
		err = report.WriteGitApplyJSON(stdout, result)
	} else {
		err = report.WriteGitApplyText(stdout, result)
	}
	if err != nil {
		// The durable outcome stands; only its delivery failed.
		fmt.Fprintf(stderr, "ai-dev: init apply: output failed after apply %s: %v\n", result.ApplyID, err)
		return 2
	}
	return result.ExitCode
}

// readPlanFile reads at most the plan bound plus one byte, so an oversized
// file reaches the engine's bound check without being read whole.
func readPlanFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("the plan cannot be read")
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, gitmanager.LimitPlanBytes.Max()+1))
}

// applyPlan composes the apply engine from the real user context: the
// toolchain is identified once, the store is opened at the canonical state
// root, and the project the plan declares is inspected freshly.
func applyPlan(ctx context.Context, deps GitManagementDependencies, request gitmanager.ApplyRequest) (gitmanager.GitApplyResult, error) {
	if deps.OpenStore == nil || deps.IdentifyGit == nil || deps.LookupEnv == nil || deps.Commands == nil || deps.RandomSource == nil {
		return gitmanager.GitApplyResult{}, fmt.Errorf("Git apply is unavailable in this build")
	}
	stateRoot, err := gitmanager.ResolveStateRoot(deps.GOOS, deps.LookupEnv)
	if err != nil {
		return gitmanager.GitApplyResult{}, err
	}
	environment, err := gitmanager.NewEnvironmentProfile(stateRoot)
	if err != nil {
		return gitmanager.GitApplyResult{}, err
	}
	tool, err := deps.IdentifyGit(ctx, environment)
	if err != nil {
		return gitmanager.GitApplyResult{}, err
	}
	store, closeStore, err := deps.OpenStore(stateRoot)
	if err != nil {
		return gitmanager.GitApplyResult{}, fmt.Errorf("the Git state store is unavailable: %v", err)
	}
	defer closeStore()
	return gitmanager.Apply(ctx, gitmanager.ApplyDependencies{
		Store: store, Clock: deps.Clock, Random: deps.RandomSource,
		Planner: gitmanager.PlannerDependencies{Git: deps.Reader, Commands: deps.Commands, Random: deps.RandomSource},
		Tool:    tool, Environment: environment, Executor: deps.Executor,
		Inspect: func(ctx context.Context, projectPath []byte) (gitmanager.GitInspection, error) {
			inspection, _, err := inspectWithEnvironment(ctx, deps, string(projectPath))
			return inspection, err
		},
	}, request), nil
}
