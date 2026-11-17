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
	"ai-dev/internal/doctor"
	"ai-dev/internal/gate"
	"ai-dev/internal/report"
	"ai-dev/internal/system"
)

// Dependencies is the composition bundle for one CLI invocation. Supplying a
// dependency grants no authority by itself: each command receives only the
// capabilities its own entry point declares.
type Dependencies struct {
	Executor  system.Executor
	Launcher  system.Launcher
	Root      fs.FS
	GateStore gate.Store
	Git       GitManagementDependencies
	Stdin     io.Reader
	Stdout    io.Writer
	Stderr    io.Writer
}

// Run executes the CLI application and returns its process exit code.
func Run(ctx context.Context, args []string, executor system.Executor, launcher system.Launcher, root fs.FS, stdin io.Reader, stdout, stderr io.Writer) int {
	return RunWithGateStore(ctx, args, executor, launcher, root, nil, stdin, stdout, stderr)
}

// RunWithGateStore executes the CLI application with the Human Gate store.
func RunWithGateStore(ctx context.Context, args []string, executor system.Executor, launcher system.Launcher, root fs.FS, store gate.Store, stdin io.Reader, stdout, stderr io.Writer) int {
	return RunWithDependencies(ctx, args, Dependencies{
		Executor:  executor,
		Launcher:  launcher,
		Root:      root,
		GateStore: store,
		Git:       unavailableGitManagementDependencies(),
		Stdin:     stdin,
		Stdout:    stdout,
		Stderr:    stderr,
	})
}

// RunWithDependencies routes one CLI invocation to its command.
func RunWithDependencies(ctx context.Context, args []string, deps Dependencies) int {
	executor, launcher, root, store := deps.Executor, deps.Launcher, deps.Root, deps.GateStore
	stdin, stdout, stderr := deps.Stdin, deps.Stdout, deps.Stderr
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
		if len(args) >= 2 && args[1] == "approve" {
			if store == nil {
				fmt.Fprintln(stderr, "ai-dev: internal error: human gate store unavailable")
				return 2
			}
			return RunWorkflowGate(gate.Approval, args[2:], root, store, stdout, stderr)
		}
		if len(args) >= 2 && args[1] == "abandon" {
			if store == nil {
				fmt.Fprintln(stderr, "ai-dev: internal error: human gate store unavailable")
				return 2
			}
			return RunWorkflowGate(gate.Abandonment, args[2:], root, store, stdout, stderr)
		}
		if len(args) >= 2 && args[1] == "next" {
			return RunWorkflowNext(args[2:], root, stdout, stderr)
		}
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
