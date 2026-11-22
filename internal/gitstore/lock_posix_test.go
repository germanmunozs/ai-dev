//go:build (darwin && !ios) || (linux && !android)

package gitstore

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/gitmanager"
)

func lockFile(statePath string, project gitmanager.PhysicalProjectIdentity) string {
	sum := sha256.Sum256([]byte("ai-dev-lock-v1\x00" + project.DeviceDecimal() + "\x00" + project.InodeDecimal()))
	return filepath.Join(statePath, "locks", "by-device-inode", fmt.Sprintf("%x", sum), "apply.lock")
}

func mustAcquire(t *testing.T, store *Store, project gitmanager.PhysicalProjectIdentity, owner gitmanager.ApplyID) gitmanager.ApplyLock {
	t.Helper()
	lock, err := store.AcquireApplyLock(context.Background(), project, owner)
	if err != nil {
		t.Fatal(err)
	}
	return lock
}

func expectLockConflict(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, gitmanager.ErrApplyLockHeld) {
		t.Fatalf("got %v, want lock conflict", err)
	}
}

// The persistent file is the exclusion resource: an active holder refuses
// a second acquisition, and once released the same file is reused and
// never deleted.
func TestApplyLockActiveHolderAndFreeReuse(t *testing.T) {
	ctx := context.Background()
	store, statePath := openTestStore(t)
	project := testProject(t, privateTempDir(t))
	lock := mustAcquire(t, store, project, testApplyID)
	_, err := store.AcquireApplyLock(ctx, project, testOtherApplyID)
	expectLockConflict(t, err)
	file := lockFile(statePath, project)
	info, err := os.Lstat(file)
	if err != nil || info.Mode().Perm() != 0o600 || !info.Mode().IsRegular() {
		t.Fatalf("lock file %v %v", info, err)
	}
	// The namespace is exactly locks/by-device-inode/<sha256 hex>; no part
	// of the project path enters it.
	relative, _ := filepath.Rel(statePath, file)
	parts := strings.Split(relative, string(filepath.Separator))
	if len(parts) != 4 || parts[0] != "locks" || parts[1] != "by-device-inode" || len(parts[2]) != 64 ||
		strings.Trim(parts[2], "0123456789abcdef") != "" || strings.Contains(file, string(project.ReversiblePathBytes())) {
		t.Fatalf("lock namespace %q", relative)
	}
	if err := lock.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if err := lock.Release(ctx); err == nil {
		t.Fatal("lock released twice")
	}
	reused := mustAcquire(t, store, project, testOtherApplyID)
	if again, err := os.Lstat(file); err != nil || !os.SameFile(info, again) {
		t.Fatal("free lock was not reused in place")
	}
	if err := reused.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(file); err != nil {
		t.Fatal("release deleted the persistent lock file")
	}
}

// Aliases of one live directory contend, and a moved project keeps its
// physical namespace; neither depends on the documentary path.
func TestApplyLockAliasesAndMoves(t *testing.T) {
	ctx := context.Background()
	store, _ := openTestStore(t)
	parent := privateTempDir(t)
	dir := filepath.Join(parent, "project")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	project := testProject(t, dir)
	link := filepath.Join(privateTempDir(t), "alias")
	if err := os.Symlink(parent, link); err != nil {
		t.Fatal(err)
	}
	aliasPath := filepath.Join(link, "project")
	alias, err := gitmanager.NewPhysicalProjectIdentity(aliasPath, []byte(aliasPath), project.DeviceDecimal(), project.InodeDecimal())
	if err != nil {
		t.Fatal(err)
	}
	if alias.Digest() == project.Digest() {
		t.Fatal("alias shares the documentary identity")
	}
	lock := mustAcquire(t, store, project, testApplyID)
	_, err = store.AcquireApplyLock(ctx, alias, testOtherApplyID)
	expectLockConflict(t, err)
	if err := lock.Release(ctx); err != nil {
		t.Fatal(err)
	}

	moved := filepath.Join(parent, "renamed")
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}
	// The old documentary path no longer proves the physical object.
	_, err = store.AcquireApplyLock(ctx, project, testOtherApplyID)
	expectLockConflict(t, err)
	current := testProject(t, moved)
	if current.DeviceDecimal() != project.DeviceDecimal() || current.InodeDecimal() != project.InodeDecimal() {
		t.Fatal("rename changed the physical identity")
	}
	mustAcquire(t, store, current, testOtherApplyID).Release(ctx)
}

