//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"strconv"
	"syscall"
)

// readSource reads one source the way Git would, following symlinks, but
// only as a regular file of at most 1 MiB that counts against the
// aggregate budget. The descriptor's identity must be unchanged after the
// read. A missing file is absent; anything else that cannot be read safely
// is unreadable, ambiguous or unsupported, never empty.
func readSource(scope string, path []byte, precedence uint64, aggregate *Budget) sourceRead {
	fail := func(state SourceReadState) sourceRead { return failedSource(scope, path, precedence, state) }
	if len(path) == 0 || path[0] != '/' || CheckPathBytes(path) != nil {
		return fail(SourceReadAmbiguous)
	}
	file, err := os.Open(string(path))
	switch {
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, syscall.ENOTDIR):
		return sourceRead{identity: InspectedSourceIdentity{Scope: scope, Path: NewObservedPath(path), ReadState: SourceReadAbsent, Precedence: precedence}}
	case err != nil:
		return fail(SourceReadUnreadable)
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil {
		return fail(SourceReadUnreadable)
	}
	st, ok := before.Sys().(*syscall.Stat_t)
	if !ok || !before.Mode().IsRegular() {
		return fail(SourceReadAmbiguous)
	}
	if CheckLimit(LimitSourceBytes, before.Size()) != nil {
		return fail(SourceReadUnsupported)
	}
	data, err := io.ReadAll(io.LimitReader(file, LimitSourceBytes.Max()+1))
	if err != nil {
		return fail(SourceReadUnreadable)
	}
	if CheckLimit(LimitSourceBytes, int64(len(data))) != nil || (aggregate != nil && aggregate.Add(int64(len(data))) != nil) {
		return fail(SourceReadUnsupported)
	}
	after, err := file.Stat()
	if err != nil || after.Size() != int64(len(data)) || !after.ModTime().Equal(before.ModTime()) {
		return fail(SourceReadAmbiguous)
	}
	device, inode := strconv.FormatUint(deviceNumber(st), 10), strconv.FormatUint(uint64(st.Ino), 10)
	return sourceRead{
		identity: InspectedSourceIdentity{
			Scope: scope, Path: NewObservedPath(path), PhysicalIdentityDigest: sourceIdentityDigest(device, inode),
			ContentDigest: contentDigest(data), ReadState: SourceReadRead, ObservedSizeDecimal: strconv.Itoa(len(data)),
			Precedence: precedence,
		},
		data: data,
	}
}

// readTreeSource reads an in-tree source (.gitignore, .gitattributes).
// Git refuses to follow a symlink there, so a symlinked source is
// ambiguous rather than read through.
func readTreeSource(scope string, path []byte, precedence uint64, aggregate *Budget) sourceRead {
	if info, err := os.Lstat(string(path)); err == nil && info.Mode()&fs.ModeSymlink != 0 {
		return failedSource(scope, path, precedence, SourceReadAmbiguous)
	}
	return readSource(scope, path, precedence, aggregate)
}
