package system

import (
	"context"
	"io"
)

// Spec describes one process without attaching domain-specific meaning to it.
type Spec struct {
	Name   string
	Args   []string
	Dir    string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// ExitStatus distinguishes a normal process exit from signal termination.
type ExitStatus struct {
	Exited   bool
	ExitCode int
	Canceled bool
}

// Handle waits for a launched process to stop.
type Handle interface {
	Wait() (ExitStatus, error)
}

// Launcher starts a process and forwards context cancellation to it.
type Launcher interface {
	Launch(ctx context.Context, spec Spec) (Handle, error)
}
