//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"context"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type rawEntry struct {
	path  string
	mode  uint32
	oid   BlobOID
	flags uint16 // added to the name length
}

// encodeIndex writes a SHA-1 index as Git does: header, 8-byte aligned
// entries, extensions and the trailing checksum.
func encodeIndex(version uint32, entries []rawEntry, extensions ...[]byte) []byte {
	data := []byte("DIRC")
	data = binary.BigEndian.AppendUint32(data, version)
	data = binary.BigEndian.AppendUint32(data, uint32(len(entries)))
	for _, e := range entries {
		entry := make([]byte, 40)
		binary.BigEndian.PutUint32(entry[24:], e.mode)
		raw, _ := hex.DecodeString(string(e.oid))
		entry = append(entry, raw...)
		entry = binary.BigEndian.AppendUint16(entry, uint16(len(e.path))|e.flags)
		entry = append(entry, e.path...)
		for pad := (len(entry) + 8) &^ 7; len(entry) < pad; {
			entry = append(entry, 0)
		}
		data = append(data, entry...)
	}
	for _, extension := range extensions {
		data = append(data, extension...)
	}
	sum := sha1.Sum(data)
	return append(data, sum[:]...)
}

func extension(signature string, body []byte) []byte {
	return append(binary.BigEndian.AppendUint32([]byte(signature), uint32(len(body))), body...)
}

