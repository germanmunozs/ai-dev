//go:build (darwin && !ios) || (linux && !android)

package configstore

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"

	"github.com/germanmunozs/ai-dev/internal/projectconfig"
)

const tempPrefix = ".agents.json."

type posixStore struct {
	dir string
	ops ops
}

// ops are the filesystem steps, injectable so tests can fail each one.
type ops struct {
	lstat     func(root *os.Root, name string) (fs.FileInfo, error)
	mkdir     func(root *os.Root, name string) error
	openDir   func(root *os.Root, name string) (*os.File, error)
	openTmp   func(root *os.Root, name string) (*os.File, error)
	chmodTmp  func(file *os.File, perm fs.FileMode) error
	writeTmp  func(file *os.File, data []byte) error
	syncTmp   func(file *os.File) error
	closeTmp  func(file *os.File) error
	rename    func(root *os.Root, oldname, newname string) error
	remove    func(root *os.Root, name string) error
	syncDir   func(file *os.File) error
	afterStep func(step string)
}

func newPlatformStore(dir string) Store { return &posixStore{dir: dir, ops: defaultOps()} }

func defaultOps() ops {
	return ops{
		lstat:   func(root *os.Root, name string) (fs.FileInfo, error) { return root.Lstat(name) },
		mkdir:   func(root *os.Root, name string) error { return root.Mkdir(name, 0o755) },
		openDir: func(root *os.Root, name string) (*os.File, error) { return root.Open(name) },
		openTmp: func(root *os.Root, name string) (*os.File, error) {
			return root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		},
		chmodTmp:  func(file *os.File, perm fs.FileMode) error { return file.Chmod(perm) },
		writeTmp:  func(file *os.File, data []byte) error { _, err := file.Write(data); return err },
		syncTmp:   func(file *os.File) error { return file.Sync() },
		closeTmp:  func(file *os.File) error { return file.Close() },
		rename:    func(root *os.Root, oldname, newname string) error { return root.Rename(oldname, newname) },
		remove:    func(root *os.Root, name string) error { return root.Remove(name) },
		syncDir:   func(file *os.File) error { return file.Sync() },
		afterStep: func(string) {},
	}
}

// Write follows research R-03: check .ai-dev (creating it if absent), open
// it as its own root and confirm it is the directory just checked, so every
// later step stays inside that directory even if the path is swapped; refuse
// a non-regular agents.json, then write a temporary file, sync, rename and
// sync the directory. Each refusal says why (review L3, L5).
func (s *posixStore) Write(data []byte) (bool, []string) {
	fail := func(format string, args ...any) (bool, []string) {
		return false, []string{"ai-dev: setup: " + fmt.Sprintf(format, args...)}
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return fail("cannot open the project directory: %v", err)
	}
	defer root.Close()

	dirInfo, err := s.ops.lstat(root, projectconfig.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		// Feature 008 shares .ai-dev and requires mode 0755 on an existing
		// directory, so the mode is set explicitly, whatever the umask.
		if err := s.ops.mkdir(root, projectconfig.Dir); err != nil {
			return fail("cannot create %s: %v", projectconfig.Dir, err)
		}
		if err := root.Chmod(projectconfig.Dir, 0o755); err != nil {
			return fail("cannot set the mode of %s: %v", projectconfig.Dir, err)
		}
		dirInfo, err = s.ops.lstat(root, projectconfig.Dir)
	}
	if err != nil || !dirInfo.IsDir() || dirInfo.Mode()&fs.ModeSymlink != 0 {
		return fail("refused: %s is a symbolic link or not a directory", projectconfig.Dir)
	}
	s.ops.afterStep("dir-checked")
	dir, err := root.OpenRoot(projectconfig.Dir)
	if err != nil {
		return fail("cannot open %s: %v", projectconfig.Dir, err)
	}
	defer dir.Close()
	dirFile, err := s.ops.openDir(dir, ".")
	if err != nil {
		return fail("cannot open %s: %v", projectconfig.Dir, err)
	}
	defer dirFile.Close()
	opened, err := dirFile.Stat()
	if err != nil || !os.SameFile(opened, dirInfo) {
		return fail("refused: %s changed while it was being checked", projectconfig.Dir)
	}

	name := path.Base(projectconfig.FileName)
	perm := fs.FileMode(0o644)
	existing, err := s.ops.lstat(dir, name)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil || !existing.Mode().IsRegular():
		return fail("refused: %s is not a regular file", projectconfig.FileName)
	default:
		perm = existing.Mode().Perm()
	}

	tmpName, err := temporaryName()
	if err != nil {
		return fail("cannot name a temporary file: %v", err)
	}
	tmp, err := s.ops.openTmp(dir, tmpName)
	if err != nil {
		return fail("cannot create a temporary file in %s: %v", projectconfig.Dir, err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = s.ops.remove(dir, tmpName)
		}
	}()
	if err := s.ops.chmodTmp(tmp, perm); err != nil {
		_ = s.ops.closeTmp(tmp)
		return fail("cannot set the temporary file mode: %v", err)
	}
	if err := s.ops.writeTmp(tmp, data); err != nil {
		_ = s.ops.closeTmp(tmp)
		return fail("cannot write the temporary file: %v", err)
	}
	if err := s.ops.syncTmp(tmp); err != nil {
		_ = s.ops.closeTmp(tmp)
		return fail("cannot sync the temporary file: %v", err)
	}
	if err := s.ops.closeTmp(tmp); err != nil {
		return fail("cannot close the temporary file: %v", err)
	}
	if err := s.ops.rename(dir, tmpName, name); err != nil {
		return fail("cannot replace %s: %v", projectconfig.FileName, err)
	}
	cleanup = false
	var warnings []string
	if err := s.ops.syncDir(dirFile); err != nil {
		warnings = append(warnings, fmt.Sprintf("ai-dev: warning: configuration written, but directory flush failed: %v", err))
	}
	return true, warnings
}

// temporaryName is distinct from Feature 008's ".config.json.<hex>.tmp", so
// a leftover after a crash names its owner (review L7).
func temporaryName() (string, error) {
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return tempPrefix + hex.EncodeToString(random) + ".tmp", nil
}
