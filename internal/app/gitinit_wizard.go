package app

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"ai-dev/internal/gitmanager"
	"ai-dev/internal/report"
)

// wizardStrategies are shown in a fixed order with no preselection.
var wizardStrategies = []gitmanager.Strategy{
	gitmanager.StrategyBasic, gitmanager.StrategyGitFlow, gitmanager.StrategyGitHubFlow, gitmanager.StrategyTrunkBased,
}

var errWizardCancelled = errors.New("cancelled")
var errWizardInvalid = errors.New("invalid input")

// wizard is the interactive assistant (CLI contract §1). It uses the same
// inspector, planner, issuer and apply engine as the non-interactive
// commands and holds no authority until the operator has typed the plan
// digest.
type wizard struct {
	ctx    context.Context
	deps   GitManagementDependencies
	in     *bufio.Reader
	out    io.Writer
	stderr io.Writer
}

// RunGitWizard implements "ai-dev init". Without an interactive terminal it
// refuses before inspecting, planning, locking, auditing or mutating.
func RunGitWizard(ctx context.Context, args []string, deps GitManagementDependencies, stdin io.Reader, interactive bool, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	projectDir := flags.String("project-dir", ".", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "Usage: ai-dev init [--project-dir PATH]")
		return 2
	}
	if !interactive {
		fmt.Fprintln(stderr, "ai-dev: init: refused: interactive_terminal_required")
		return 2
	}
	w := &wizard{ctx: ctx, deps: deps, in: bufio.NewReader(stdin), out: stdout, stderr: stderr}
	code, err := w.run(*projectDir)
	switch {
	case errors.Is(err, errWizardCancelled):
		fmt.Fprintln(stdout, "Cancelled. Nothing was authorized or changed.")
		return 1
	case errors.Is(err, errWizardInvalid):
		fmt.Fprintln(stderr, "ai-dev: init: invalid input; nothing was authorized or changed")
		return 2
	case err != nil:
		fmt.Fprintf(stderr, "ai-dev: init: %v\n", err)
		return 1
	}
	return code
}

// ask prints a prompt and reads one line. EOF cancels; an empty answer is
// invalid because the assistant has no defaults.
func (w *wizard) ask(prompt string) (string, error) {
	fmt.Fprint(w.out, prompt)
	line, err := w.in.ReadString('\n')
	if err != nil && (line == "" || !errors.Is(err, io.EOF)) {
		return "", errWizardCancelled
	}
	answer := strings.TrimRight(line, "\r\n")
	if answer == "" {
		return "", errWizardInvalid
	}
	return answer, nil
}

// confirm requires the literal answer "yes"; "no" cancels.
func (w *wizard) confirm(prompt string) error {
	answer, err := w.ask(prompt + " [yes/no] ")
	switch {
	case err != nil:
		return err
	case answer == "yes":
		return nil
	case answer == "no":
		return errWizardCancelled
	}
	return errWizardInvalid
}

