//go:build darwin

package fswatch

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// After an atomic replace the old descriptor names a deleted file; Sync
// reopens the path, so the new file's own writes are seen.
func TestSyncReopensReplacedFile(t *testing.T) {
	root, feature, record, spec := project(t)
	w := newWatcher(t)
	if _, err := w.Sync([]string{root, feature}, []string{record, spec}); err != nil {
		t.Fatal(err)
	}
	replaceAtomically(t, record, `{"state":"approved"}`)
	waitFor(t, w, Changed, feature, record)
	drain(w, 300*time.Millisecond)
	if _, err := w.Sync([]string{root, feature}, []string{record, spec}); err != nil {
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
	if _, err := w.Sync([]string{root}, []string{link}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	quietFor(t, w, time.Second)
}

func TestCloseReleasesDescriptors(t *testing.T) {
	root, feature, record, spec := project(t)
	watcher, err := New()
	if err != nil {
		t.Fatal(err)
	}
	w := watcher.(*kqueueWatcher)
	if added, err := w.Sync([]string{root, feature}, []string{record, spec}); err != nil || len(added) != 4 {
		t.Fatalf("Sync added %v, %v; want 4 paths", added, err)
	}
	// The exact descriptors, rather than a count of /dev/fd, which other
	// goroutines of the test process change (seen on a CI runner).
	fds := []int{w.kq}
	w.mu.Lock()
	for _, v := range w.paths {
		fds = append(fds, v.fd)
	}
	w.mu.Unlock()
	if len(fds) != 5 {
		t.Fatalf("descriptors %v, want the kqueue and 4 paths", fds)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	for range w.Events() {
	}
	for _, fd := range fds {
		var st syscall.Stat_t
		if err := syscall.Fstat(fd, &st); err != syscall.EBADF {
			t.Errorf("descriptor %d still open after Close (Fstat: %v)", fd, err)
		}
	}
}

func TestSyncDropsPaths(t *testing.T) {
	root, feature, record, spec := project(t)
	w := newWatcher(t)
	if _, err := w.Sync([]string{root, feature}, []string{record, spec}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Sync([]string{root}, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	quietFor(t, w, time.Second)
}

// A FIFO in place of a file is skipped, never opened in a way that blocks
// (review F9).
func TestFIFOIsSkippedWithoutBlocking(t *testing.T) {
	root, feature, _, spec := project(t)
	if err := os.Remove(spec); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(spec, 0o644); err != nil {
		t.Fatal(err)
	}
	w := newWatcher(t)
	done := make(chan error, 1)
	go func() {
		_, err := w.Sync([]string{root, feature}, []string{spec})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Sync blocked on a FIFO")
	}
	if added, _ := w.Sync([]string{root, feature}, []string{spec}); len(added) != 0 {
		t.Fatalf("second Sync added %v", added)
	}
}

// The FIFO case also holds when the swap happens between Sync's Lstat and
// its open: add itself refuses anything but a file or directory.
func TestAddSkipsSpecialFiles(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "fifo")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Fatal(err)
	}
	w := newWatcher(t).(*kqueueWatcher)
	w.mu.Lock()
	ok, err := w.add(fifo)
	w.mu.Unlock()
	if ok || err != nil {
		t.Fatalf("add(fifo) = %v, %v; want skipped", ok, err)
	}
}

// When the reader fails for any reason but Close, Events is closed so the
// loop knows watching was lost (review F8).
func TestReaderFailureClosesEvents(t *testing.T) {
	w := newWatcher(t).(*kqueueWatcher)
	syscall.Close(w.kq) // Kevent now fails with EBADF
	select {
	case _, ok := <-w.Events():
		if ok {
			t.Fatal("unexpected change")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Events not closed after the reader failed")
	}
}

// Sync reports the paths it started observing, including a reopened one.
func TestSyncReportsAddedPaths(t *testing.T) {
	root, feature, record, spec := project(t)
	w := newWatcher(t)
	added, err := w.Sync([]string{root, feature}, []string{record, spec})
	if err != nil || len(added) != 4 {
		t.Fatalf("first Sync added %v, %v; want 4 paths", added, err)
	}
	if added, _ := w.Sync([]string{root, feature}, []string{record, spec}); len(added) != 0 {
		t.Fatalf("unchanged Sync added %v", added)
	}
	replaceAtomically(t, record, "{}")
	if added, _ := w.Sync([]string{root, feature}, []string{record, spec}); len(added) != 1 || added[0] != record {
		t.Fatalf("Sync after a replace added %v, want [%s]", added, record)
	}
}
