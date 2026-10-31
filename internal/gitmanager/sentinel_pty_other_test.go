//go:build !((darwin && !ios) || (linux && !android))

package gitmanager

import (
	"fmt"
	"os"
	"syscall"
)

func openPseudoTerminal() (*os.File, *os.File, error) {
	return nil, nil, fmt.Errorf("pseudo-terminal sentinel test unsupported on this platform")
}

func pseudoTerminalProcessAttributes() *syscall.SysProcAttr {
	return nil
}

func pseudoTerminalNonzeroProcessGroupAttributesForTest() *syscall.SysProcAttr {
	return nil
}

func pseudoTerminalProcessGroupID(process *os.Process, attributes *syscall.SysProcAttr) (int, error) {
	return pseudoTerminalProcessGroupIDWithLookup(process, attributes, nil, nil)
}

func pseudoTerminalProcessGroupIDWithLookup(
	process *os.Process,
	attributes *syscall.SysProcAttr,
	_ func(int) (int, error),
	_ func(*os.Process) error,
) (int, error) {
	if process == nil || process.Pid <= 1 {
		return 0, fmt.Errorf("unsafe process ID for PTY process group: %v", process)
	}
	if attributes != nil {
		return 0, fmt.Errorf("unexpected PTY process-group attributes on unsupported platform")
	}
	return process.Pid, nil
}

func pseudoTerminalProcessIsUnreaped(*os.Process) error {
	return fmt.Errorf("PTY process lifecycle probing unsupported on this platform")
}

func terminatePseudoTerminalProcessGroup(groupID int) error {
	return terminatePseudoTerminalProcessGroupWith(groupID, func(validGroupID int) error {
		process, err := os.FindProcess(validGroupID)
		if err != nil {
			return err
		}
		return process.Kill()
	})
}

func terminatePseudoTerminalProcessGroupWith(groupID int, terminate func(int) error) error {
	if groupID <= 1 {
		return fmt.Errorf("refusing to signal unsafe PTY process group %d", groupID)
	}
	if terminate == nil {
		return fmt.Errorf("missing PTY process-group terminator")
	}
	return terminate(groupID)
}

func pseudoTerminalProcessExists(int) bool {
	return false
}

func setPseudoTerminalNonblocking(*os.File) error {
	return fmt.Errorf("nonblocking pseudo-terminal reads unsupported on this platform")
}

func readPseudoTerminalNonblocking(*os.File, []byte) (int, error) {
	return 0, fmt.Errorf("nonblocking pseudo-terminal reads unsupported on this platform")
}

func pollPseudoTerminalReadable(*os.File) (bool, error) {
	return false, fmt.Errorf("pseudo-terminal polling unsupported on this platform")
}

func pseudoTerminalReadWouldBlock(error) bool {
	return false
}

func pseudoTerminalReadComplete(error) bool {
	return false
}

func pollPseudoTerminalProcess(*os.Process) (bool, error) {
	return false, fmt.Errorf("pseudo-terminal process polling unsupported on this platform")
}

func detachPseudoTerminalSession() error {
	return fmt.Errorf("session detachment unsupported on this platform")
}
