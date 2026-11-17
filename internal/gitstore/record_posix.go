//go:build (darwin && !ios) || (linux && !android)

package gitstore

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"syscall"

	"ai-dev/internal/gitmanager"
)

// recordOps are the filesystem steps of record publication and
// replacement. Tests substitute single steps to inject faults.
type recordOps struct {
	lock          func(*os.File) error
	read          func(*os.Root, string) ([]byte, error)
	openTemporary func(*os.Root, string) (*os.File, error)
	write         func(*os.File, []byte) error
	syncFile      func(*os.File) error
	closeFile     func(*os.File) error
	link          func(*os.Root, string, string) error
	rename        func(*os.Root, string, string) error
	remove        func(*os.Root, string) error
	syncDirectory func(*os.File) error
	step          func(string)
}

func defaultRecordOps() recordOps {
	return recordOps{
		// Blocking: the critical section is one read, write and sync.
		lock: func(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_EX) },
		read: readBounded,
		openTemporary: func(root *os.Root, name string) (*os.File, error) {
			return root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o600)
		},
		write:         func(f *os.File, data []byte) error { _, err := f.Write(data); return err },
		syncFile:      func(f *os.File) error { return f.Sync() }, // F_FULLFSYNC on Darwin
		closeFile:     func(f *os.File) error { return f.Close() },
		link:          func(root *os.Root, oldname, newname string) error { return root.Link(oldname, newname) },
		rename:        func(root *os.Root, oldname, newname string) error { return root.Rename(oldname, newname) },
		remove:        func(root *os.Root, name string) error { return root.Remove(name) },
		syncDirectory: func(f *os.File) error { return f.Sync() },
		step:          func(string) {},
	}
}

func readBounded(root *os.Root, name string) ([]byte, error) {
	f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxRecordBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxRecordBytes {
		return nil, fmt.Errorf("record %s exceeds %d bytes", name, maxRecordBytes)
	}
	return data, nil
}

// records publishes and replaces private records beneath one root.
type records struct {
	root *os.Root
	ops  recordOps
}

// writeTemporary writes data to a fresh same-directory temporary, syncs and
// closes it. On failure the temporary is removed: it is owned and the
// process is alive, so it is not crash evidence.
func (r records) writeTemporary(dir string, data []byte) (string, error) {
	token := make([]byte, 8)
	if _, err := rand.Read(token); err != nil {
		return "", err
	}
	temporary := path.Join(dir, temporaryPrefix+hex.EncodeToString(token))
	r.ops.step("open-temporary")
	f, err := r.ops.openTemporary(r.root, temporary)
	if err != nil {
		return "", err
	}
	fail := func(err error) (string, error) {
		_ = r.root.Remove(temporary)
		return "", err
	}
	// The mode is fixed explicitly: the umask must not widen nor narrow it.
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return fail(err)
	}
	r.ops.step("write")
	if err := r.ops.write(f, data); err != nil {
		f.Close()
		return fail(err)
	}
	r.ops.step("sync-file")
	if err := r.ops.syncFile(f); err != nil {
		f.Close()
		return fail(err)
	}
	r.ops.step("close")
	if err := r.ops.closeFile(f); err != nil {
		return fail(err)
	}
	return temporary, nil
}

// syncDirectory makes a published name durable. Failure leaves the record
// in place with unknown durability.
func (r records) syncDirectory(dir string) error {
	d, err := r.root.Open(dir)
	if err == nil {
		r.ops.step("sync-directory")
		err = r.ops.syncDirectory(d)
		d.Close()
	}
	if err != nil {
		return fmt.Errorf("%w: directory sync after publication: %v", gitmanager.ErrStoreOutcomeUnverifiable, err)
	}
	return nil
}

// publish writes an immutable record without ever replacing an existing
// one: the complete temporary is synced, hard-linked to the absent final
// name, verified to be the same file, unlinked, and the directory synced.
func (r records) publish(dir, name string, data []byte) (gitmanager.ContentDigest, error) {
	if !validRecordName(name) {
		return "", fmt.Errorf("invalid record name %q", name)
	}
	final := path.Join(dir, name)
	temporary, err := r.writeTemporary(dir, data)
	if err != nil {
		return "", err
	}
	r.ops.step("link")
	if err := r.ops.link(r.root, temporary, final); err != nil {
		_ = r.root.Remove(temporary)
		if errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("%w: %s", ErrRecordExists, final)
		}
		return "", err
	}
	r.ops.step("verify")
	published, err := r.root.Lstat(final)
	if err != nil {
		return "", fmt.Errorf("%w: published record vanished: %v", gitmanager.ErrStoreOutcomeUnverifiable, err)
	}
	if owned, err := r.root.Lstat(temporary); err != nil || !os.SameFile(owned, published) {
		// Someone replaced a name we own; leave both for reconciliation.
		return "", fmt.Errorf("%w: published record is not the written temporary", gitmanager.ErrStoreOutcomeUnverifiable)
	}
	r.ops.step("remove-temporary")
	// A leftover temporary after a successful link is a harmless non-event.
	_ = r.ops.remove(r.root, temporary)
	if err := r.syncDirectory(dir); err != nil {
		return "", err
	}
	return gitmanager.ContentDigest(fmt.Sprintf("sha256:%x", sha256.Sum256(data))), nil
}

