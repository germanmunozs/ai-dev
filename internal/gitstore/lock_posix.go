//go:build (darwin && !ios) || (linux && !android)

package gitstore

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"sync"
	"syscall"

	"ai-dev/internal/gitmanager"
)

// lockHolderSchema versions the diagnostic holder record.
const lockHolderSchema = "ai-dev.git-lock-holder/v1"

// lockKey is the path-free namespace of one physical directory.
func lockKey(device, inode string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte("ai-dev-lock-v1\x00"+device+"\x00"+inode)))
}

// AcquireApplyLock takes the persistent advisory lock of the physical
// project. The file is opened without following symlinks and must be a
// private, singly linked regular file owned by this user; the advisory
// lock must be free; and, while it is held, the lock name must still be
// that file and the project path must still be the identified device and
// inode. Any proof that fails is lock_conflict. An existing file is reused
// in place and never repaired or deleted. The diagnostic holder record is
// rebuilt from these current proofs and grants nothing.
func (s *Store) AcquireApplyLock(_ context.Context, project gitmanager.PhysicalProjectIdentity, owner gitmanager.ApplyID) (gitmanager.ApplyLock, error) {
	if err := project.Validate(); err != nil {
		return nil, err
	}
	if err := owner.Validate(); err != nil {
		return nil, err
	}
	conflict := func(detail string, err error) error {
		return fmt.Errorf("%w: %s: %v", gitmanager.ErrApplyLockHeld, detail, err)
	}
	key := lockKey(project.DeviceDecimal(), project.InodeDecimal())
	dir := path.Join("locks", "by-device-inode", key)
	for _, name := range []string{path.Join("locks", "by-device-inode"), dir} {
		if err := ensurePrivateDirectory(s.root, name); err != nil {
			return nil, conflict("lock directory", err)
		}
	}
	name := path.Join(dir, "apply.lock")
	file, err := s.openLockFile(dir, name)
	if err != nil {
		return nil, conflict("lock file", err)
	}
	release := func(err error) (gitmanager.ApplyLock, error) {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		file.Close()
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, conflict("active holder", err)
		}
		return nil, conflict("advisory lock", err)
	}
	if err := s.sameLockFile(file, name); err != nil {
		return release(conflict("lock file changed while locking", err))
	}
	if err := proveProject(project, key); err != nil {
		return release(conflict("project identity", err))
	}
	holder, err := gitmanager.MarshalCanonical(map[string]any{
		"schema_version":          lockHolderSchema,
		"lock_key":                key,
		"apply_id":                string(owner),
		"device_decimal":          project.DeviceDecimal(),
		"inode_decimal":           project.InodeDecimal(),
		"project_identity_digest": string(project.Digest()),
	})
	if err == nil {
		err = s.records.put(dir, "holder.json", holder)
	}
	if err != nil {
		return release(conflict("diagnostic holder record", err))
	}
	return &applyLock{store: s, file: file, name: name}, nil
}

// openLockFile opens the existing lock file, or creates it exclusively with
// mode 0600 and makes the new name durable.
func (s *Store) openLockFile(dir, name string) (*os.File, error) {
	file, err := s.root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o600)
	if err == nil {
		if err = file.Chmod(0o600); err == nil {
			if err = file.Sync(); err == nil {
				err = s.records.syncDirectory(dir)
			}
		}
		if err != nil {
			file.Close()
			return nil, err
		}
		return file, nil
	}
	if !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	file, err = s.root.OpenFile(name, os.O_RDWR|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err == nil {
		st, ok := info.Sys().(*syscall.Stat_t)
		switch {
		case !ok || !info.Mode().IsRegular():
			err = errors.New("not a regular file")
		case info.Mode().Perm() != 0o600:
			err = fmt.Errorf("mode %04o, not 0600", info.Mode().Perm())
		case int(st.Uid) != os.Geteuid():
			err = errors.New("not owned by the current user")
		case st.Nlink != 1:
			err = fmt.Errorf("%d hard links", st.Nlink)
		}
	}
	if err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}

// sameLockFile proves the lock name still refers to the held descriptor.
func (s *Store) sameLockFile(file *os.File, name string) error {
	held, err := file.Stat()
	if err != nil {
		return err
	}
	named, err := s.root.Lstat(name)
	if err != nil {
		return err
	}
	if !os.SameFile(held, named) {
		return gitmanager.ErrOwnedResourceReplaced
	}
	return nil
}

// proveProject opens the project path as a directory and proves, through
// the descriptor, that it is the identified device and inode and that the
// lock namespace key derives from them.
func proveProject(project gitmanager.PhysicalProjectIdentity, key string) error {
	dir, err := os.OpenFile(string(project.ReversiblePathBytes()), os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_DIRECTORY, 0)
	if err != nil {
		return err
	}
	defer dir.Close()
	info, err := dir.Stat()
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() {
		return errors.New("project is not a directory")
	}
	device, inode := strconv.FormatUint(deviceNumber(st), 10), strconv.FormatUint(uint64(st.Ino), 10)
	if device != project.DeviceDecimal() || inode != project.InodeDecimal() || lockKey(device, inode) != key {
		return errors.New("project path is not the identified physical directory")
	}
	return nil
}

// applyLock is one held apply lock.
type applyLock struct {
	mu       sync.Mutex
	store    *Store
	file     *os.File
	name     string
	released bool
}

// Release unlocks and closes the held descriptor once. The persistent file
// stays. If the lock name no longer refers to the held file the descriptor
// is still unlocked, but ErrOwnedResourceReplaced reports that ownership of
// the namespace could not be verified.
func (l *applyLock) Release(context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.released {
		return errors.New("apply lock already released")
	}
	l.released = true
	ownership := l.store.sameLockFile(l.file, l.name)
	unlock := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	if ownership != nil {
		return fmt.Errorf("%w: apply lock: %v", gitmanager.ErrOwnedResourceReplaced, ownership)
	}
	if unlock != nil {
		return unlock
	}
	return closeErr
}