func (w *wizard) run(projectDir string) (int, error) {
	inspection, environment, err := inspectWithEnvironment(w.ctx, w.deps, projectDir, nil)
	if err != nil {
		return 0, err
	}
	if err := report.WriteGitInspectionText(w.out, inspection); err != nil {
		return 0, err
	}
	req, err := w.strategyRequest(inspection)
	if err != nil {
		return 0, err
	}
	planID, err := newInspectionID()
	if err != nil {
		return 0, err
	}
	now := time.Now
	if w.deps.Clock != nil {
		now = w.deps.Clock.Now
	}
	planner := gitmanager.PlannerDependencies{Git: w.deps.Reader, Commands: w.deps.Commands, Random: w.deps.RandomSource}
	result, rejection := gitmanager.Plan(w.ctx, planner, req, inspection, environment,
		gitmanager.PlanMetadata{PlanID: gitmanager.PlanID(planID), GeneratedAt: now().UTC().Truncate(time.Second)})
	if rejection != nil {
		fmt.Fprintf(w.out, "refused: %s: %s\n", rejection.Reason, rejection.Detail)
		return rejection.ExitCode(), nil
	}
	plan := result.Plan
	fmt.Fprintf(w.out, "\nPlan %s\n", plan.PlanDigest)
	for _, operation := range plan.DerivedOperations() {
		fmt.Fprintf(w.out, "  %d. %s %s\n", operation.Ordinal, operation.Kind, operation.TargetIdentity)
	}
	for _, pending := range result.PendingDependencies {
		fmt.Fprintf(w.out, "  pending: %s\n", pending)
	}

	destination, err := w.ask("Plan destination file, outside the project: ")
	if err != nil {
		return 0, err
	}
	if !filepath.IsAbs(destination) {
		workingDir, err := w.deps.WorkingDir()
		if err != nil {
			return 0, err
		}
		destination = filepath.Join(workingDir, destination)
	}
	if err := w.confirm(fmt.Sprintf("Write the plan to %s?", destination)); err != nil {
		return 0, err
	}
	data, err := gitmanager.EncodePlanFile(plan)
	if err != nil {
		return 0, err
	}
	written, rejection := gitmanager.WritePlanOutput(gitmanager.PlanOutputRequest{
		Path: destination, WorkingDir: "/", Inspection: inspection, ObserveFilesystem: w.deps.ObserveFilesystem,
	}, data)
	if rejection != nil {
		fmt.Fprintf(w.out, "refused: %s: %s\n", rejection.Reason, rejection.Detail)
		return rejection.ExitCode(), nil
	}
	fmt.Fprintf(w.out, "Plan written to %s\n", written)
	if w.deps.Executor == nil {
		fmt.Fprintln(w.out, "Apply is not enabled yet in this build; no authorization was issued.")
		return 0, nil
	}

	// Written confirmation bound to the digest is supervised
	// authorization, not authentication.
	typed, err := w.ask("Type the plan digest to authorize applying it: ")
	if err != nil {
		return 0, err
	}
	if typed != string(plan.PlanDigest) {
		fmt.Fprintln(w.out, "The typed digest does not match the plan.")
		return 0, errWizardCancelled
	}
	operator, err := w.ask("Declared operator (a label, not authentication): ")
	if err != nil {
		return 0, err
	}
	authorizationID, err := randomUUID(w.deps.RandomSource)
	if err != nil {
		return 0, err
	}
	fmt.Fprintf(w.out, "Authorization %s for plan %s covering %d operations\n", authorizationID, plan.PlanDigest, len(plan.DerivedOperations()))
	applied, err := applyPlan(w.ctx, w.deps, gitmanager.ApplyRequest{
		PlanFile: data, ConfirmPlan: string(plan.PlanDigest), AuthorizationID: authorizationID,
		Issue: &gitmanager.IssueRequest{AuthorizedBy: operator},
	})
	if err != nil {
		return 0, err
	}
	return writeApplyResult(w.out, w.stderr, "text", applied), nil
}

// strategyRequest gathers the explicit strategy inputs.
func (w *wizard) strategyRequest(inspection gitmanager.GitInspection) (gitmanager.StrategyRequest, error) {
	fmt.Fprintln(w.out, "\nSelect a Git strategy (there is no default):")
	for i, strategy := range wizardStrategies {
		fmt.Fprintf(w.out, "  %d) %s\n", i+1, strategy)
	}
	answer, err := w.ask("> ")
	if err != nil {
		return gitmanager.StrategyRequest{}, err
	}
	var strategy gitmanager.Strategy
	for i, candidate := range wizardStrategies {
		if answer == fmt.Sprint(i+1) || answer == string(candidate) {
			strategy = candidate
		}
	}
	if strategy == "" {
		return gitmanager.StrategyRequest{}, errWizardInvalid
	}
	req := gitmanager.StrategyRequest{Strategy: string(strategy), Branches: map[string]string{}, Prefixes: map[string]string{}}
	absent := inspection.Observation.RepositoryKind == gitmanager.RepositoryKindAbsent
	if absent {
		if err := w.confirm("The project has no repository. Initialize one?"); err != nil {
			return req, err
		}
		req.InitializeRepository = true
	}
	branch := func(option, prompt string) error {
		value, err := w.ask(prompt)
		if err == nil {
			req.Branches[option] = value
		}
		return err
	}
	switch strategy {
	case gitmanager.StrategyBasic, gitmanager.StrategyGitHubFlow:
		if absent {
			if err := branch("initial-branch", "Initial branch name: "); err != nil {
				return req, err
			}
		}
	case gitmanager.StrategyTrunkBased:
		if err := branch("trunk-branch", "Trunk branch name: "); err != nil {
			return req, err
		}
	case gitmanager.StrategyGitFlow:
		if err := branch("production-branch", "Production branch name: "); err != nil {
			return req, err
		}
		if err := branch("development-branch", "Development branch name: "); err != nil {
			return req, err
		}
		if inspection.Observation.GitFlowEvidence.Classification == gitmanager.GitFlowAbsent {
			dialect, err := w.ask("Git Flow dialect (avh or git-flow-next): ")
			if err != nil {
				return req, err
			}
			req.GitFlowDialect = dialect
			for _, name := range []string{"feature", "bugfix", "release", "hotfix", "support", "version_tag"} {
				value, err := w.ask(strings.ReplaceAll(name, "_", " ") + " prefix: ")
				if err != nil {
					return req, err
				}
				req.Prefixes[name] = value
			}
		}
	}
	return req, nil
}

// randomUUID draws a UUIDv4 from the injected random source.
func randomUUID(random io.Reader) (string, error) {
	if random == nil {
		return "", errors.New("no random source")
	}
	b := make([]byte, 16)
	if _, err := io.ReadFull(random, b); err != nil {
		return "", err
	}
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
