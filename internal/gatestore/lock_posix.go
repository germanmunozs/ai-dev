//go:build (darwin && !ios) || (linux && !android)

package gatestore

import (
	"errors"
	"os"
	"syscall"
)

type lockResult int

const (
	lockAcquired lockResult = iota + 1
	lockHeld
	lockAmbiguous
)

func flockDir(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

func classifyLock(err error) lockResult {
	if err == nil {
		return lockAcquired
	}
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return lockHeld
	}
	return lockAmbiguous
}
