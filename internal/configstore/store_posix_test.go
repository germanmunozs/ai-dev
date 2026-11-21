//go:build (darwin && !ios) || (linux && !android)

package configstore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func readConfig(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".ai-dev", "agents.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func entries(t *testing.T, dir string) []string {
	t.Helper()
	list, err := os.ReadDir(filepath.Join(dir, ".ai-dev"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range list {
		names = append(names, entry.Name())
	}
	return names
}

// A new configuration creates .ai-dev (0755) and agents.json (0644) and
// leaves no temporary file (F011-FR-007).
func TestWriteCreates(t *testing.T) {
	dir := t.TempDir()
	if written, warnings := New(dir).Write([]byte("one\n")); !written || len(warnings) != 0 {
		t.Fatalf("written=%v warnings=%v", written, warnings)
	}
	if got := readConfig(t, dir); got != "one\n" {
		t.Fatalf("content %q", got)
	}
	dirInfo, _ := os.Stat(filepath.Join(dir, ".ai-dev"))
	fileInfo, _ := os.Stat(filepath.Join(dir, ".ai-dev", "agents.json"))
	if dirInfo.Mode().Perm() != 0o755 || fileInfo.Mode().Perm() != 0o644 {
		t.Fatalf("modes dir=%v file=%v", dirInfo.Mode(), fileInfo.Mode())
	}
	if got := entries(t, dir); len(got) != 1 {
		t.Fatalf("entries %v", got)
	}
}

func TestWriteReplacesKeepingPermissions(t *testing.T) {
	dir := t.TempDir()
	New(dir).Write([]byte("one\n"))
	if err := os.Chmod(filepath.Join(dir, ".ai-dev", "agents.json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if written, _ := New(dir).Write([]byte("two\n")); !written {
		t.Fatalf("written=%v", written)
	}
	info, _ := os.Stat(filepath.Join(dir, ".ai-dev", "agents.json"))
	if readConfig(t, dir) != "two\n" || info.Mode().Perm() != 0o600 || len(entries(t, dir)) != 1 {
		t.Fatalf("content=%q mode=%v entries=%v", readConfig(t, dir), info.Mode(), entries(t, dir))
	}
}

// Nothing is ever written through a symbolic link (F011-FR-007).
func TestWriteRefusesSymlinksAndNonRegularFiles(t *testing.T) {
	t.Run("symlinked directory", func(t *testing.T) {
		dir, target := t.TempDir(), t.TempDir()
		if err := os.Symlink(target, filepath.Join(dir, ".ai-dev")); err != nil {
			t.Fatal(err)
		}
		if written, _ := New(dir).Write([]byte("x")); written {
			t.Fatal("wrote through a symlinked .ai-dev")
		}
		if list, _ := os.ReadDir(target); len(list) != 0 {
			t.Fatalf("target changed: %v", list)
		}
	})
	t.Run("symlinked file", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(t.TempDir(), "victim")
		os.WriteFile(target, []byte("victim\n"), 0o644)
		os.Mkdir(filepath.Join(dir, ".ai-dev"), 0o755)
		if err := os.Symlink(target, filepath.Join(dir, ".ai-dev", "agents.json")); err != nil {
			t.Fatal(err)
		}
		if written, _ := New(dir).Write([]byte("x")); written {
			t.Fatal("replaced a symlinked agents.json")
		}
		if data, _ := os.ReadFile(target); string(data) != "victim\n" {
			t.Fatalf("target changed: %q", data)
		}
		if info, _ := os.Lstat(filepath.Join(dir, ".ai-dev", "agents.json")); info.Mode()&os.ModeSymlink == 0 {
			t.Fatal("symlink was replaced")
		}
	})
	t.Run("agents.json is a directory", func(t *testing.T) {
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, ".ai-dev", "agents.json"), 0o755)
		if written, _ := New(dir).Write([]byte("x")); written {
			t.Fatal("replaced a directory")
		}
	})
	t.Run(".ai-dev is a file", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, ".ai-dev"), []byte("x"), 0o644)
		if written, _ := New(dir).Write([]byte("x")); written {
			t.Fatal("wrote into a file path")
		}
	})
}

