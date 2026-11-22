//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"errors"
	"io"
	"os"
	"syscall"
)

// fileOps are the filesystem steps of the dispatcher's own mutations: the
// index.lock protocol, the prepared index and the project configuration.
// Tests substitute single steps to inject faults; production uses osFiles.
type fileOps struct {
	openFile func(path string, flag int, perm os.FileMode) (*os.File, error)
	fstat    func(*os.File) (syscall.Stat_t, error)
	chmod    func(*os.File, os.FileMode) error
	write    func(*os.File, []byte) (int, error)
	sync     func(*os.File) error
	// readFile reads a regular file without following a symlink and
	// returns the identity of what it read.
	readFile func(path string) ([]byte, syscall.Stat_t, error)
	mkdir    func(path string, perm os.FileMode) error
	rename   func(oldpath, newpath string) error
	link     func(oldpath, newpath string) error
	remove   func(path string) error
	syncDir  func(path string) error
}

var osFiles = fileOps{
	openFile: os.OpenFile,
	fstat:    descriptorStat,
	chmod:    func(f *os.File, mode os.FileMode) error { return f.Chmod(mode) },
	write:    func(f *os.File, data []byte) (int, error) { return f.Write(data) },
	sync:     func(f *os.File) error { return f.Sync() },
	readFile: readNoFollow,
	mkdir:    os.Mkdir,
	rename:   os.Rename,
	link:     os.Link,
	remove:   os.Remove,
	syncDir:  syncDirectory,
}

// files returns the dispatcher's filesystem steps.
func (d *OperationDispatcher) files() fileOps {
	if d.fileOps == nil {
		return osFiles
	}
	return *d.fileOps
}

// errCreatedUnverified reports an effect this attempt may have made but
// cannot prove: an exclusively created entry whose identity could then not
// be read, or a publication that reported failure after it may have
// happened. Nothing unproven is removed, and the operation is
// indeterminate rather than failed.
var errCreatedUnverified = errors.New("an exclusively created entry could not be identified")

func readNoFollow(path string) ([]byte, syscall.Stat_t, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, syscall.Stat_t{}, err
	}
	defer file.Close()
	identity, err := descriptorStat(file)
	if err != nil {
		return nil, syscall.Stat_t{}, err
	}
	data, err := io.ReadAll(file)
	return data, identity, err
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
