package app

import (
	"context"

	"ai-dev/internal/gitmanager"
)

// GitManagementDependencies is the Feature 008 composition bundle. Merely
// supplying it does not activate a command or grant mutation authority.
// Rendering is not a dependency: Git commands return domain results that
// internal/report renders, as every earlier feature does.
type GitManagementDependencies struct {
	Runner       gitmanager.GitRunner
	Store        gitmanager.GitStore
	Filesystem   gitmanager.Filesystem
	Clock        gitmanager.Clock
	RandomSource gitmanager.RandomSource

	// Read-only inspection capabilities.
	Reader            gitmanager.GitReader
	ObserveFilesystem func(path string) (gitmanager.FilesystemObservation, error)
	IdentifyGit       func(context.Context, gitmanager.EnvironmentProfile) (gitmanager.GitToolIdentity, error)
	LookupEnv         func(string) (string, bool)
	Environ           func() []string
	WorkingDir        func() (string, error)
	GOOS              string
}

func unavailableGitManagementDependencies() GitManagementDependencies {
	return GitManagementDependencies{}
}
