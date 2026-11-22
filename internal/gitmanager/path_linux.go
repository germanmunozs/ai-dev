//go:build linux && !android

package gitmanager

import "syscall"

func deviceNumber(st *syscall.Stat_t) uint64 { return uint64(st.Dev) }

func statMtime(st *syscall.Stat_t) syscall.Timespec { return st.Mtim }
func statCtime(st *syscall.Stat_t) syscall.Timespec { return st.Ctim }
