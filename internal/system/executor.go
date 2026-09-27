// Package system contains the application's narrow external-process boundary.
package system

import (
	"context"
	"os/exec"
)

// Executor locates executables and runs them with fixed argument vectors.
type Executor interface {
	LookPath(file string) (string, error)
	Run(ctx context.Context, name string, args ...string) (string, error)
}

// OSExecutor delegates process operations to os/exec.
type OSExecutor struct{}

func (OSExecutor) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (OSExecutor) Run(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}
