//go:build linux && !android

package gitstore

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// statfs(2) f_flags bits.
const (
	stReadOnly    = 0x0001
	stNoSuid      = 0x0002
	stNoDev       = 0x0004
	stNoExec      = 0x0008
	stSynchronous = 0x0010
)

// remoteFilesystems are network or userspace filesystems; anything else
// reported by mountinfo is block-device backed or virtual and local.
var remoteFilesystems = map[string]bool{
	"nfs": true, "nfs4": true, "cifs": true, "smb3": true, "smbfs": true, "9p": true, "ceph": true,
	"glusterfs": true, "afs": true, "lustre": true, "gpfs": true, "virtiofs": true,
}

// observeMount reads the exact filesystem type from /proc/self/mountinfo,
// because statfs reports one magic for ext2, ext3 and ext4, and the flags
// from statfs. The mount is the longest mount point containing path whose
// device matches path's device.
func observeMount(path string) (mountFacts, error) {
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		return mountFacts{}, fmt.Errorf("stat %s: %w", path, err)
	}
	major, minor := (st.Dev>>8)&0xfff|(st.Dev>>32)&^0xfff, st.Dev&0xff|(st.Dev>>12)&^0xff
	device := strconv.FormatUint(major, 10) + ":" + strconv.FormatUint(minor, 10)
	file, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return mountFacts{}, fmt.Errorf("read mountinfo: %w", err)
	}
	defer file.Close()
	var best mountFacts
	bestLength := -1
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		separator := -1
		for i, field := range fields {
			if field == "-" {
				separator = i
				break
			}
		}
		if len(fields) < 5 || separator < 0 || separator+1 >= len(fields) || fields[2] != device {
			continue
		}
		mountPoint := unescapeMountinfo(fields[4])
		if !strings.HasPrefix(path+"/", strings.TrimSuffix(mountPoint, "/")+"/") || len(mountPoint) <= bestLength {
			continue
		}
		fsType := fields[separator+1]
		best = mountFacts{fsType: fsType, identity: "mount=" + fields[0] + " dev=" + device + " " + mountPoint,
			local: !remoteFilesystems[fsType] && !strings.HasPrefix(fsType, "fuse")}
		bestLength = len(mountPoint)
	}
	if err := scanner.Err(); err != nil {
		return mountFacts{}, err
	}
	if bestLength < 0 {
		return mountFacts{}, errors.New("no mountinfo entry for " + path)
	}
	var sfs syscall.Statfs_t
	if err := syscall.Statfs(path, &sfs); err != nil {
		return mountFacts{}, fmt.Errorf("statfs %s: %w", path, err)
	}
	best.readOnly = sfs.Flags&stReadOnly != 0
	if best.local {
		best.flags = append(best.flags, "local")
	}
	for _, flag := range []struct {
		bit  int64
		name string
	}{{stReadOnly, "read_only"}, {stSynchronous, "synchronous"}, {stNoExec, "noexec"}, {stNoSuid, "nosuid"}, {stNoDev, "nodev"}} {
		if sfs.Flags&flag.bit != 0 {
			best.flags = append(best.flags, flag.name)
		}
	}
	return best, nil
}

// unescapeMountinfo decodes the octal escapes mountinfo uses for space,
// tab, newline and backslash.
func unescapeMountinfo(value string) string {
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+3 < len(value) {
			if n, err := strconv.ParseUint(value[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(value[i])
	}
	return b.String()
}

func deviceNumber(st *syscall.Stat_t) uint64 { return uint64(st.Dev) }
