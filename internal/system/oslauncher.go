package system

import (
	"context"
	"os"
	"os/exec"
	"sync/atomic"
	"time"
)

const processStopGracePeriod = 5 * time.Second

// OSLauncher starts and supervises operating-system processes.
type OSLauncher struct{}

func (OSLauncher) Launch(ctx context.Context, spec Spec) (Handle, error) {
	cmd := exec.CommandContext(ctx, spec.Name, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Stdin = spec.Stdin
	cmd.Stdout = spec.Stdout
	cmd.Stderr = spec.Stderr
	handle := &osHandle{cmd: cmd}
	cmd.Cancel = func() error {
		err := cmd.Process.Signal(os.Interrupt)
		if err == nil {
			handle.canceled.Store(true)
		}
		if err != nil && cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = processStopGracePeriod
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return handle, nil
}

type osHandle struct {
	cmd      *exec.Cmd
	canceled atomic.Bool
}

func (h *osHandle) Wait() (ExitStatus, error) {
	err := h.cmd.Wait()
	if h.cmd.ProcessState == nil {
		return ExitStatus{}, err
	}
	return ExitStatus{
		Exited:   h.cmd.ProcessState.Exited(),
		ExitCode: h.cmd.ProcessState.ExitCode(),
		Canceled: h.canceled.Load(),
	}, err
}
