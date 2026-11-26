//go:build linux

package fswatch

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestWatchLimitIsUnavailable(t *testing.T) {
	saved := addWatch
	t.Cleanup(func() { addWatch = saved })
	addWatch = func(int, string, uint32) (int, error) { return -1, syscall.ENOSPC }
	root, feature, _, _ := project(t)
	w := newWatcher(t)
	err := w.Sync([]string{root, feature}, nil)
	var u Unavailable
	if !errors.As(err, &u) || u.Reason != "watch_limit" || !strings.Contains(u.Detail, "fs.inotify.max_user_watches") ||
		!strings.Contains(u.Remedy, "sudo sysctl fs.inotify.max_user_watches=") {
		t.Fatalf("Sync = %v; want watch_limit naming fs.inotify.max_user_watches", err)
	}
}

func TestInstanceLimitIsUnavailable(t *testing.T) {
	saved := inotifyInit
	t.Cleanup(func() { inotifyInit = saved })
	inotifyInit = func() (int, error) { return -1, syscall.EMFILE }
	_, err := New()
	var u Unavailable
	if !errors.As(err, &u) || u.Reason != "instance_limit" || !strings.Contains(u.Detail, "fs.inotify.max_user_instances") {
		t.Fatalf("New = %v; want instance_limit naming fs.inotify.max_user_instances", err)
	}
}

func TestParseOverflow(t *testing.T) {
	record := make([]byte, syscall.SizeofInotifyEvent)
	binary.NativeEndian.PutUint32(record[0:4], ^uint32(0)) // wd -1
	binary.NativeEndian.PutUint32(record[4:8], syscall.IN_Q_OVERFLOW)
	changes := parseEvents(record, func(int32) (string, bool) { return "", false })
	if len(changes) != 1 || changes[0].Kind != Overflow {
		t.Fatalf("parseEvents = %+v; want one Overflow", changes)
	}
}

func TestParseNamesTheWatchedDirectory(t *testing.T) {
	name := []byte("spec.md\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	record := make([]byte, syscall.SizeofInotifyEvent+len(name))
	binary.NativeEndian.PutUint32(record[0:4], 7)
	binary.NativeEndian.PutUint32(record[4:8], syscall.IN_CLOSE_WRITE)
	binary.NativeEndian.PutUint32(record[12:16], uint32(len(name)))
	copy(record[syscall.SizeofInotifyEvent:], name)
	gone := make([]byte, syscall.SizeofInotifyEvent)
	binary.NativeEndian.PutUint32(gone[0:4], 7)
	binary.NativeEndian.PutUint32(gone[4:8], syscall.IN_DELETE_SELF)
	changes := parseEvents(append(record, gone...), func(wd int32) (string, bool) { return "/p/specs/001-a", wd == 7 })
	if len(changes) != 2 || changes[0] != (Change{Path: "/p/specs/001-a", Kind: Changed}) || changes[1] != (Change{Path: "/p/specs/001-a", Kind: Gone}) {
		t.Fatalf("parseEvents = %+v", changes)
	}
}

func TestFilesAreIgnoredAndDirectoriesSuffice(t *testing.T) {
	root, feature, record, spec := project(t)
	w := newWatcher(t)
	if err := w.Sync([]string{root, feature}, []string{record, spec}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor(t, w, Changed, feature)
	drain(w, 300*time.Millisecond)
	if err := os.Mkdir(filepath.Join(root, "specs", "002-b"), 0o755); err != nil {
		t.Fatal(err)
	}
	quietFor(t, w, time.Second) // specs itself is not watched here
}
