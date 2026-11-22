//go:build !((darwin && !ios) || (linux && !android))

package gitprocess

import "os/exec"

// isolateProcessGroup has nothing to isolate where Git is never run.
func isolateProcessGroup(*exec.Cmd) {}
