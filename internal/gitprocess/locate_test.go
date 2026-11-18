package gitprocess

import (
	"os"
	"path/filepath"
	"testing"

	"ai-dev/internal/gitmanager"
)

func TestLocateGit(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(second, "git"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(first, "git"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	path, mode, err := LocateGit("relative" + string(os.PathListSeparator) + first + string(os.PathListSeparator) + second)
	if err != nil || path != filepath.Join(second, "git") || mode != gitmanager.InvocationModeDirect {
		t.Fatalf("%q %s %v", path, mode, err)
	}
	if _, _, err := LocateGit(first); err == nil {
		t.Fatal("non-executable git selected")
	}
	if _, _, err := LocateGit(""); err == nil {
		t.Fatal("empty PATH accepted")
	}
}
