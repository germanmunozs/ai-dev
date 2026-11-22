package app

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/germanmunozs/ai-dev/internal/agent"
	"github.com/germanmunozs/ai-dev/internal/execution"
	"github.com/germanmunozs/ai-dev/internal/execution/adapters"
	"github.com/germanmunozs/ai-dev/internal/instruction"
	"github.com/germanmunozs/ai-dev/internal/report"
	"github.com/germanmunozs/ai-dev/internal/system"
)

const agentRunUsage = "Usage: ai-dev agent run [--format text|json] <agent-id> [instruction]"

var getWorkingDirectory = os.Getwd

// RunAgentRun validates and executes one explicitly selected coding agent.
func RunAgentRun(
	ctx context.Context,
	args []string,
	executor system.Executor,
	launcher system.Launcher,
	stdin io.Reader,
	stdinIsInteractive bool,
	workdir string,
	stdout, stderr io.Writer,
) int {
	usage := func() { fmt.Fprintln(stderr, agentRunUsage) }
	flags := newFlagSet("agent run")
	format := flags.String("format", "text", "output format: text or json")
	if err := flags.Parse(args); err != nil || flags.NArg() < 1 || flags.NArg() > 2 || (*format != "text" && *format != "json") {
		usage()
		return 2
	}

	requestedID := flags.Arg(0)
	resolvedInstruction, err := instruction.Resolve(flags.Arg(1), flags.NArg() == 2, stdin, stdinIsInteractive)
	if err != nil {
		return finishAgentRun(*format, execution.Report{
			RequestedAgentID: requestedID,
			Outcome:          execution.ExecutionFailed,
			Explanation:      err.Error(),
		}, stdout, stderr)
	}
	if workdir == "" {
		workdir, err = getWorkingDirectory()
		if err != nil {
			return finishAgentRun(*format, execution.Report{
				RequestedAgentID: requestedID,
				Outcome:          execution.ExecutionFailed,
				Explanation:      fmt.Sprintf("determine current working directory: %v", err),
			}, stdout, stderr)
		}
	}

	runner := execution.Runner{
		Registry: agent.NewRegistry(executor),
		Launcher: launcher,
		Adapters: map[agent.ID]execution.Adapter{
			agent.ClaudeCode: adapters.Claude{},
			agent.CodexCLI:   adapters.Codex{},
		},
		Stdin: stdin,
	}

	agentStdout, agentStderr := stdout, stderr
	if *format == "json" {
		agentStdout = stderr
		agentStderr = stderr
	}
	result := runner.Run(ctx, execution.Request{
		AgentID:     agent.ID(requestedID),
		Instruction: resolvedInstruction,
	}, workdir, agentStdout, agentStderr)
	return finishAgentRun(*format, result, stdout, stderr)
}

func finishAgentRun(format string, result execution.Report, stdout, stderr io.Writer) int {
	var err error
	if format == "json" {
		err = report.WriteExecutionJSON(stdout, result)
	} else {
		err = report.WriteExecutionText(stderr, result)
	}
	if err != nil {
		fmt.Fprintf(stderr, "write execution report: %v\n", err)
		return 1
	}
	if result.Outcome == execution.AgentSucceeded {
		return 0
	}
	return 1
}
