package checks

import (
	"context"
	"fmt"
	"strings"

	"github.com/germanmunozs/ai-dev/internal/doctor"
	"github.com/germanmunozs/ai-dev/internal/system"
)

type GitRepoCheck struct{ executor system.Executor }

func NewGitRepoCheck(executor system.Executor) GitRepoCheck { return GitRepoCheck{executor: executor} }

func (GitRepoCheck) ID() string { return "git-repository" }

func (c GitRepoCheck) Run(ctx context.Context) (result doctor.CheckResult) {
	result = doctor.CheckResult{ID: c.ID(), Name: "Current Git Repository", Area: "Repository"}
	defer func() {
		if recovered := recover(); recovered != nil {
			result.Status = doctor.Fail
			result.Explanation = fmt.Sprintf("Git could not establish a working tree because execution failed unexpectedly: %v.", recovered)
			result.Remediation = "Verify Git is installed and run the command from a Git working tree."
		}
	}()
	stdout, err := c.executor.Run(ctx, "git", "rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(stdout) != "true" {
		result.Status = doctor.Fail
		result.Explanation = "Git could not establish that the current directory is inside a working tree."
		result.Remediation = "Run the command from a Git working tree or initialize a Git repository."
		return result
	}
	result.Status = doctor.Pass
	return result
}
