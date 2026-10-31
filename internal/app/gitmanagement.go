package app

import "ai-dev/internal/gitmanager"

// GitManagementDependencies is the Feature 008 composition bundle. Merely
// supplying it does not activate a command or grant mutation authority.
type GitManagementDependencies struct {
	Runner       gitmanager.GitRunner
	Store        gitmanager.GitStore
	Filesystem   gitmanager.Filesystem
	Clock        gitmanager.Clock
	RandomSource gitmanager.RandomSource
	Reporter     gitmanager.Reporter
}

func unavailableGitManagementDependencies() GitManagementDependencies {
	return GitManagementDependencies{}
}
