package app

import (
	"context"
	"crypto/rand"
	"os"
	"runtime"

	"ai-dev/internal/gitmanager"
	"ai-dev/internal/gitprocess"
	"ai-dev/internal/gitstore"
)

// DefaultGitManagementDependencies composes Git Management from the real
// implementations (T107). The runner enables apply: every mutation runs
// through the production OperationDispatcher, which accepts only the
// safety kernel's MutationPermit, and the kernel's terminal.go remains the
// sole closer.
func DefaultGitManagementDependencies() GitManagementDependencies {
	return GitManagementDependencies{
		Runner:            gitprocess.Runner{},
		Reader:            gitprocess.Reader{Runner: gitprocess.Runner{}},
		Commands:          gitprocess.Builder{},
		RandomSource:      rand.Reader,
		ObserveFilesystem: gitstore.ObserveFilesystem,
		IdentifyGit: func(ctx context.Context, environment gitmanager.EnvironmentProfile) (gitmanager.GitToolIdentity, error) {
			path, mode, err := gitprocess.LocateGit(os.Getenv("PATH"))
			if err != nil {
				return gitmanager.GitToolIdentity{}, err
			}
			return gitprocess.Identify(ctx, mode, path, nil, environment, []byte("/"))
		},
		OpenStore: func(root gitmanager.ObservedPath) (gitmanager.GitStore, func(), error) {
			store, err := gitstore.Open(root)
			if err != nil {
				return nil, nil, err
			}
			return store, func() { store.Close() }, nil
		},
		LookupEnv:  os.LookupEnv,
		Environ:    os.Environ,
		WorkingDir: os.Getwd,
		GOOS:       runtime.GOOS,
	}
}
