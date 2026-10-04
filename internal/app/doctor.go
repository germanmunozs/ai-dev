// Package app assembles checks and implements the command-level behavior.
package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"

	"ai-dev/internal/agent"
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

func Run(ctx context.Context, args []string, executor system.Executor, launcher system.Launcher, root fs.FS, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "Usage: ai-dev <doctor|agents|agent|workflow> [options]")
		return 2
	}
	if args[0] == "agents" {
		return RunAgentsList(args[1:], executor, stdout, stderr)
	}
	if args[0] == "agent" {
		if len(args) >= 2 && args[1] == "run" {
			return RunAgentRun(ctx, args[2:], executor, launcher, stdin, readerIsInteractive(stdin), "", stdout, stderr)
		}
		if len(args) >= 2 && args[1] == "select" {
			registry := agent.NewRegistry(executor)
			return RunAgentSelect(args[2:], registry, stdout, stderr)
		}
		fmt.Fprintln(stderr, "Usage: ai-dev agent <run|select> [options]")
		return 2
	}
	if args[0] == "workflow" {
		if len(args) >= 2 && args[1] == "state" {
			return RunWorkflowState(args[2:], root, stdout, stderr)
		}
		if len(args) >= 2 && args[1] == "transition" {
			return RunWorkflowTransition(args[2:], root, stdout, stderr)
		}
		fmt.Fprintln(stderr, "Usage: ai-dev workflow <state|transition> [options]")
		return 2
	}
	usage := func() { fmt.Fprintln(stderr, "Usage: ai-dev doctor [--format text|json]") }
	if args[0] != "doctor" {
		fmt.Fprintln(stderr, "Usage: ai-dev <doctor|agents|agent|workflow> [options]")
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

func readerIsInteractive(reader io.Reader) bool {
	file, ok := reader.(interface {
		Stat() (os.FileInfo, error)
	})
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