// A pre-existing free lock file for a recycled device/inode is reused once
// the current object is proven; an identity the current object does not
// match is ambiguous and fails closed.
func TestApplyLockRecycledAndAmbiguousIdentity(t *testing.T) {
	ctx := context.Background()
	store, statePath := openTestStore(t)
	project := testProject(t, privateTempDir(t))
	file := lockFile(statePath, project)
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	mustAcquire(t, store, project, testApplyID).Release(ctx)

	stale, err := gitmanager.NewPhysicalProjectIdentity(string(project.ReversiblePathBytes()), project.ReversiblePathBytes(), project.DeviceDecimal(), "1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.AcquireApplyLock(ctx, stale, testApplyID)
	expectLockConflict(t, err)
	missingPath := filepath.Join(privateTempDir(t), "missing")
	missing, _ := gitmanager.NewPhysicalProjectIdentity(missingPath, []byte(missingPath), project.DeviceDecimal(), project.InodeDecimal())
	_, err = store.AcquireApplyLock(ctx, missing, testApplyID)
	expectLockConflict(t, err)
}

// Lock reuse is independent of authorization state: an earlier consumed
// authorization constrains only itself.
func TestApplyLockIgnoresConsumedAuthorizations(t *testing.T) {
	ctx := context.Background()
	store, _ := openTestStore(t)
	project := testProject(t, privateTempDir(t))
	authorization := testAuthorization(t, project)
	if err := store.IssueAuthorization(ctx, authorization); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.TransitionAuthorization(ctx, transition(authorization, gitmanager.AuthorizationStateClaimed, testApplyID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.TransitionAuthorization(ctx, transition(claimed, gitmanager.AuthorizationStateMutationStarted, testApplyID)); err != nil {
		t.Fatal(err)
	}
	mustAcquire(t, store, project, testOtherApplyID).Release(ctx)
}

// The diagnostic holder record names the current holder, is rebuilt from
// the current proofs on every acquisition, and grants nothing.
func TestApplyLockDiagnostics(t *testing.T) {
	ctx := context.Background()
	store, statePath := openTestStore(t)
	project := testProject(t, privateTempDir(t))
	holder := filepath.Join(filepath.Dir(lockFile(statePath, project)), "holder.json")
	lock := mustAcquire(t, store, project, testApplyID)
	data, err := os.ReadFile(holder)
	if err != nil || !strings.Contains(string(data), string(testApplyID)) || strings.Contains(string(data), string(project.ReversiblePathBytes())) {
		t.Fatalf("holder record %q %v", data, err)
	}
	if info, _ := os.Lstat(holder); info.Mode().Perm() != 0o600 {
		t.Fatalf("holder mode %v", info.Mode())
	}
	lock.Release(ctx)
	if err := os.WriteFile(holder, []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	lock = mustAcquire(t, store, project, testOtherApplyID)
	if data, _ := os.ReadFile(holder); !strings.Contains(string(data), string(testOtherApplyID)) {
		t.Fatalf("stale diagnostics not rebuilt: %q", data)
	}
	lock.Release(ctx)
	// Metadata that cannot be rebuilt fails closed and deletes nothing.
	if err := os.Remove(holder); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(holder, 0o700); err != nil {
		t.Fatal(err)
	}
	_, err = store.AcquireApplyLock(ctx, project, testApplyID)
	expectLockConflict(t, err)
	if _, err := os.Lstat(lockFile(statePath, project)); err != nil {
		t.Fatal("refusal deleted the lock file")
	}
	// The refused acquisition left the lock free.
	if err := os.Remove(holder); err != nil {
		t.Fatal(err)
	}
	mustAcquire(t, store, project, testApplyID).Release(ctx)
}

// A lock file that is not a private regular file owned by this user is
// never used, repaired or deleted.
func TestApplyLockRefusesUnsafeLockFiles(t *testing.T) {
	ctx := context.Background()
	for name, setup := range map[string]func(t *testing.T, file string){
		"symlink": func(t *testing.T, file string) {
			target := filepath.Join(privateTempDir(t), "target")
			if err := os.WriteFile(target, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, file); err != nil {
				t.Fatal(err)
			}
		},
		"shared mode": func(t *testing.T, file string) { writeFile(t, file, 0o644) },
		"directory": func(t *testing.T, file string) {
			if err := os.Mkdir(file, 0o700); err != nil {
				t.Fatal(err)
			}
		},
		"hard link": func(t *testing.T, file string) {
			writeFile(t, file, 0o600)
			if err := os.Link(file, filepath.Join(filepath.Dir(file), "second")); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			store, statePath := openTestStore(t)
			project := testProject(t, privateTempDir(t))
			file := lockFile(statePath, project)
			if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
				t.Fatal(err)
			}
			setup(t, file)
			before, _ := os.Lstat(file)
			_, err := store.AcquireApplyLock(ctx, project, testApplyID)
			expectLockConflict(t, err)
			after, err := os.Lstat(file)
			if err != nil || after.Mode() != before.Mode() || !os.SameFile(before, after) {
				t.Fatalf("unsafe lock file altered: %v %v", after, err)
			}
		})
	}
}

// If the lock file is replaced while held, release still unlocks the held
// descriptor but reports that ownership could not be verified.
func TestApplyLockReleaseDetectsReplacement(t *testing.T) {
	ctx := context.Background()
	store, statePath := openTestStore(t)
	project := testProject(t, privateTempDir(t))
	lock := mustAcquire(t, store, project, testApplyID)
	file := lockFile(statePath, project)
	if err := os.Rename(file, file+".moved"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, file, 0o600)
	if err := lock.Release(ctx); !errors.Is(err, gitmanager.ErrOwnedResourceReplaced) {
		t.Fatalf("release of a replaced lock: %v", err)
	}
	if _, err := os.Lstat(file + ".moved"); err != nil {
		t.Fatal("release removed the moved lock file")
	}
}
