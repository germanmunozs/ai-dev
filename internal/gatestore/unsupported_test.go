package gatestore

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"ai-dev/internal/gate"
	"ai-dev/internal/workflow"
)

func TestUnsupportedStoreFailsClosedWithoutFilesystemAccess(t *testing.T) {
	repo := t.TempDir()
	dir := filepath.Join(repo, "specs", "f")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "marker")
	if err := os.WriteFile(marker, []byte("unchanged"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}

	status, warnings := (unsupportedStore{}).Commit("f", workflow.Draft, testEvidence(workflow.Draft, workflow.Specified, "", false))
	if status != gate.Failed {
		t.Fatalf("status=%v", status)
	}
	want := []string{unsupportedPlatformWarning}
	if !reflect.DeepEqual(warnings, want) {
		t.Fatalf("warnings=%v, want %v", warnings, want)
	}
	after, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatal("repository changed")
	}
}
