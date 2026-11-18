//go:build darwin

package app

import "syscall"

const terminalAttributesRequest = syscall.TIOCGETA
