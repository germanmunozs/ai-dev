//go:build linux && !android

package gitprocess

import (
	"fmt"
	"io/fs"
	"strconv"
	"syscall"
)

// deviceInode reports the decimal device and inode of a file.
func deviceInode(info fs.FileInfo) (string, string, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", "", fmt.Errorf("no device and inode for %s", info.Name())
	}
	return strconv.FormatUint(uint64(stat.Dev), 10), strconv.FormatUint(uint64(stat.Ino), 10), nil
}
