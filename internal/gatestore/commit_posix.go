//go:build (darwin && !ios) || (linux && !android)

package gatestore

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"

	"ai-dev/internal/gate"
	"ai-dev/internal/workflow"
)

type posixStore struct {
	dir string
	ops ops
}

func newPlatformStore(dir string) gate.Store { return &posixStore{dir: dir, ops: defaultOps()} }

func defaultOps() ops {
	return ops{
		openRoot: os.OpenRoot,
		openDir:  func(root *os.Root, name string) (*os.File, error) { return root.Open(name) },
		lstat:    func(root *os.Root, name string) (fs.FileInfo, error) { return root.Lstat(name) },
		flock:    flockDir,
		readFile: func(root *os.Root, name string) ([]byte, error) { return root.ReadFile(name) },
		openTmp: func(root *os.Root, name string, flag int, perm fs.FileMode) (*os.File, error) {
			return root.OpenFile(name, flag, perm)
		},
		chmodTmp:  func(file *os.File, perm fs.FileMode) error { return file.Chmod(perm) },
		writeTmp:  func(file *os.File, data []byte) error { _, err := file.Write(data); return err },
		syncTmp:   func(file *os.File) error { return file.Sync() },
		closeTmp:  func(file *os.File) error { return file.Close() },
		rename:    func(root *os.Root, oldname, newname string) error { return root.Rename(oldname, newname) },
		remove:    func(root *os.Root, name string) error { return root.Remove(name) },
		syncDir:   func(file *os.File) error { return file.Sync() },
		closeDir:  func(file *os.File) error { return file.Close() },
		afterStep: func(string) {},
	}
}

func (s *posixStore) Commit(featureID string, expected workflow.State, entry gate.Evidence) (status gate.CommitStatus, warnings []string) {
	dir, ok := workflow.FeatureDir(featureID)
	if !ok {
		return gate.Failed, nil
	}
	root, err := s.ops.openRoot(s.dir)
	if err != nil {
		return gate.Failed, nil
	}
	defer root.Close()
	dirFile, err := s.ops.openDir(root, dir)
	if err != nil {
		return gate.Failed, nil
	}
	defer func() {
		if err := s.ops.closeDir(dirFile); err != nil {
			if status == gate.Committed {
				warnings = append(warnings, fmt.Sprintf("ai-dev: warning: committed, but directory close failed: %v", err))
			} else {
				warnings = append(warnings, fmt.Sprintf("ai-dev: warning: directory close failed: %v", err))
			}
		}
	}()

	switch classifyLock(s.ops.flock(dirFile)) {
	case lockHeld:
		return gate.Conflict, nil
	case lockAmbiguous:
		return gate.Failed, nil
	}
	s.ops.afterStep("locked")
	openedInfo, err := dirFile.Stat()
	if err != nil {
		return gate.Conflict, nil
	}
	pathInfo, err := s.ops.lstat(root, dir)
	if err != nil || !pathInfo.IsDir() || pathInfo.Mode()&fs.ModeSymlink != 0 || !os.SameFile(openedInfo, pathInfo) {
		return gate.Conflict, nil
	}
	rootFS := root.FS()
	if !workflow.FeatureExists(rootFS, featureID) {
		return gate.Conflict, nil
	}
	current, invalid := workflow.ResolveRecord(rootFS, featureID)
	if invalid || current != expected {
		return gate.Conflict, nil
	}

	recordPath := path.Join(dir, workflow.RecordFileName)
	existing, err := s.ops.readFile(root, recordPath)
	exists := true
	perm := fs.FileMode(0o644)
	if errors.Is(err, fs.ErrNotExist) {
		exists = false
		existing = nil
	} else if err != nil {
		return gate.Failed, nil
	} else {
		info, err := s.ops.lstat(root, recordPath)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&fs.ModeSymlink != 0 {
			return gate.Failed, nil
		}
		perm = info.Mode().Perm()
	}
	data, err := buildRecord(existing, exists, entry.ToState, entry)
	if err != nil {
		return gate.Failed, nil
	}
	tmpPath, err := temporaryPath(dir)
	if err != nil {
		return gate.Failed, nil
	}
	tmp, err := s.ops.openTmp(root, tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return gate.Failed, nil
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = s.ops.remove(root, tmpPath)
		}
	}()
	if exists {
		if err := s.ops.chmodTmp(tmp, perm); err != nil {
			_ = s.ops.closeTmp(tmp)
			return gate.Failed, nil
		}
	}
	if err := s.ops.writeTmp(tmp, data); err != nil {
		_ = s.ops.closeTmp(tmp)
		return gate.Failed, nil
	}
	if err := s.ops.syncTmp(tmp); err != nil {
		_ = s.ops.closeTmp(tmp)
		return gate.Failed, nil
	}
	if err := s.ops.closeTmp(tmp); err != nil {
		return gate.Failed, nil
	}
	s.ops.afterStep("tmp-written")
	if err := s.ops.rename(root, tmpPath, recordPath); err != nil {
		return gate.Failed, nil
	}
	cleanup = false
	s.ops.afterStep("renamed")
	status = gate.Committed
	if err := s.ops.syncDir(dirFile); err != nil {
		warnings = append(warnings, fmt.Sprintf("ai-dev: warning: committed, but directory flush failed: %v", err))
	}
	s.ops.afterStep("dir-synced")
	return status, warnings
}

func temporaryPath(dir string) (string, error) {
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return path.Join(dir, tempPrefix+hex.EncodeToString(random)+".tmp"), nil
}
