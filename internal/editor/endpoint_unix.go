//go:build darwin || linux

package editor

import (
	"os"
	"path/filepath"
	"syscall"
)

// ownerOf returns the owner of a file; tests replace it to cover a socket
// owned by another user.
var ownerOf = func(info os.FileInfo) (int, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(stat.Uid), true
}

// CheckEndpoint accepts addr only when it is the absolute path of a Unix
// socket owned by the current user (F014-FR-003, research R-03). A TCP
// address, a symbolic link, a file or a directory is refused, so a stale or
// foreign $NVIM is never dialed.
func CheckEndpoint(addr string) error {
	if !filepath.IsAbs(addr) {
		return &EndpointError{Reason: ReasonNotLocalSocket}
	}
	info, err := os.Lstat(addr)
	if err != nil || info.Mode()&os.ModeSocket == 0 {
		return &EndpointError{Reason: ReasonNotSocket}
	}
	if uid, ok := ownerOf(info); !ok || uid != os.Getuid() {
		return &EndpointError{Reason: ReasonNotOwned}
	}
	return nil
}
