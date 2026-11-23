package checks

import (
	"context"
	"strings"

	"github.com/germanmunozs/ai-dev/internal/agent"
	"github.com/germanmunozs/ai-dev/internal/doctor"
	"github.com/germanmunozs/ai-dev/internal/system"
)

const codingAgentRemediation = "Install a supported coding agent; run ai-dev agents to see the list."

// CodingAgentCheck passes when at least one catalog agent is on PATH
// (F013-FR-009). A passing result names the available agents in its name,
// because 001's report contract keeps a passing check's explanation empty.
type CodingAgentCheck struct {
	executor system.Executor
}

func NewCodingAgentCheck(executor system.Executor) CodingAgentCheck {
	return CodingAgentCheck{executor: executor}
}

func (CodingAgentCheck) ID() string { return "coding-agent-tool" }

func (c CodingAgentCheck) Run(context.Context) (result doctor.CheckResult) {
	result = doctor.CheckResult{ID: c.ID(), Name: "Coding agent", Area: "Environment"}
	defer func() {
		if recover() != nil {
			result = doctor.CheckResult{ID: c.ID(), Name: "Coding agent", Area: "Environment", Status: doctor.Fail,
				Explanation: "Coding agent lookup failed unexpectedly.", Remediation: codingAgentRemediation}
		}
	}()
	var available []string
	for _, status := range agent.NewRegistry(c.executor).List() {
		if status.Availability == agent.Available {
			available = append(available, string(status.Agent.ID))
		}
	}
	if len(available) == 0 {
		result.Status = doctor.Fail
		result.Explanation = "No supported coding agent was found on PATH."
		result.Remediation = codingAgentRemediation
		return result
	}
	result.Status = doctor.Pass
	result.Name = "Coding agent: " + strings.Join(available, ", ")
	return result
}
