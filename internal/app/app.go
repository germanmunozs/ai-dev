// Package app assembles checks and implements the command-level behavior.
package app

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/germanmunozs/ai-dev/internal/agent"
	"github.com/germanmunozs/ai-dev/internal/doctor"
	"github.com/germanmunozs/ai-dev/internal/gate"
	"github.com/germanmunozs/ai-dev/internal/report"
	"github.com/germanmunozs/ai-dev/internal/system"
)

const doctorUsage = "Usage: ai-dev doctor [--format text|json]"

const workflowGroupUsage = "Usage: ai-dev workflow <state|transition|next|guide|approve|abandon> [options]"

const agentGroupUsage = "Usage: ai-dev agent <run|select> [options]"

// topLevelUsage names every command group (F009-FR-005, F010-FR-010).
const topLevelUsage = "Usage: ai-dev <doctor|agents|agent|workflow|init|git|setup|tui|version|help|completion> [options]"

// Dependencies is the composition bundle for one CLI invocation. Supplying a
// dependency grants no authority by itself: each command receives only the
// capabilities its own entry point declares.
type Dependencies struct {
	Executor  system.Executor
	Launcher  system.Launcher
	Root      fs.FS
	GateStore gate.Store
	// ConfigStore writes the Feature 011 project configuration; nil makes
	// every write report persistence_failed.
	ConfigStore ConfigStore
	Git         GitManagementDependencies
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
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
		fmt.Fprintln(stderr, topLevelUsage)
		return 2
	}
	// A help request is answered before routing, so no command code runs
	// and no dependency is used (F010-FR-003, F010-FR-004).
	if spec, ok := helpRequest(args); ok {
		return writeHelp(pathHelp(spec), stdout, stderr)
	}
	switch args[0] {
	case "version", "--version":
		return RunVersion(args[1:], stdout, stderr)
	case "help", "--help", "-h":
		return RunHelp(args[1:], stdout, stderr)
	case "completion":
		return RunCompletion(args[1:], stdout, stderr)
	}
	if args[0] == "git" {
		if len(args) >= 3 && args[1] == "commit" && args[2] == "validate" {
			return RunGitCommitValidate(args[3:], stdout, stderr)
		}
		fmt.Fprintln(stderr, gitCommitValidateUsage)
		return 2
	}
	if args[0] == "init" {
		return RunGitInit(ctx, args[1:], deps.Git, stdin, isTerminal(stdin), stdout, stderr)
	}
	if args[0] == "tui" {
		return RunCockpit(ctx, args[1:], deps)
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
			return RunAgentSelect(args[2:], registry, root, stdout, stderr)
		}
		fmt.Fprintln(stderr, agentGroupUsage)
		return 2
	}
	if args[0] == "setup" {
		if len(args) >= 2 && args[1] == "show" {
			return RunSetupShow(args[2:], executor, root, stdout, stderr)
		}
		if len(args) >= 2 && args[1] == "apply" {
			return RunSetupApply(args[2:], executor, root, deps.ConfigStore, stdout, stderr)
		}
		if len(args) == 1 || strings.HasPrefix(args[1], "-") {
			interactive := isTerminal(stdin) && writerIsTerminal(stdout)
			return RunSetupWizard(ctx, args[1:], executor, root, deps.ConfigStore, stdin, interactive, stdout, stderr)
		}
		fmt.Fprintln(stderr, setupGroupUsage)
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
		if len(args) >= 2 && args[1] == "guide" {
			return RunWorkflowGuide(args[2:], root, executor, stdout, stderr)
		}
		if len(args) >= 2 && args[1] == "state" {
			return RunWorkflowState(args[2:], root, stdout, stderr)
		}
		if len(args) >= 2 && args[1] == "transition" {
			return RunWorkflowTransition(args[2:], root, stdout, stderr)
		}
		fmt.Fprintln(stderr, workflowGroupUsage)
		return 2
	}
	usage := func() { fmt.Fprintln(stderr, doctorUsage) }
	if args[0] != "doctor" {
		fmt.Fprintln(stderr, topLevelUsage)
		return 2
	}
	flags := newFlagSet("doctor")
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

// writerIsTerminal reports whether writer is a terminal device.
func writerIsTerminal(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	return ok && isTerminal(file)
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
