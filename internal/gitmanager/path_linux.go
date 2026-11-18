//go:build linux && !android

package gitmanager

import "syscall"

func deviceNumber(st *syscall.Stat_t) uint64 { return uint64(st.Dev) }
