package app

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"ai-dev/internal/gitmanager"
	"ai-dev/internal/report"
)

const gitInitUsage = "Usage: ai-dev init inspect [--project-dir PATH] [--format text|json]"

// RunGitInit routes the Git Management "init" commands. Only the read-only
// inspect command is active; it receives no store, no authorization and no
// mutating port.
func RunGitInit(ctx context.Context, args []string, deps GitManagementDependencies, stdout, stderr io.Writer) int {
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
	if deps.Reader == nil || deps.IdentifyGit == nil || deps.LookupEnv == nil || deps.Environ == nil || deps.WorkingDir == nil {
		return gitmanager.GitInspection{}, errors.New("Git inspection is unavailable in this build")
	}
	stateRoot, err := gitmanager.ResolveStateRoot(deps.GOOS, deps.LookupEnv)
	if err != nil {
		return gitmanager.GitInspection{}, err
	}
	environment, err := gitmanager.NewEnvironmentProfile(stateRoot)
	if err != nil {
		return gitmanager.GitInspection{}, err
	}
	tool, err := deps.IdentifyGit(ctx, environment)
	if err != nil {
		return gitmanager.GitInspection{}, err
	}
	workingDir, err := deps.WorkingDir()
	if err != nil {
		return gitmanager.GitInspection{}, err
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
		return gitmanager.GitInspection{}, err
	}
	now := time.Now
	if deps.Clock != nil {
		now = deps.Clock.Now
	}
	return gitmanager.Inspect(ctx, gitmanager.InspectorDependencies{Git: deps.Reader, ObserveFilesystem: deps.ObserveFilesystem},
		gitmanager.InspectRequest{
			ProjectDir: projectDir, WorkingDir: workingDir, Tool: tool, Environment: environment, User: user,
			InspectionID: id, InspectedAt: now().UTC().Truncate(time.Second),
		})
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
