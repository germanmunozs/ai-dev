//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// errLiteralFile reports an allowlisted path that is not, or is no longer,
// one literal regular file beneath the physical root.
var errLiteralFile = errors.New("not a literal regular file beneath the project root")

// readLiteralFile reads one allowlisted file (F008-FR-087, 089, 191 Phase 1
// steps 1–5). Every directory component is opened without following a
// symlink, the file is opened non-blocking so a FIFO cannot stall, and its
// type and identity come from the descriptor. After the read the file's
// metadata must be unchanged and every component must still be the
// directory that was opened, so the bytes were read from the literal path.
// The standard library has no portable openat, so the components are
// re-proven by identity instead of being opened relative to each other.
func readLiteralFile(root []byte, rel string, limit int64) (FileContent, error) {
	flags := syscall.O_RDONLY | syscall.O_NOFOLLOW | syscall.O_CLOEXEC
	parts := strings.Split(rel, "/")
	paths := []string{string(root)}
	for i := range parts {
		paths = append(paths, string(root)+"/"+strings.Join(parts[:i+1], "/"))
	}
	var directories []*os.File
	defer func() {
		for _, dir := range directories {
			dir.Close()
		}
	}()
	for _, path := range paths[:len(paths)-1] {
		dir, err := os.OpenFile(path, flags|syscall.O_DIRECTORY, 0)
		if err != nil {
			return FileContent{}, errLiteralFile
		}
		directories = append(directories, dir)
	}
	file, err := os.OpenFile(paths[len(paths)-1], flags|syscall.O_NONBLOCK, 0)
	if err != nil {
		return FileContent{}, errLiteralFile
	}
	defer file.Close()
	before, err := descriptorStat(file)
	if err != nil || before.Mode&syscall.S_IFMT != syscall.S_IFREG {
		return FileContent{}, errLiteralFile
	}
	if before.Size > limit {
		return FileContent{}, &LimitError{Kind: LimitAuthorizedFileBytes, Reason: ReasonRepositoryStateIncompatible, Max: limit, Observed: before.Size}
	}
	data, err := io.ReadAll(io.LimitReader(file, before.Size+1))
	after, statErr := descriptorStat(file)
	if err != nil || statErr != nil || int64(len(data)) != before.Size || !sameMetadata(before, after) {
		return FileContent{}, errLiteralFile
	}
	for i, dir := range append(directories, file) {
		opened, err := descriptorStat(dir)
		var named syscall.Stat_t
		if err != nil || syscall.Lstat(paths[i], &named) != nil || deviceNumber(&opened) != deviceNumber(&named) || opened.Ino != named.Ino {
			return FileContent{}, errLiteralFile
		}
	}
	info, err := file.Stat()
	if err != nil {
		return FileContent{}, errLiteralFile
	}
	return FileContent{Identity: FileIdentity{
		DeviceDecimal: strconv.FormatUint(deviceNumber(&after), 10), InodeDecimal: strconv.FormatUint(uint64(after.Ino), 10),
		Mode: info.Mode(), Size: after.Size,
	}, Data: data}, nil
}

func descriptorStat(file *os.File) (syscall.Stat_t, error) {
	var st syscall.Stat_t
	err := syscall.Fstat(int(file.Fd()), &st)
	return st, err
}

// sameMetadata is the before/after barrier of one descriptor read.
func sameMetadata(a, b syscall.Stat_t) bool {
	return deviceNumber(&a) == deviceNumber(&b) && a.Ino == b.Ino && a.Mode == b.Mode && a.Size == b.Size &&
		a.Nlink == b.Nlink && statMtime(&a) == statMtime(&b) && statCtime(&a) == statCtime(&b)
}
