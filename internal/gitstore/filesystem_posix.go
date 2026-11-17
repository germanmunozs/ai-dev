//go:build (darwin && !ios) || (linux && !android)

package gitstore

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"ai-dev/internal/gitmanager"
)

// mountFacts is what the per-OS adapter learns about a mount.
type mountFacts struct {
	fsType   string
	identity string
	flags    []string
	local    bool
	readOnly bool
}

// ObserveFilesystem reports the filesystem holding an existing absolute
// path, with the path's own device and inode.
func ObserveFilesystem(path string) (gitmanager.FilesystemObservation, error) {
	if !filepath.IsAbs(path) {
		return gitmanager.FilesystemObservation{}, fmt.Errorf("observe filesystem: %q is not absolute", path)
	}
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		return gitmanager.FilesystemObservation{}, fmt.Errorf("stat %s: %w", path, err)
	}
	facts, err := observeMount(path)
	if err != nil {
		return gitmanager.FilesystemObservation{}, err
	}
	return gitmanager.FilesystemObservation{
		GOOS:           runtime.GOOS,
		FilesystemType: facts.fsType,
		MountIdentity:  facts.identity,
		DeviceDecimal:  strconv.FormatUint(deviceNumber(&st), 10),
		InodeDecimal:   strconv.FormatUint(uint64(st.Ino), 10),
		MountFlags:     facts.flags,
		Local:          facts.local,
		ReadOnly:       facts.readOnly,
	}, nil
}

// ProbePrimitives proves the durability and exclusion primitives on the
// filesystem of a private directory (mode 0700, owned by this user, not a
// symlink) by exercising each one on files it creates there, then removes
// exactly those files. A primitive that misbehaves is reported false; an
// unexpected error aborts the probe.
func ProbePrimitives(dir string) (gitmanager.FilesystemPrimitives, error) {
	if err := requirePrivateDirectory(dir); err != nil {
		return gitmanager.FilesystemPrimitives{}, err
	}
	token := make([]byte, 8)
	if _, err := rand.Read(token); err != nil {
		return gitmanager.FilesystemPrimitives{}, err
	}
	name := func(suffix string) string { return filepath.Join(dir, ".probe-"+hex.EncodeToString(token)+"-"+suffix) }
	var created []string
	defer func() {
		for _, path := range created {
			_ = os.Remove(path)
		}
	}()
	var p gitmanager.FilesystemPrimitives

	first := name("a")
	file, err := os.OpenFile(first, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return p, fmt.Errorf("probe exclusive create: %w", err)
	}
	created = append(created, first)
	defer file.Close()
	_, err = os.OpenFile(first, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o600)
	p.ExclusiveCreate = errors.Is(err, os.ErrExist)
	if err := file.Chmod(0o600); err != nil {
		return p, err
	}
	if _, err := file.Write([]byte("probe\n")); err != nil {
		return p, err
	}
	p.FileSync = file.Sync() == nil

	opened, err := file.Stat()
	if err != nil {
		return p, err
	}
	named, err := os.Lstat(first)
	p.StableIdentity = err == nil && os.SameFile(opened, named) && named.Mode().Perm() == 0o600

	// An exclusive advisory lock on one open file description must make a
	// second, non-blocking attempt on another description fail.
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
		other, err := os.Open(first)
		if err != nil {
			return p, err
		}
		contended := syscall.Flock(int(other.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		other.Close()
		p.AdvisoryLock = errors.Is(contended, syscall.EWOULDBLOCK) || errors.Is(contended, syscall.EAGAIN)
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	}

	renamed := name("b")
	if err := os.Rename(first, renamed); err == nil {
		created = append(created, renamed)
		moved, err := os.Lstat(renamed)
		p.AtomicSameFilesystemRename = err == nil && os.SameFile(opened, moved)
	}
	target := name("c")
	if err := os.WriteFile(target, []byte("target\n"), 0o600); err != nil {
		return p, err
	}
	created = append(created, target)
	if err := os.Rename(renamed, target); err == nil {
		replaced, err := os.Lstat(target)
		p.AtomicReplace = err == nil && os.SameFile(opened, replaced)
	}
	// No-replace publication relies on link failing over an existing name.
	linked := name("d")
	if err := os.Link(target, linked); err == nil {
		created = append(created, linked)
		p.AtomicReplace = p.AtomicReplace && errors.Is(os.Link(target, linked), os.ErrExist)
	} else {
		p.AtomicReplace = false
	}
	directory, err := os.Open(dir)
	if err != nil {
		return p, err
	}
	p.DirectorySync = directory.Sync() == nil
	directory.Close()

	evidence := fmt.Sprintf("%s|%s|exclusive_create=%t|advisory_lock=%t|file_sync=%t|directory_sync=%t|rename=%t|replace=%t|stable_identity=%t",
		gitmanager.FilesystemProfileVersion, runtime.GOOS, p.ExclusiveCreate, p.AdvisoryLock, p.FileSync, p.DirectorySync,
		p.AtomicSameFilesystemRename, p.AtomicReplace, p.StableIdentity)
	p.EvidenceDigest = gitmanager.ContentDigest(fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(evidence))))
	return p, nil
}

// requirePrivateDirectory accepts only a real directory with mode 0700
// owned by the effective user.
func requirePrivateDirectory(dir string) error {
	refuse := func(detail string) error {
		return &PlatformError{Reason: gitmanager.ReasonAuditUnavailable, Detail: dir + " " + detail}
	}
	if !filepath.IsAbs(dir) || strings.ContainsRune(dir, 0) {
		return refuse("is not an absolute path")
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return refuse("cannot be inspected: " + err.Error())
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return refuse("is not a real directory")
	}
	if info.Mode().Perm() != 0o700 {
		return refuse(fmt.Sprintf("has mode %04o, not 0700", info.Mode().Perm()))
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(st.Uid) != os.Geteuid() {
		return refuse("is not owned by the current user")
	}
	return nil
}
