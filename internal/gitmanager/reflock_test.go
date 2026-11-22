//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"os"
	"path/filepath"
	"testing"
)

// T128/R-05: reference locks are listed from the shared and the worktree's
// own refs, and an observation a symlink or a missing piece could make
// incomplete is reported as such instead of as "no locks".
func TestReferenceLockObservation(t *testing.T) {
	setup := func(t *testing.T) (gitDir, commonDir string) {
		commonDir = physicalTempDir(t)
		mustMkdir(t, filepath.Join(commonDir, "refs", "heads"))
		return commonDir, commonDir
	}
	t.Run("absent refs", func(t *testing.T) {
		dir := physicalTempDir(t)
		if locks, ok := referenceLocks(dir, dir); !ok || len(locks) != 0 {
			t.Fatalf("locks %v ok %v", locks, ok)
		}
	})
	t.Run("locks listed", func(t *testing.T) {
		gitDir, commonDir := setup(t)
		for _, name := range []string{"HEAD.lock", "packed-refs.lock", "refs/heads/main.lock"} {
			mustWrite(t, filepath.Join(commonDir, name), "")
		}
		locks, ok := referenceLocks(gitDir, commonDir)
		if !ok || len(locks) != 3 {
			t.Fatalf("locks %v ok %v", locks, ok)
		}
	})
	t.Run("linked worktree refs", func(t *testing.T) {
		_, commonDir := setup(t)
		gitDir := filepath.Join(commonDir, "worktrees", "w")
		mustMkdir(t, filepath.Join(gitDir, "refs", "bisect"))
		mustWrite(t, filepath.Join(gitDir, "refs", "bisect", "bad.lock"), "")
		if locks, ok := referenceLocks(gitDir, commonDir); !ok || len(locks) != 1 || locks[0] != "refs/bisect/bad.lock" {
			t.Fatalf("locks %v ok %v", locks, ok)
		}
	})
	t.Run("symlinked refs directory", func(t *testing.T) {
		dir := physicalTempDir(t)
		other := physicalTempDir(t)
		mustMkdir(t, filepath.Join(other, "heads"))
		if err := os.Symlink(other, filepath.Join(dir, "refs")); err != nil {
			t.Fatal(err)
		}
		if _, ok := referenceLocks(dir, dir); ok {
			t.Fatal("a symlinked refs directory was observed as complete")
		}
	})
	t.Run("symlink inside refs", func(t *testing.T) {
		gitDir, commonDir := setup(t)
		if err := os.Symlink(physicalTempDir(t), filepath.Join(commonDir, "refs", "heads", "elsewhere")); err != nil {
			t.Fatal(err)
		}
		if _, ok := referenceLocks(gitDir, commonDir); ok {
			t.Fatal("a symlink inside refs was observed as complete")
		}
	})
}
