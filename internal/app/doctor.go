package app

import (
	"context"
	"io/fs"

	"github.com/germanmunozs/ai-dev/internal/checks"
	"github.com/germanmunozs/ai-dev/internal/doctor"
	"github.com/germanmunozs/ai-dev/internal/system"
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
