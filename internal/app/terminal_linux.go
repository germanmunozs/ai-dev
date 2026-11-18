//go:build linux

package app

import "syscall"

const terminalAttributesRequest = syscall.TCGETS
