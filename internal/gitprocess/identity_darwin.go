//go:build darwin && !ios

package gitprocess

import (
	"fmt"
	"io/fs"
	"strconv"
	"syscall"
)

// deviceInode reports the decimal device and inode of a file. Darwin's
// st_dev is a signed 32-bit field; it is read as its unsigned bit pattern.
func deviceInode(info fs.FileInfo) (string, string, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", "", fmt.Errorf("no device and inode for %s", info.Name())
	}
	return strconv.FormatUint(uint64(uint32(stat.Dev)), 10), strconv.FormatUint(stat.Ino, 10), nil
}
