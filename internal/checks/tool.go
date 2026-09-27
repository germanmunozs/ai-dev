package checks

import (
	"context"
	"fmt"

	"ai-dev/internal/doctor"
	"ai-dev/internal/system"
)

type ToolCheck struct {
	id, name, binary string
	executor         system.Executor
}

func NewToolCheck(id, name, binary string, executor system.Executor) ToolCheck {
	return ToolCheck{id: id, name: name, binary: binary, executor: executor}
}

func (c ToolCheck) ID() string { return c.id }

func (c ToolCheck) Run(context.Context) (result doctor.CheckResult) {
	result = doctor.CheckResult{ID: c.id, Name: c.name, Area: "Environment"}
	defer func() {
		if recover() != nil {
			result.Status = doctor.Fail
			result.Explanation = fmt.Sprintf("%s (%s) lookup failed unexpectedly.", c.name, c.binary)
			result.Remediation = fmt.Sprintf("Verify the %s installation and ensure %s is available on PATH.", c.name, c.binary)
		}
	}()
	if _, err := c.executor.LookPath(c.binary); err != nil {
		result.Status = doctor.Fail
		result.Explanation = fmt.Sprintf("%s (%s) could not be found on PATH.", c.name, c.binary)
		result.Remediation = fmt.Sprintf("Install %s and ensure %s is available on PATH.", c.name, c.binary)
		return result
	}
	result.Status = doctor.Pass
	return result
}
