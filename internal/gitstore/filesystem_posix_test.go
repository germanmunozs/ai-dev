//go:build (darwin && !ios) || (linux && !android)

package gitstore

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"testing"

	"ai-dev/internal/gitmanager"
)

func privateTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestObserveFilesystem(t *testing.T) {
	dir := privateTempDir(t)
	obs, err := ObserveFilesystem(dir)
	if err != nil {
		t.Fatal(err)
	}
	var stat syscall.Stat_t
	if err := syscall.Stat(dir, &stat); err != nil {
		t.Fatal(err)
	}
	if obs.GOOS != runtime.GOOS || obs.FilesystemType == "" || obs.MountIdentity == "" ||
		obs.InodeDecimal != strconv.FormatUint(uint64(stat.Ino), 10) {
		t.Fatalf("observation %+v", obs)
	}
	if runtime.GOOS == "darwin" && (obs.FilesystemType != "apfs" || !obs.Local || obs.ReadOnly) {
		t.Fatalf("macOS temporary directory observed as %+v, expected local writable APFS", obs)
	}
	if _, err := ObserveFilesystem(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing path observed")
	}
	if _, err := ObserveFilesystem("relative"); err == nil {
		t.Fatal("relative path observed")
	}
}

// The probe proves each primitive on the real filesystem and removes only
// the files it created.
func TestProbePrimitives(t *testing.T) {
	dir := privateTempDir(t)
	keep := filepath.Join(dir, "unrelated")
	if err := os.WriteFile(keep, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	primitives, err := ProbePrimitives(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !(primitives.ExclusiveCreate && primitives.AdvisoryLock && primitives.FileSync && primitives.DirectorySync &&
		primitives.AtomicSameFilesystemRename && primitives.AtomicReplace && primitives.StableIdentity) {
		t.Fatalf("primitives %+v", primitives)
	}
	if err := primitives.EvidenceDigest.Validate(); err != nil {
		t.Fatalf("evidence digest: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "unrelated" {
		t.Fatalf("probe left %v, %v", entries, err)
	}
	if data, _ := os.ReadFile(keep); string(data) != "keep" {
		t.Fatal("probe changed an unrelated file")
	}
	obs, err := ObserveFilesystem(dir)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := gitmanager.ClassifyFilesystem(gitmanager.FilesystemRoleStateRoot, obs, primitives)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "darwin" && !profile.ProductionSupported {
		t.Fatalf("probed APFS state root not production supported: %+v", profile)
	}
}

func TestProbeRefusesSharedDirectory(t *testing.T) {
	dir := privateTempDir(t)
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ProbePrimitives(dir); err == nil {
		t.Fatal("probe ran in a directory other users can read")
	}
	link := filepath.Join(privateTempDir(t), "link")
	if err := os.Symlink(privateTempDir(t), link); err != nil {
		t.Fatal(err)
	}
	if _, err := ProbePrimitives(link); err == nil {
		t.Fatal("probe followed a symlinked directory")
	}
}
