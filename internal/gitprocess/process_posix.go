//go:build (darwin && !ios) || (linux && !android)

package gitprocess

import (
	"os/exec"
	"syscall"
)

// isolateProcessGroup starts Git in its own process group, so a terminal
// interrupt reaches ai-dev, which defers it (INT-01), but never kills Git in
// the middle of an operation.
func isolateProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
