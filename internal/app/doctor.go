// Package app assembles checks and implements the command-level behavior.
package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/fs"

	"ai-dev/internal/checks"
	"ai-dev/internal/doctor"
	"ai-dev/internal/report"
	"ai-dev/internal/system"
)

// RunDoctor assembles and runs exactly the nine contract checks in stable order.
func RunDoctor(ctx context.Context, executor system.Executor, root fs.FS) doctor.Report {
	all := []doctor.Check{
		checks.NewToolCheck("git-tool", "Git", "git", executor),
		checks.NewToolCheck("speckit-tool", "Spec Kit", "specify", executor),
		checks.NewToolCheck("claude-code-tool", "Claude Code", "claude", executor),
		checks.NewToolCheck("codex-cli-tool", "Codex CLI", "codex", executor),
		checks.NewGitRepoCheck(executor),
		checks.NewSpecKitCheck(root),
		checks.NewConstitutionCheck(root),
		checks.NewManifestIntegrationCheck("claude", "claude-integration", "Claude Spec Kit Integration", root),
		checks.NewManifestIntegrationCheck("codex", "codex-integration", "Codex Spec Kit Integration", root),
	}
	return doctor.RunAll(ctx, all)
}

// Run executes the CLI application and returns its process exit code.
func Run(ctx context.Context, args []string, executor system.Executor, root fs.FS, stdout, stderr io.Writer) int {
	usage := func() { fmt.Fprintln(stderr, "Usage: ai-dev doctor [--format text|json]") }
	if len(args) == 0 || args[0] != "doctor" {
		usage()
		return 2
	}
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	format := flags.String("format", "text", "output format: text or json")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || (*format != "text" && *format != "json") {
		usage()
		return 2
	}
	reportValue := RunDoctor(ctx, executor, root)
	var err error
	if *format == "json" {
		err = report.WriteJSON(stdout, reportValue)
	} else {
		err = report.WriteText(stdout, reportValue)
	}
	if err != nil {
		fmt.Fprintf(stderr, "write report: %v\n", err)
		return 2
	}
	if reportValue.Overall == doctor.Pass {
		return 0
	}
	return 1
}
