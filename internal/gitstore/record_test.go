//go:build (darwin && !ios) || (linux && !android)

package gitstore

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"

	"ai-dev/internal/gitmanager"
)

// newTestRecords opens a private root with one private record directory
// and returns a step trace of every record operation.
func newTestRecords(t *testing.T) (records, string, *[]string) {
	t.Helper()
	base := privateTempDir(t)
	root, err := os.OpenRoot(base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	if err := ensurePrivateDirectory(root, "records"); err != nil {
		t.Fatal(err)
	}
	var trace []string
	ops := defaultRecordOps()
	ops.step = func(name string) { trace = append(trace, name) }
	return records{root: root, ops: ops}, filepath.Join(base, "records"), &trace
}

func entries(t *testing.T, dir string) []string {
	t.Helper()
	list, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range list {
		names = append(names, entry.Name())
	}
	return names
}

func TestPublishCreatesPrivateImmutableRecord(t *testing.T) {
	r, dir, trace := newTestRecords(t)
	data := []byte(`{"event":1}`)
	digest, err := r.publish("records", "a.json", data)
	if err != nil {
		t.Fatal(err)
	}
	if want := gitmanager.ContentDigest(fmt.Sprintf("sha256:%x", sha256.Sum256(data))); digest != want {
		t.Fatalf("digest %q want %q", digest, want)
	}
	info, err := os.Lstat(filepath.Join(dir, "a.json"))
	if err != nil || info.Mode().Perm() != 0o600 || !info.Mode().IsRegular() {
		t.Fatalf("record mode %v, %v", info.Mode(), err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "a.json")); !bytes.Equal(got, data) {
		t.Fatalf("record bytes %q", got)
	}
	// Durability order: the complete temporary is synced before it is
	// published, and the directory is synced after.
	want := []string{"open-temporary", "write", "sync-file", "close", "link", "verify", "remove-temporary", "sync-directory"}
	if !reflect.DeepEqual(*trace, want) {
		t.Fatalf("steps %v, want %v", *trace, want)
	}
	if names := entries(t, dir); len(names) != 1 {
		t.Fatalf("directory holds %v", names)
	}

	// Publication never replaces: the original survives a second publish.
	if _, err := r.publish("records", "a.json", []byte("other")); !errors.Is(err, ErrRecordExists) {
		t.Fatalf("second publish: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "a.json")); !bytes.Equal(got, data) {
		t.Fatal("published record was replaced")
	}
	if names := entries(t, dir); len(names) != 1 {
		t.Fatalf("refused publication left %v", names)
	}
}

func TestPublishFaultsBeforePublicationLeaveNoRecord(t *testing.T) {
	for _, step := range []string{"open-temporary", "write", "sync-file", "close", "link"} {
		t.Run(step, func(t *testing.T) {
			r, dir, _ := newTestRecords(t)
			r.ops = failAt(r.ops, step)
			if _, err := r.publish("records", "a.json", []byte("data")); !errors.Is(err, errInjectedFault) {
				t.Fatalf("publish: %v", err)
			}
			if names := entries(t, dir); len(names) != 0 {
				t.Fatalf("failed publication left %v", names)
			}
		})
	}
}

// After the link the record exists; if the directory cannot be synced its
// durability is unknown, and the caller must not treat it as conclusive.
func TestPublishUnverifiableAfterLink(t *testing.T) {
	r, dir, _ := newTestRecords(t)
	r.ops = failAt(r.ops, "sync-directory")
	if _, err := r.publish("records", "a.json", []byte("data")); !errors.Is(err, gitmanager.ErrStoreOutcomeUnverifiable) {
		t.Fatalf("publish: %v", err)
	}
	if names := entries(t, dir); !reflect.DeepEqual(names, []string{"a.json"}) {
		t.Fatalf("directory holds %v", names)
	}
}

func TestReplaceComparesAndSwapsUnderLock(t *testing.T) {
	r, dir, trace := newTestRecords(t)
	if _, err := r.publish("records", "auth.json", []byte("v1")); err != nil {
		t.Fatal(err)
	}
	*trace = nil
	swap := func(want, next string) func([]byte) ([]byte, error) {
		return func(current []byte) ([]byte, error) {
			if string(current) != want {
				return nil, gitmanager.ErrAuthorizationConflict
			}
			return []byte(next), nil
		}
	}
	if err := r.replace("records", "auth.json", swap("v1", "v2")); err != nil {
		t.Fatal(err)
	}
	want := []string{"lock", "read", "open-temporary", "write", "sync-file", "close", "rename", "sync-directory"}
	if !reflect.DeepEqual(*trace, want) {
		t.Fatalf("steps %v, want %v", *trace, want)
	}
	info, _ := os.Lstat(filepath.Join(dir, "auth.json"))
	if got, _ := os.ReadFile(filepath.Join(dir, "auth.json")); string(got) != "v2" || info.Mode().Perm() != 0o600 {
		t.Fatalf("record %q mode %v", got, info.Mode())
	}
	if err := r.replace("records", "auth.json", swap("v1", "v3")); !errors.Is(err, gitmanager.ErrAuthorizationConflict) {
		t.Fatalf("stale compare: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "auth.json")); string(got) != "v2" {
		t.Fatalf("failed compare changed the record: %q", got)
	}
	if names := entries(t, dir); len(names) != 1 {
		t.Fatalf("directory holds %v", names)
	}
}

func TestReplaceFaults(t *testing.T) {
	accept := func([]byte) ([]byte, error) { return []byte("v2"), nil }
	for _, step := range []string{"lock", "read", "open-temporary", "write", "sync-file", "close", "rename"} {
		t.Run(step, func(t *testing.T) {
			r, dir, _ := newTestRecords(t)
			if _, err := r.publish("records", "auth.json", []byte("v1")); err != nil {
				t.Fatal(err)
			}
			r.ops = failAt(r.ops, step)
			if err := r.replace("records", "auth.json", accept); !errors.Is(err, errInjectedFault) {
				t.Fatalf("replace: %v", err)
			}
			if got, _ := os.ReadFile(filepath.Join(dir, "auth.json")); string(got) != "v1" {
				t.Fatalf("record changed to %q", got)
			}
			if names := entries(t, dir); len(names) != 1 {
				t.Fatalf("directory holds %v", names)
			}
		})
	}
	r, dir, _ := newTestRecords(t)
	if _, err := r.publish("records", "auth.json", []byte("v1")); err != nil {
		t.Fatal(err)
	}
	r.ops = failAt(r.ops, "sync-directory")
	if err := r.replace("records", "auth.json", accept); !errors.Is(err, gitmanager.ErrStoreOutcomeUnverifiable) {
		t.Fatalf("directory sync failure: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "auth.json")); string(got) != "v2" {
		t.Fatalf("record %q", got)
	}
}

// Concurrent compare-and-swaps against the same expected value: exactly
// one wins, because the comparison and the swap happen under one lock.
func TestReplaceSerializesConcurrentWriters(t *testing.T) {
	r, dir, _ := newTestRecords(t)
	r.ops.step = func(string) {}
	if _, err := r.publish("records", "auth.json", []byte("0")); err != nil {
		t.Fatal(err)
	}
	const writers = 16
	var wins, conflicts int
	var mu sync.Mutex
	var wait sync.WaitGroup
	for i := 1; i <= writers; i++ {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			err := r.replace("records", "auth.json", func(current []byte) ([]byte, error) {
				if string(current) != "0" {
					return nil, gitmanager.ErrAuthorizationConflict
				}
				return []byte(strconv.Itoa(i)), nil
			})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				wins++
			case errors.Is(err, gitmanager.ErrAuthorizationConflict):
				conflicts++
			default:
				t.Errorf("writer %d: %v", i, err)
			}
		}(i)
	}
	wait.Wait()
	if wins != 1 || conflicts != writers-1 {
		t.Fatalf("wins=%d conflicts=%d", wins, conflicts)
	}
	if names := entries(t, dir); len(names) != 1 {
		t.Fatalf("directory holds %v", names)
	}
}

func TestReadRefusesUnsafeRecords(t *testing.T) {
	r, dir, _ := newTestRecords(t)
	if err := os.WriteFile(filepath.Join(dir, "open.json"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, "open.json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "open.json"), filepath.Join(dir, "link.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "dir.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"open.json", "link.json", "dir.json", "missing.json"} {
		if _, err := r.read("records", name); err == nil {
			t.Errorf("%s read", name)
		}
		if err := r.replace("records", name, func([]byte) ([]byte, error) { return []byte("y"), nil }); err == nil {
			t.Errorf("%s replaced", name)
		}
	}
	for _, name := range []string{"../escape.json", ".hidden.json", "", "a/b.json", ".tmp-x"} {
		if _, err := r.publish("records", name, []byte("x")); err == nil {
			t.Errorf("unsafe record name %q published", name)
		}
	}
}

// A truncated or unpublished temporary left by a crash is crash evidence:
// listing reports it separately, never as a record, and never removes it.
func TestTruncatedTemporariesAreEvidenceNotRecords(t *testing.T) {
	r, dir, _ := newTestRecords(t)
	if _, err := r.publish("records", "a.json", []byte("complete")); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dir, temporaryPrefix+"deadbeefdeadbeef")
	if err := os.WriteFile(stale, []byte(`{"trunc`), 0o600); err != nil {
		t.Fatal(err)
	}
	names, temporaries, err := r.list("records")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, []string{"a.json"}) || !reflect.DeepEqual(temporaries, []string{temporaryPrefix + "deadbeefdeadbeef"}) {
		t.Fatalf("records %v temporaries %v", names, temporaries)
	}
	if _, err := r.publish("records", "b.json", []byte("next")); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(stale); err != nil || string(data) != `{"trunc` {
		t.Fatal("crash evidence was altered")
	}
}

func TestEnsurePrivateDirectory(t *testing.T) {
	base := privateTempDir(t)
	root, err := os.OpenRoot(base)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	previous := syscall.Umask(0o022)
	defer syscall.Umask(previous)
	if err := ensurePrivateDirectory(root, "fresh"); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Lstat(filepath.Join(base, "fresh")); info.Mode().Perm() != 0o700 {
		t.Fatalf("created with %v", info.Mode())
	}
	if err := ensurePrivateDirectory(root, "fresh"); err != nil {
		t.Fatalf("existing private directory refused: %v", err)
	}
	if err := os.Mkdir(filepath.Join(base, "shared"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateDirectory(root, "shared"); err == nil {
		t.Fatal("shared directory accepted")
	}
	if err := os.Symlink(filepath.Join(base, "fresh"), filepath.Join(base, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateDirectory(root, "linked"); err == nil {
		t.Fatal("symlinked directory accepted")
	}
	if err := os.WriteFile(filepath.Join(base, "file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateDirectory(root, "file"); err == nil {
		t.Fatal("regular file accepted as a directory")
	}
	if !strings.HasPrefix(temporaryPrefix, ".") {
		t.Fatal("temporaries must be hidden names")
	}
}

var errInjectedFault = errors.New("injected record fault")

// failAt makes one named step fail; every other step behaves normally.
func failAt(ops recordOps, step string) recordOps {
	fail := func() error { return errInjectedFault }
	switch step {
	case "lock":
		ops.lock = func(*os.File) error { return fail() }
	case "read":
		ops.read = func(*os.Root, string) ([]byte, error) { return nil, fail() }
	case "open-temporary":
		ops.openTemporary = func(*os.Root, string) (*os.File, error) { return nil, fail() }
	case "write":
		ops.write = func(*os.File, []byte) error { return fail() }
	case "sync-file":
		ops.syncFile = func(*os.File) error { return fail() }
	case "close":
		next := ops.closeFile
		ops.closeFile = func(f *os.File) error { _ = next(f); return fail() }
	case "link":
		ops.link = func(*os.Root, string, string) error { return fail() }
	case "rename":
		ops.rename = func(*os.Root, string, string) error { return fail() }
	case "sync-directory":
		ops.syncDirectory = func(*os.File) error { return fail() }
	default:
		panic("unknown step " + step)
	}
	return ops
}
