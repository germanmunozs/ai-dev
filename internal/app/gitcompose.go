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

// DefaultGitManagementDependencies composes the read-only inspection
// capabilities from the real implementations. It supplies no store and no
// mutating port.
func DefaultGitManagementDependencies() GitManagementDependencies {
	return GitManagementDependencies{
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
		LookupEnv:  os.LookupEnv,
		Environ:    os.Environ,
		WorkingDir: os.Getwd,
		GOOS:       runtime.GOOS,
	}
}
