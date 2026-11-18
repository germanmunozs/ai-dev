//go:build darwin && !ios

package gitmanager

import "syscall"

// deviceNumber reads Darwin's signed 32-bit st_dev as its unsigned bit
// pattern.
func deviceNumber(st *syscall.Stat_t) uint64 { return uint64(uint32(st.Dev)) }

func statMtime(st *syscall.Stat_t) syscall.Timespec { return st.Mtimespec }
func statCtime(st *syscall.Stat_t) syscall.Timespec { return st.Ctimespec }