// read returns a private record: a regular, non-symlink file with mode 0600.
func (r records) read(dir, name string) ([]byte, error) {
	if !validRecordName(name) {
		return nil, fmt.Errorf("invalid record name %q", name)
	}
	target := path.Join(dir, name)
	info, err := r.root.Lstat(target)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return nil, fmt.Errorf("record %s is not a private regular file (%v)", target, info.Mode())
	}
	r.ops.step("read")
	return r.ops.read(r.root, target)
}

// replace performs one compare-and-swap of an existing record: under an
// exclusive lock on its directory, check sees the current bytes, and only
// when it accepts them is the record atomically replaced by next.
func (r records) replace(dir, name string, check func(current []byte) error, next []byte) error {
	d, err := r.root.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	r.ops.step("lock")
	if err := r.ops.lock(d); err != nil {
		return err
	}
	defer syscall.Flock(int(d.Fd()), syscall.LOCK_UN)
	current, err := r.read(dir, name)
	if err != nil {
		return err
	}
	if err := check(current); err != nil {
		return err
	}
	temporary, err := r.writeTemporary(dir, next)
	if err != nil {
		return err
	}
	r.ops.step("rename")
	if err := r.ops.rename(r.root, temporary, path.Join(dir, name)); err != nil {
		_ = r.root.Remove(temporary)
		return err
	}
	return r.syncDirectory(dir)
}

// list returns the record names and, separately, the temporaries in a
// directory, both sorted. Temporaries are reported, never removed.
func (r records) list(dir string) (names, temporaries []string, err error) {
	d, err := r.root.Open(dir)
	if err != nil {
		return nil, nil, err
	}
	defer d.Close()
	all, err := d.Readdirnames(-1)
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(all)
	for _, entry := range all {
		if strings.HasPrefix(entry, temporaryPrefix) {
			temporaries = append(temporaries, entry)
		} else {
			names = append(names, entry)
		}
	}
	return names, temporaries, nil
}

// publishEvent appends one immutable audit event.
func (r records) publishEvent(dir string, sequence uint64, kind gitmanager.AuditEventKind, data []byte) (gitmanager.ContentDigest, error) {
	if sequence == 0 || !kind.Valid() || len(data) == 0 {
		return "", fmt.Errorf("invalid audit event %d %q", sequence, kind)
	}
	return r.publish(dir, eventName(sequence, kind), data)
}

// listEvents returns the events of a directory in sequence order, which
// must run 1..n without gaps or duplicates over private regular files.
// Anything else rejects the whole directory with ErrInconsistentEvents.
func (r records) listEvents(dir string) ([]event, []string, error) {
	names, temporaries, err := r.list(dir)
	if err != nil {
		return nil, nil, err
	}
	events := make([]event, 0, len(names))
	for _, name := range names {
		sequence, kind, err := parseEventName(name)
		if err != nil {
			return nil, temporaries, err
		}
		info, err := r.root.Lstat(path.Join(dir, name))
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			return nil, temporaries, fmt.Errorf("%w: %s is not a private regular file", ErrInconsistentEvents, name)
		}
		events = append(events, event{sequence: sequence, kind: kind, name: name})
	}
	sort.Slice(events, func(i, j int) bool { return events[i].sequence < events[j].sequence })
	for i, e := range events {
		if e.sequence != uint64(i+1) {
			return nil, temporaries, fmt.Errorf("%w: expected sequence %d, found %s", ErrInconsistentEvents, i+1, e.name)
		}
	}
	return events, temporaries, nil
}

// ensurePrivateDirectory creates one directory level with mode 0700, or
// accepts an existing one only if it is a real directory, mode 0700, owned
// by the effective user.
func ensurePrivateDirectory(root *os.Root, name string) error {
	created := false
	if err := root.Mkdir(name, 0o700); err == nil {
		created = true
	} else if !errors.Is(err, os.ErrExist) {
		return err
	}
	if created {
		if err := root.Chmod(name, 0o700); err != nil {
			return err
		}
	}
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o700 || !ok || int(st.Uid) != os.Geteuid() {
		return &PlatformError{Reason: gitmanager.ReasonAuditUnavailable, Detail: name + " is not a private directory owned by this user"}
	}
	return nil
}