// T082/AC-025: only an autonomous version-2 index with a valid checksum,
// sorted stage-0 entries and at most a cache tree is accepted
// (F008-FR-157–158).
func TestAutonomousIndexParser(t *testing.T) {
	a, b := blobOID(ObjectFormatSHA1, []byte("a")), blobOID(ObjectFormatSHA1, []byte("b"))
	entries := []rawEntry{{path: "a.txt", mode: 0o100644, oid: a}, {path: "dir/b.sh", mode: 0o100755, oid: b}}
	valid := encodeIndex(2, entries, extension("TREE", []byte("cached")))
	parsed, err := parseAutonomousIndex(valid, ObjectFormatSHA1)
	if err != nil || len(parsed) != 2 || parsed[1].path != "dir/b.sh" || parsed[1].mode != 0o100755 || parsed[1].oid != ObjectOID(b) {
		t.Fatalf("parsed %+v %v", parsed, err)
	}
	files := []AuthorizedFile{{PathUTF8: "a.txt", Mode: "100644", BlobOID: a}, {PathUTF8: "dir/b.sh", Mode: "100755", BlobOID: b}}
	trees, _ := commitTrees(ObjectFormatSHA1, files)
	if !indexHoldsTree(valid, ObjectFormatSHA1, trees[len(trees)-1].oid) || indexHoldsTree(valid, ObjectFormatSHA1, trees[0].oid) {
		t.Fatal("the index is not bound to exactly its tree")
	}
	corrupt := append([]byte(nil), valid...)
	corrupt[len(corrupt)-1] ^= 1
	for name, data := range map[string][]byte{
		"bad checksum":      corrupt,
		"version 3":         encodeIndex(3, entries),
		"extended flag":     encodeIndex(2, []rawEntry{{path: "a.txt", mode: 0o100644, oid: a, flags: 0x4000}}),
		"conflict stage":    encodeIndex(2, []rawEntry{{path: "a.txt", mode: 0o100644, oid: a, flags: 0x1000}}),
		"split index link":  encodeIndex(2, entries, extension("link", make([]byte, 20))),
		"sparse directory":  encodeIndex(2, entries, extension("sdir", nil)),
		"untracked cache":   encodeIndex(2, entries, extension("UNTR", nil)),
		"fsmonitor":         encodeIndex(2, entries, extension("FSMN", nil)),
		"unsorted":          encodeIndex(2, []rawEntry{entries[1], entries[0]}),
		"duplicate":         encodeIndex(2, []rawEntry{entries[0], entries[0]}),
		"symlink entry":     encodeIndex(2, []rawEntry{{path: "a.txt", mode: 0o120000, oid: a}}),
		"gitlink entry":     encodeIndex(2, []rawEntry{{path: "a.txt", mode: 0o160000, oid: a}}),
		"truncated":         valid[:40],
		"wrong signature":   append([]byte("DIRX"), valid[4:]...),
		"extension overrun": encodeIndex(2, entries, binary.BigEndian.AppendUint32([]byte("TREE"), 1<<20)),
	} {
		if _, err := parseAutonomousIndex(data, ObjectFormatSHA1); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// T082/AC-017/AC-025: the descriptor-owned index.lock steps. A
// pre-existing lock is never taken or removed; a lock replaced after its
// creation is neither installed nor removed; only the still-owned inode
// is unlinked on abandonment (F008-FR-117, 119, 184).
func TestIndexLockProtocol(t *testing.T) {
	gitDir := physicalTempDir(t)
	lockPath, indexPath := filepath.Join(gitDir, "index.lock"), filepath.Join(gitDir, "index")

	// Step 1: a pre-existing lock is a conflict and stays.
	mustWrite(t, lockPath, "foreign")
	if _, err := acquireIndexLock(gitDir); err == nil {
		t.Fatal("a pre-existing lock was acquired")
	}
	if data, _ := os.ReadFile(lockPath); string(data) != "foreign" {
		t.Fatal("a pre-existing lock was touched")
	}
	os.Remove(lockPath)

	// Steps 2–4 and 13: an abandoned owned lock is unlinked.
	lock, err := acquireIndexLock(gitDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.release(context.Background()); err != nil {
		t.Fatalf("release of an owned lock: %v", err)
	}
	if _, err := os.Lstat(lockPath); !os.IsNotExist(err) {
		t.Fatal("the owned lock remains")
	}

	// A lock replaced after creation is not installed and not removed.
	lock, err = acquireIndexLock(gitDir)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, lockPath+".other", "replacement")
	if err := os.Rename(lockPath+".other", lockPath); err != nil {
		t.Fatal(err)
	}
	if err := lock.install(gitDir, []byte("prepared")); !errors.Is(err, ErrOwnedResourceReplaced) {
		t.Fatalf("install over a replaced lock: %v", err)
	}
	if _, err := os.Lstat(indexPath); !os.IsNotExist(err) {
		t.Fatal("an index was installed from a replaced lock")
	}
	if err := lock.release(context.Background()); !errors.Is(err, ErrOwnedResourceReplaced) {
		t.Fatalf("release of a replaced lock: %v", err)
	}
	if data, _ := os.ReadFile(lockPath); string(data) != "replacement" {
		t.Fatal("a replaced lock was removed")
	}
	os.Remove(lockPath)

	// Steps 7–11: the bytes go through the retained descriptor and the
	// owned lock is renamed to index; release afterwards does nothing.
	lock, err = acquireIndexLock(gitDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.install(gitDir, []byte("prepared")); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(indexPath); string(data) != "prepared" {
		t.Fatalf("installed index %q", data)
	}
	if err := lock.release(context.Background()); err != nil {
		t.Fatalf("release after installation: %v", err)
	}
	if _, err := os.Lstat(indexPath); err != nil {
		t.Fatal("release removed the installed index")
	}
}

// T082/AC-017: prepared-index storage is private and its release removes
// only what this attempt created.
func TestPreparedIndexStorage(t *testing.T) {
	root := physicalTempDir(t)
	mustMkdir(t, filepath.Join(root, "prepared"))
	prepared, err := newPreparedIndex([]byte(root), newFakeRandomSource(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(prepared.dir); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("prepared directory %v %v", info, err)
	}
	mustWrite(t, prepared.path, "index")
	if _, err := prepared.read(); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, prepared.path+".tmp", "replacement")
	os.Rename(prepared.path+".tmp", prepared.path)
	if err := prepared.release(context.Background()); !errors.Is(err, ErrOwnedResourceReplaced) {
		t.Fatalf("release of a replaced prepared index: %v", err)
	}
	if data, _ := os.ReadFile(prepared.path); string(data) != "replacement" {
		t.Fatal("a replaced prepared index was removed")
	}
}
