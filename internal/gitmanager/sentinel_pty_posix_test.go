//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"syscall"
	"unsafe"
)

func openPseudoTerminal() (*os.File, *os.File, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}
	fail := func(err error) (*os.File, *os.File, error) {
		_ = master.Close()
		return nil, nil, err
	}

	var slavePath string
	switch runtime.GOOS {
	case "darwin":
		const (
			tiocptyunlk  = uintptr(0x20007452)
			tiocptygname = uintptr(0x40807453)
			tiocptygrant = uintptr(0x20007454)
		)
		if err := pseudoTerminalIOCTL(master.Fd(), tiocptygrant, nil); err != nil {
			return fail(fmt.Errorf("grant pty: %w", err))
		}
		if err := pseudoTerminalIOCTL(master.Fd(), tiocptyunlk, nil); err != nil {
			return fail(fmt.Errorf("unlock pty: %w", err))
		}
		var name [128]byte
		if err := pseudoTerminalIOCTL(master.Fd(), tiocptygname, unsafe.Pointer(&name[0])); err != nil {
			return fail(fmt.Errorf("get pty name: %w", err))
		}
		end := 0
		for end < len(name) && name[end] != 0 {
			end++
		}
		slavePath = string(name[:end])
	case "linux":
		const (
			tiocsptlck = uintptr(0x40045431)
			tiocgptn   = uintptr(0x80045430)
		)
		var unlock int32
		if err := pseudoTerminalIOCTL(master.Fd(), tiocsptlck, unsafe.Pointer(&unlock)); err != nil {
			return fail(fmt.Errorf("unlock pty: %w", err))
		}
		var number uint32
		if err := pseudoTerminalIOCTL(master.Fd(), tiocgptn, unsafe.Pointer(&number)); err != nil {
			return fail(fmt.Errorf("get pty number: %w", err))
		}
		slavePath = fmt.Sprintf("/dev/pts/%d", number)
	default:
		return fail(fmt.Errorf("unsupported pty platform %s", runtime.GOOS))
	}

	slave, err := os.OpenFile(slavePath, os.O_RDWR, 0)
	if err != nil {
		return fail(err)
	}
	return master, slave, nil
}

func pseudoTerminalIOCTL(fd, request uintptr, argument unsafe.Pointer) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, request, uintptr(argument))
	runtime.KeepAlive(argument)
	if errno != 0 {
		return errno
	}
	return nil
}

func pseudoTerminalProcessAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

func pseudoTerminalNonzeroProcessGroupAttributesForTest() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true, Pgid: 7}
}

func pseudoTerminalProcessGroupID(process *os.Process, attributes *syscall.SysProcAttr) (int, error) {
	return pseudoTerminalProcessGroupIDWithLookup(process, attributes, syscall.Getpgid, pseudoTerminalProcessIsUnreaped)
}

func pseudoTerminalProcessGroupIDWithLookup(
	process *os.Process,
	attributes *syscall.SysProcAttr,
	lookup func(int) (int, error),
	checkUnreaped func(*os.Process) error,
) (int, error) {
	if process == nil || process.Pid <= 1 {
		return 0, fmt.Errorf("unsafe process ID for PTY process group: %v", process)
	}
	if attributes == nil || !attributes.Setpgid || attributes.Pgid != 0 {
		return 0, fmt.Errorf("process %d lacks isolated PTY process-group attributes", process.Pid)
	}
	if lookup == nil || checkUnreaped == nil {
		return 0, fmt.Errorf("process %d lacks PTY process-group identity probes", process.Pid)
	}
	if err := checkUnreaped(process); err != nil {
		return 0, fmt.Errorf("process %d is not an unreaped PTY group leader: %w", process.Pid, err)
	}
	groupID, err := lookup(process.Pid)
	if errors.Is(err, syscall.ESRCH) {
		// Start succeeded with Setpgid=true and Pgid=0, so the Go child setup
		// established PGID=PID before exec. On Darwin Getpgid may report ESRCH
		// for an exited leader. The lifecycle probe above proves this specific
		// os.Process has not been waited on or released, so its PID is reserved.
		return process.Pid, nil
	}
	if err != nil {
		return 0, fmt.Errorf("resolve PTY process group for PID %d: %w", process.Pid, err)
	}
	if groupID <= 1 || groupID != process.Pid {
		return 0, fmt.Errorf("unsafe PTY process group %d for leader PID %d", groupID, process.Pid)
	}
	return groupID, nil
}

func pseudoTerminalProcessIsUnreaped(process *os.Process) error {
	return process.Signal(syscall.Signal(0))
}

func terminatePseudoTerminalProcessGroup(groupID int) error {
	return terminatePseudoTerminalProcessGroupWith(groupID, func(validGroupID int) error {
		err := syscall.Kill(-validGroupID, syscall.SIGKILL)
		if err == syscall.ESRCH {
			return nil
		}
		return err
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

func pseudoTerminalProcessExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

func setPseudoTerminalNonblocking(file *os.File) error {
	return syscall.SetNonblock(int(file.Fd()), true)
}

func readPseudoTerminalNonblocking(file *os.File, buffer []byte) (int, error) {
	return syscall.Read(int(file.Fd()), buffer)
}

func pollPseudoTerminalReadable(file *os.File) (bool, error) {
	fd := int(file.Fd())
	var readSet syscall.FdSet
	bits := reflect.ValueOf(&readSet.Bits).Elem()
	wordBits := int(bits.Type().Elem().Bits())
	wordIndex := fd / wordBits
	if wordIndex < 0 || wordIndex >= bits.Len() {
		return false, fmt.Errorf("PTY descriptor %d exceeds select set capacity", fd)
	}
	word := bits.Index(wordIndex)
	mask := int64(uint64(1) << uint(fd%wordBits))
	word.SetInt(word.Int() | mask)
	timeout := syscall.Timeval{}
	selectFunction := reflect.ValueOf(syscall.Select)
	results := selectFunction.Call([]reflect.Value{
		reflect.ValueOf(fd + 1),
		reflect.ValueOf(&readSet),
		reflect.Zero(selectFunction.Type().In(2)),
		reflect.Zero(selectFunction.Type().In(3)),
		reflect.ValueOf(&timeout),
	})
	last := results[len(results)-1]
	if !last.IsNil() && errors.Is(last.Interface().(error), syscall.EINTR) {
		return false, nil
	}
	if !last.IsNil() {
		return false, last.Interface().(error)
	}
	return bits.Index(wordIndex).Int()&mask != 0, nil
}

func pseudoTerminalReadWouldBlock(err error) bool {
	return errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK)
}

func pseudoTerminalReadComplete(err error) bool {
	return errors.Is(err, syscall.EIO)
}

func pollPseudoTerminalProcess(process *os.Process) (bool, error) {
	var status syscall.WaitStatus
	pid, err := syscall.Wait4(process.Pid, &status, syscall.WNOHANG, nil)
	if errors.Is(err, syscall.EINTR) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if pid == 0 {
		return false, nil
	}

	var statusErr error
	switch {
	case status.Exited() && status.ExitStatus() == 0:
	case status.Exited():
		statusErr = fmt.Errorf("exit status %d", status.ExitStatus())
	case status.Signaled():
		statusErr = fmt.Errorf("signal: %s", status.Signal())
	default:
		statusErr = fmt.Errorf("unexpected process status %#x", uint32(status))
	}
	return true, errors.Join(statusErr, process.Release())
}

func detachPseudoTerminalSession() error {
	_, err := syscall.Setsid()
	return err
}
