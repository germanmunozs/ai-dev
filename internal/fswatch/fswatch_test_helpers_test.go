//go:build darwin || linux

package fswatch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// waitFor waits up to 5 s for a change to one of paths with kind.
func waitFor(t *testing.T, w Watcher, kind Kind, paths ...string) Change {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case c, ok := <-w.Events():
			if !ok {
				t.Fatalf("events closed while waiting for %v %v", kind, paths)
			}
			for _, p := range paths {
				if c.Path == p && c.Kind == kind {
					return c
				}
			}
		case <-deadline:
			t.Fatalf("no change of kind %v for %v within 5s", kind, paths)
		}
	}
}

// drain discards pending notifications for d.
func drain(w Watcher, d time.Duration) {
	deadline := time.After(d)
	for {
		select {
		case <-w.Events():
		case <-deadline:
			return
		}
	}
}

// quietFor asserts that no change arrives for d.
func quietFor(t *testing.T, w Watcher, d time.Duration) {
	t.Helper()
	select {
	case c := <-w.Events():
		t.Fatalf("unexpected change %+v", c)
	case <-time.After(d):
	}
}

// project creates a project with one feature and returns its paths.
func project(t *testing.T) (root, feature, record, spec string) {
	t.Helper()
	root = t.TempDir()
	feature = filepath.Join(root, "specs", "001-a")
	if err := os.MkdirAll(feature, 0o755); err != nil {
		t.Fatal(err)
	}
	record = filepath.Join(feature, "workflow-state.json")
	spec = filepath.Join(feature, "spec.md")
	for _, f := range []string{record, spec} {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, feature, record, spec
}

func replaceAtomically(t *testing.T, path, content string) {
	t.Helper()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
}

func newWatcher(t *testing.T) Watcher {
	t.Helper()
	w, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}

// The shared cases, on the real file system (R-01).

func TestAtomicReplaceIsSeen(t *testing.T) {
	root, feature, record, spec := project(t)
	w := newWatcher(t)
	if _, err := w.Sync([]string{root, feature}, []string{record, spec}); err != nil {
		t.Fatal(err)
	}
	replaceAtomically(t, record, `{"state":"approved"}`)
	waitFor(t, w, Changed, feature, record)
}

func TestInPlaceWriteIsSeen(t *testing.T) {
	root, feature, record, spec := project(t)
	w := newWatcher(t)
	if _, err := w.Sync([]string{root, feature}, []string{record, spec}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor(t, w, Changed, feature, spec)
}

func TestSymbolicLinkDirectoryIsNotFollowed(t *testing.T) {
	root, feature, _, _ := project(t)
	outside := t.TempDir()
	link := filepath.Join(root, "specs", "002-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	w := newWatcher(t)
	if _, err := w.Sync([]string{root, feature, link}, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "x"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	quietFor(t, w, time.Second)
}

func TestCloseEndsEvents(t *testing.T) {
	root, feature, record, spec := project(t)
	w, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Sync([]string{root, feature}, []string{record, spec}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case _, ok := <-w.Events():
		for ok {
			_, ok = <-w.Events()
		}
	case <-time.After(5 * time.Second):
		t.Fatal("events not closed after Close")
	}
}

func TestFeatureDirectoryCreatedIsSeen(t *testing.T) {
	root, feature, record, spec := project(t)
	specs := filepath.Join(root, "specs")
	w := newWatcher(t)
	if _, err := w.Sync([]string{root, specs, feature}, []string{record, spec}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(specs, "002-new"), 0o755); err != nil {
		t.Fatal(err)
	}
	waitFor(t, w, Changed, specs, filepath.Join(specs, "002-new"))
}

func TestFeatureDirectoryRemovedIsSeen(t *testing.T) {
	root, feature, record, spec := project(t)
	specs := filepath.Join(root, "specs")
	w := newWatcher(t)
	if _, err := w.Sync([]string{root, specs, feature}, []string{record, spec}); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(feature); err != nil {
		t.Fatal(err)
	}
	seenSpecs, seenGone := false, false
	deadline := time.After(5 * time.Second)
	for !seenSpecs || !seenGone {
		select {
		case c := <-w.Events():
			seenSpecs = seenSpecs || (c.Path == specs || c.Path == feature) && c.Kind == Changed
			seenGone = seenGone || c.Path == feature && c.Kind == Gone
		case <-deadline:
			t.Fatalf("within 5s: specs changed %v, feature gone %v", seenSpecs, seenGone)
		}
	}
}

func TestSpecsCreatedLaterIsSeenOnTheProject(t *testing.T) {
	root := t.TempDir()
	w := newWatcher(t)
	if _, err := w.Sync([]string{root, filepath.Join(root, "specs")}, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	waitFor(t, w, Changed, root, filepath.Join(root, "specs"))
}

// A path that cannot be read is skipped, not fatal: the parent directory
// still reports it, and the commands report the file as invalid (review F1).
func TestUnreadablePathsAreSkipped(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permissions do not apply to root")
	}
	root, feature, record, spec := project(t)
	locked := filepath.Join(root, "specs", "002-locked")
	if err := os.Mkdir(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{spec, locked} {
		if err := os.Chmod(p, 0); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _ = os.Chmod(spec, 0o644); _ = os.Chmod(locked, 0o755) })
	w := newWatcher(t)
	if _, err := w.Sync([]string{root, feature, locked}, []string{record, spec}); err != nil {
		t.Fatalf("Sync with unreadable paths: %v", err)
	}
	replaceAtomically(t, record, `{"state":"approved"}`)
	waitFor(t, w, Changed, feature, record)
}
