package app

import (
	"context"

	"github.com/germanmunozs/ai-dev/internal/gitmanager"
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
	// root. Runner executes Git; with it the composition runs permitted
	// operations through the production OperationDispatcher. Executor, when
	// set, replaces that dispatcher (tests only). Without either, apply is
	// refused before any record is created.
	OpenStore func(gitmanager.ObservedPath) (gitmanager.GitStore, func(), error)
	Executor  gitmanager.OperationExecutor
}

// executor is the composition's operation executor: the injected one, or
// the production dispatcher over the composed runner and builder, whose
// fresh inspections use the same user context as every other command. It
// adds no mutation path: the dispatcher accepts only kernel permits.
func (deps GitManagementDependencies) executor() gitmanager.OperationExecutor {
	if deps.Executor != nil {
		return deps.Executor
	}
	if deps.Runner == nil || deps.Commands == nil || deps.RandomSource == nil {
		return nil
	}
	return gitmanager.NewOperationDispatcher(gitmanager.DispatcherDependencies{
		Runner: deps.Runner, Commands: deps.Commands, Random: deps.RandomSource,
		Inspect: func(ctx context.Context, projectPath []byte, candidates [][]byte) (gitmanager.GitInspection, error) {
			inspection, _, err := inspectWithEnvironment(ctx, deps, string(projectPath), candidates)
			return inspection, err
		},
	})
}

func unavailableGitManagementDependencies() GitManagementDependencies {
	return GitManagementDependencies{}
}
