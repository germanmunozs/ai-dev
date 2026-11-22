//go:build darwin || linux

package app

import (
	"io"
	"os"
	"syscall"
	"unsafe"
)

// isTerminal reports whether reader is a terminal device: reading its
// terminal attributes must succeed. A character device such as /dev/null
// is not a terminal.
func isTerminal(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	if !ok {
		return false
	}
	var attributes syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), uintptr(terminalAttributesRequest), uintptr(unsafe.Pointer(&attributes)))
	return errno == 0
}
