//go:build darwin && !ios

package gitstore

import (
	"fmt"
	"strconv"
	"syscall"
)

// Mount flags from <sys/mount.h>; the syscall package does not export them.
const (
	mntReadOnly    = 0x00000001
	mntSynchronous = 0x00000002
	mntNoExec      = 0x00000004
	mntNoSuid      = 0x00000008
	mntNoDev       = 0x00000010
	mntLocal       = 0x00001000
	mntJournaled   = 0x00800000
)

// observeMount reads the filesystem type, mount identity and flags of the
// mount holding path.
func observeMount(path string) (mountFacts, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return mountFacts{}, fmt.Errorf("statfs %s: %w", path, err)
	}
	facts := mountFacts{
		fsType:   cString(st.Fstypename[:]),
		identity: cString(st.Mntonname[:]) + " fsid=" + strconv.FormatInt(int64(st.Fsid.Val[0]), 10) + ":" + strconv.FormatInt(int64(st.Fsid.Val[1]), 10),
		local:    st.Flags&mntLocal != 0,
		readOnly: st.Flags&mntReadOnly != 0,
	}
	for _, flag := range []struct {
		bit  uint32
		name string
	}{{mntLocal, "local"}, {mntReadOnly, "read_only"}, {mntSynchronous, "synchronous"}, {mntNoExec, "noexec"},
		{mntNoSuid, "nosuid"}, {mntNoDev, "nodev"}, {mntJournaled, "journaled"}} {
		if st.Flags&flag.bit != 0 {
			facts.flags = append(facts.flags, flag.name)
		}
	}
	return facts, nil
}

func cString(value []int8) string {
	b := make([]byte, 0, len(value))
	for _, c := range value {
		if c == 0 {
			break
		}
		b = append(b, byte(c))
	}
	return string(b)
}

// deviceNumber reads Darwin's signed 32-bit st_dev as its unsigned bit
// pattern.
func deviceNumber(st *syscall.Stat_t) uint64 { return uint64(uint32(st.Dev)) }
