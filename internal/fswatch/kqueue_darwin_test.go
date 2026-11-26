//go:build darwin

package fswatch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func openDescriptors(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/dev/fd")
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}

// After an atomic replace the old descriptor names a deleted file; Sync
// reopens the path, so the new file's own writes are seen.
func TestSyncReopensReplacedFile(t *testing.T) {
	root, feature, record, spec := project(t)
	w := newWatcher(t)
	if err := w.Sync([]string{root, feature}, []string{record, spec}); err != nil {
		t.Fatal(err)
	}
	replaceAtomically(t, record, `{"state":"approved"}`)
	waitFor(t, w, Changed, feature, record)
	drain(w, 300*time.Millisecond)
	if err := w.Sync([]string{root, feature}, []string{record, spec}); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(record, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(" "); err != nil {
		t.Fatal(err)
	}
	file.Close()
	waitFor(t, w, Changed, record)
}

// A symbolic link given as a file is opened as the link itself: writes to
// its target are not seen.
func TestSymbolicLinkFileIsNotFollowed(t *testing.T) {
	root, feature, _, _ := project(t)
	target := filepath.Join(t.TempDir(), "target.md")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(feature, "plan.md")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	w := newWatcher(t)
	if err := w.Sync([]string{root}, []string{link}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	quietFor(t, w, time.Second)
}

func TestCloseReleasesDescriptors(t *testing.T) {
	root, feature, record, spec := project(t)
	before := openDescriptors(t)
	w, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Sync([]string{root, feature}, []string{record, spec}); err != nil {
		t.Fatal(err)
	}
	if during := openDescriptors(t); during < before+5 {
		t.Fatalf("descriptors during watch %d, want at least %d", during, before+5)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	for range w.Events() {
	}
	if after := openDescriptors(t); after != before {
		t.Fatalf("descriptors after Close %d, want %d", after, before)
	}
}

func TestSyncDropsPaths(t *testing.T) {
	root, feature, record, spec := project(t)
	w := newWatcher(t)
	if err := w.Sync([]string{root, feature}, []string{record, spec}); err != nil {
		t.Fatal(err)
	}
	if err := w.Sync([]string{root}, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	quietFor(t, w, time.Second)
}
