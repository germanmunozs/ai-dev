package execution

import (
	"context"
	"fmt"
	"io"

	"github.com/germanmunozs/ai-dev/internal/agent"
	"github.com/germanmunozs/ai-dev/internal/system"
)

// Runner owns the vendor-neutral lifecycle for exactly one execution request.
// Adapters is a fixed map assembled by the application; Run never selects,
// retries, or falls back to another agent.
type Runner struct {
	Registry agent.Registry
	Launcher system.Launcher
	Adapters map[agent.ID]Adapter
	Stdin    io.Reader
}

// Run resolves, launches, waits for, and classifies one explicitly selected
// agent. The caller chooses stream destinations before entering the lifecycle.
func (r Runner) Run(ctx context.Context, req Request, workdir string, stdout, stderr io.Writer) Report {
	report := Report{RequestedAgentID: string(req.AgentID)}

	resolved, ok := r.Registry.Resolve(req.AgentID)
	if !ok {
		report.Outcome = ExecutionFailed
		report.Explanation = fmt.Sprintf("unsupported agent: %s", req.AgentID)
		return report
	}

	available := false
	for _, status := range r.Registry.List() {
		if status.Agent.ID == resolved.ID {
			available = status.Availability == agent.Available
			break
		}
	}
	if !available {
		report.Outcome = ExecutionFailed
		report.Explanation = fmt.Sprintf("agent unavailable: %s not found on PATH", resolved.ID)
		return report
	}

	adapter, ok := r.Adapters[resolved.ID]
	if !ok || adapter == nil {
		report.Outcome = ExecutionFailed
		report.Explanation = fmt.Sprintf("execution adapter unavailable for %s", resolved.ID)
		return report
	}

	name, args := adapter.Command(workdir, req.Instruction)
	handle, err := r.Launcher.Launch(ctx, system.Spec{
		Name:   name,
		Args:   args,
		Dir:    workdir,
		Stdin:  r.Stdin,
		Stdout: stdout,
		Stderr: stderr,
	})
	if err != nil {
		report.Outcome = ExecutionFailed
		report.Explanation = fmt.Sprintf("launch agent: %v", err)
		return report
	}

	status, waitErr := handle.Wait()
	if status.Canceled {
		report.Outcome = ExecutionFailed
		report.Explanation = "agent execution canceled"
		return report
	}
	if !status.Exited {
		report.Outcome = ExecutionFailed
		if waitErr != nil {
			report.Explanation = fmt.Sprintf("agent terminated abnormally: %v", waitErr)
		} else {
			report.Explanation = "agent terminated abnormally"
		}
		return report
	}
	if waitErr != nil && status.ExitCode == 0 {
		report.Outcome = ExecutionFailed
		report.Explanation = fmt.Sprintf("wait for agent: %v", waitErr)
		return report
	}

	// A normal non-zero process exit is commonly returned together with an
	// exec.ExitError. The adapter still owns classification of that exit code.
	report.Outcome = adapter.Classify(status.ExitCode)
	if report.Outcome == AgentFailed {
		report.Explanation = fmt.Sprintf("agent exited with status %d", status.ExitCode)
	}
	return report
}
