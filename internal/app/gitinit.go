package app

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/germanmunozs/ai-dev/internal/gitmanager"
	"github.com/germanmunozs/ai-dev/internal/report"
)

const gitInitUsage = "Usage: ai-dev init [inspect|plan|apply|reconcile] [options]"

// RunGitInit routes the Git Management "init" commands: the assistant,
// inspect, plan, apply and the read-only reconcile.
func RunGitInit(ctx context.Context, args []string, deps GitManagementDependencies, stdin io.Reader, interactive bool, stdout, stderr io.Writer) int {
	// INT-01: SIGTERM, and SIGHUP when the terminal closes, are interrupts
	// like the terminal's SIGINT, which the entrypoint already turns into
	// cancellation; apply defers them while a mutation is under way.
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	done := make(chan struct{})
	defer close(done)
	go exitOnSecondInterrupt(ctx, done, stderr)
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return RunGitWizard(ctx, args, deps, stdin, interactive, stdout, stderr)
	}
	if len(args) > 0 && args[0] == "plan" {
		return RunGitPlan(ctx, args[1:], deps, stdout, stderr)
	}
	if len(args) > 0 && args[0] == "apply" {
		return RunGitApply(ctx, args[1:], deps, stdout, stderr)
	}
	if args[0] == "reconcile" {
		return RunGitReconcile(ctx, args[1:], deps, stdout, stderr)
	}
	if len(args) == 0 || args[0] != "inspect" {
		fmt.Fprintln(stderr, gitInitUsage)
		return 2
	}
	flags := flag.NewFlagSet("init inspect", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	projectDir := flags.String("project-dir", ".", "project directory")
	format := flags.String("format", "text", "output format: text or json")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || (*format != "text" && *format != "json") || *projectDir == "" {
		fmt.Fprintln(stderr, gitInitUsage)
		return 2
	}
	inspection, err := inspectProject(ctx, deps, *projectDir)
	if err != nil {
		fmt.Fprintf(stderr, "ai-dev: init inspect: %v\n", err)
		return 1
	}
	if *format == "json" {
		err = report.WriteGitInspectionJSON(stdout, inspection)
	} else {
		err = report.WriteGitInspectionText(stdout, inspection)
	}
	if err != nil {
		fmt.Fprintf(stderr, "ai-dev: init inspect: output failed: %v\n", err)
		return 2
	}
	return 0
}

// inspectProject composes one read-only inspection from the real user
// context: the canonical state root names the private Git environment,
// the toolchain is identified once, and the in-process evaluators read the
// user's real HOME and XDG configuration.
func inspectProject(ctx context.Context, deps GitManagementDependencies, projectDir string) (gitmanager.GitInspection, error) {
	inspection, _, err := inspectWithEnvironment(ctx, deps, projectDir, nil)
	return inspection, err
}

// inspectWithEnvironment inspects, observing any initial-commit
// candidates, and also returns the private Git environment profile the
// inspection used.
func inspectWithEnvironment(ctx context.Context, deps GitManagementDependencies, projectDir string, candidates [][]byte) (gitmanager.GitInspection, gitmanager.EnvironmentProfile, error) {
	fail := func(err error) (gitmanager.GitInspection, gitmanager.EnvironmentProfile, error) {
		return gitmanager.GitInspection{}, gitmanager.EnvironmentProfile{}, err
	}
	if deps.Reader == nil || deps.IdentifyGit == nil || deps.LookupEnv == nil || deps.Environ == nil || deps.WorkingDir == nil {
		return fail(errors.New("Git inspection is unavailable in this build"))
	}
	stateRoot, err := gitmanager.ResolveStateRoot(deps.GOOS, deps.LookupEnv)
	if err != nil {
		return fail(err)
	}
	environment, err := gitmanager.NewEnvironmentProfile(stateRoot)
	if err != nil {
		return fail(err)
	}
	tool, err := deps.IdentifyGit(ctx, environment)
	if err != nil {
		return fail(err)
	}
	workingDir, err := deps.WorkingDir()
	if err != nil {
		return fail(err)
	}
	user := gitmanager.UserContext{}
	if home, ok := deps.LookupEnv("HOME"); ok {
		user.Home = []byte(home)
	}
	if xdg, ok := deps.LookupEnv("XDG_CONFIG_HOME"); ok && xdg != "" {
		user.XDGConfigHome = []byte(xdg)
	}
	for _, entry := range deps.Environ() {
		if strings.HasPrefix(entry, "GIT_CONFIG") {
			user.EnvironmentOverrides = true
		}
	}
	id, err := newInspectionID()
	if err != nil {
		return fail(err)
	}
	now := time.Now
	if deps.Clock != nil {
		now = deps.Clock.Now
	}
	inspection, err := gitmanager.Inspect(ctx, gitmanager.InspectorDependencies{Git: deps.Reader, ObserveFilesystem: deps.ObserveFilesystem},
		gitmanager.InspectRequest{
			ProjectDir: projectDir, WorkingDir: workingDir, Tool: tool, Environment: environment, User: user,
			InspectionID: id, InspectedAt: now().UTC().Truncate(time.Second), Candidates: candidates,
		})
	return inspection, environment, err
}

// newInspectionID is a random UUIDv4 naming one inspection.
func newInspectionID() (gitmanager.InspectionID, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return gitmanager.InspectionID(fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])), nil
}

// exitOnSecondInterrupt keeps ai-dev stoppable once an interrupt has been
// deferred (INT-01, review R-03): a hung Git or a blocked terminal read
// would otherwise ignore every further SIGINT and SIGTERM. A second
// interrupt exits at once, which is the crash path: the attempt stays
// unclosed and is reconciled with ai-dev init reconcile.
func exitOnSecondInterrupt(ctx context.Context, done <-chan struct{}, stderr io.Writer) {
	select {
	case <-ctx.Done():
	case <-done:
		return
	}
	second := make(chan os.Signal, 1)
	signal.Notify(second, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(second)
	select {
	case <-second:
		fmt.Fprintln(stderr, "ai-dev: second interrupt: exiting without closing the attempt; inspect it with ai-dev init reconcile")
		os.Exit(130)
	case <-done:
	}
}
