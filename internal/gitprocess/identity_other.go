//go:build !((darwin && !ios) || (linux && !android))

package gitprocess

import (
	"errors"
	"io/fs"
)

// deviceInode fails closed where the toolchain cannot be identified by
// device and inode; plan and apply are unsupported there.
func deviceInode(fs.FileInfo) (string, string, error) {
	return "", "", errors.New("file identity is unsupported on this platform")
}
