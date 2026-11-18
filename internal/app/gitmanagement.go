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

	// Read-only inspection and planning capabilities. Commands only
	// builds argv for a plan; it runs nothing.
	Reader            gitmanager.GitReader
	Commands          gitmanager.GitCommandBuilder
	ObserveFilesystem func(path string) (gitmanager.FilesystemObservation, error)
	IdentifyGit       func(context.Context, gitmanager.EnvironmentProfile) (gitmanager.GitToolIdentity, error)
	LookupEnv         func(string) (string, bool)
	Environ           func() []string
	WorkingDir        func() (string, error)
	GOOS              string

	// Apply capabilities. OpenStore opens the durable store at the state
	// root. Executor runs permitted operations; it stays nil until the
	// mutation operations exist (T107), and apply is then refused before
	// any record is created.
	OpenStore func(gitmanager.ObservedPath) (gitmanager.GitStore, func(), error)
	Executor  gitmanager.OperationExecutor
}

func unavailableGitManagementDependencies() GitManagementDependencies {
	return GitManagementDependencies{}
}