// A swap of .ai-dev between the check and the open is detected.
func TestWriteDetectsDirectorySwap(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, ".ai-dev"), 0o755)
	store := newTestStore(dir)
	store.ops.afterStep = func(step string) {
		if step == "dir-checked" {
			os.Rename(filepath.Join(dir, ".ai-dev"), filepath.Join(dir, "moved"))
			os.Mkdir(filepath.Join(dir, ".ai-dev"), 0o755)
		}
	}
	if written, _ := store.Write([]byte("x")); written {
		t.Fatal("swap not detected")
	}
	if _, err := os.Stat(filepath.Join(dir, ".ai-dev", "agents.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("written after swap: %v", err)
	}
}

// A failure at any step before rename leaves the previous file and no
// temporary file; a directory-sync failure after rename is a warning.
func TestWriteFaults(t *testing.T) {
	injected := errors.New("injected")
	for _, step := range []string{"openTmp", "writeTmp", "syncTmp", "closeTmp", "rename"} {
		t.Run(step, func(t *testing.T) {
			dir := t.TempDir()
			New(dir).Write([]byte("previous\n"))
			store := newTestStore(dir)
			store.fail(step, injected)
			if written, _ := store.Write([]byte("next\n")); written {
				t.Fatalf("written=%v", written)
			}
			if got := readConfig(t, dir); got != "previous\n" || len(entries(t, dir)) != 1 {
				t.Fatalf("content=%q entries=%v", got, entries(t, dir))
			}
		})
	}
	t.Run("syncDir", func(t *testing.T) {
		dir := t.TempDir()
		store := newTestStore(dir)
		store.fail("syncDir", injected)
		written, warnings := store.Write([]byte("next\n"))
		if !written || len(warnings) != 1 || !strings.Contains(warnings[0], "flush") {
			t.Fatalf("written=%v warnings=%v", written, warnings)
		}
	})
}

func newTestStore(dir string) *posixStore {
	return &posixStore{dir: dir, ops: defaultOps()}
}

func (s *posixStore) fail(step string, err error) {
	switch step {
	case "openTmp":
		s.ops.openTmp = func(*os.Root, string) (*os.File, error) { return nil, err }
	case "writeTmp":
		s.ops.writeTmp = func(*os.File, []byte) error { return err }
	case "syncTmp":
		s.ops.syncTmp = func(*os.File) error { return err }
	case "closeTmp":
		close := s.ops.closeTmp
		s.ops.closeTmp = func(file *os.File) error { close(file); return err }
	case "rename":
		s.ops.rename = func(*os.Root, string, string) error { return err }
	case "syncDir":
		s.ops.syncDir = func(*os.File) error { return err }
	}
}

// .ai-dev gets mode 0755 even under a restrictive umask, because Feature 008
// requires that mode on an existing .ai-dev before it writes config.json.
func TestWriteCreatesDirectoryWithModeUnderUmask(t *testing.T) {
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)
	dir := t.TempDir()
	if written, _ := New(dir).Write([]byte("x\n")); !written {
		t.Fatal("not written")
	}
	info, err := os.Stat(filepath.Join(dir, ".ai-dev"))
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("mode %v, %v", info.Mode(), err)
	}
}

// Writing agents.json never touches Feature 008's config.json beside it.
func TestWriteLeavesFeature008ConfigAlone(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".ai-dev"), 0o755); err != nil {
		t.Fatal(err)
	}
	config008 := filepath.Join(dir, ".ai-dev", "config.json")
	content := `{"schema_version":"ai-dev.project-config/v1"}` + "\n"
	if err := os.WriteFile(config008, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if written, _ := New(dir).Write([]byte("agents\n")); !written {
		t.Fatal("not written")
	}
	if data, _ := os.ReadFile(config008); string(data) != content {
		t.Fatalf("config.json changed: %q", data)
	}
	if got := entries(t, dir); len(got) != 2 {
		t.Fatalf("entries %v", got)
	}
}
