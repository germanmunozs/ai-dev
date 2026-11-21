//go:build (darwin && !ios) || (linux && !android)

package gitstore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/gitmanager"
)

func stateRootPath(t *testing.T) (gitmanager.ObservedPath, string) {
	t.Helper()
	path := filepath.Join(privateTempDir(t), "state", "ai-dev", "git")
	return gitmanager.NewObservedPath([]byte(path)), path
}

func TestOpenCreatesPrivateLayout(t *testing.T) {
	root, path := stateRootPath(t)
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, dir := range []string{"", "attempts", "environment", "locks", "projects"} {
		info, err := os.Lstat(filepath.Join(path, dir))
		if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
			t.Fatalf("%q: %v %v", dir, info, err)
		}
	}
	for _, leaf := range []string{gitmanager.EnvironmentHomeLeaf, gitmanager.EnvironmentXDGLeaf} {
		if _, err := os.Lstat(filepath.Join(path, leaf)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("environment leaf %s exists: %v", leaf, err)
		}
	}
	project := gitmanager.ComputeProjectIdentityDigest([]byte("/tmp/project"), "1", "2")
	dir, err := store.projectDirectory(project)
	if err != nil {
		t.Fatal(err)
	}
	if want := "projects/" + strings.TrimPrefix(string(project), "sha256:"); dir != want {
		t.Fatalf("project directory %q want %q", dir, want)
	}
	for _, sub := range []string{"", "authorizations", "attempts"} {
		if info, err := os.Lstat(filepath.Join(path, dir, sub)); err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("%s/%s: %v", dir, sub, err)
		}
	}
	if _, err := store.projectDirectory("sha256:../../etc"); err == nil {
		t.Fatal("malformed project digest accepted")
	}
	store.Close()
	// Reopening an existing private layout succeeds.
	again, err := Open(root)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	again.Close()
}

func TestOpenRefusesUnsafeState(t *testing.T) {
	expectRefusal := func(t *testing.T, err error) {
		t.Helper()
		var platformErr *PlatformError
		if !errors.As(err, &platformErr) || platformErr.Reason != gitmanager.ReasonAuditUnavailable {
			t.Fatalf("got %v, want audit_unavailable", err)
		}
	}
	t.Run("shared state root", func(t *testing.T) {
		root, path := stateRootPath(t)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := Open(root)
		expectRefusal(t, err)
	})
	t.Run("present environment leaf", func(t *testing.T) {
		root, path := stateRootPath(t)
		if err := os.MkdirAll(filepath.Join(path, gitmanager.EnvironmentHomeLeaf), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filepath.Join(path, "environment"), 0o700); err != nil {
			t.Fatal(err)
		}
		_, err := Open(root)
		expectRefusal(t, err)
	})
	t.Run("symlinked state root", func(t *testing.T) {
		target := privateTempDir(t)
		link := filepath.Join(privateTempDir(t), "git")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		_, err := Open(gitmanager.NewObservedPath([]byte(link)))
		expectRefusal(t, err)
	})
	t.Run("shared subdirectory", func(t *testing.T) {
		root, path := stateRootPath(t)
		if err := os.MkdirAll(filepath.Join(path, "projects"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filepath.Join(path, "projects"), 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := Open(root)
		expectRefusal(t, err)
	})
	if _, err := Open(gitmanager.NewObservedPath([]byte("relative/git"))); err == nil {
		t.Fatal("relative state root accepted")
	}
}
