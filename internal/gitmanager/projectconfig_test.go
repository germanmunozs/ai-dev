//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func renderedConfig(t *testing.T, plan GitInitializationPlan) []byte {
	t.Helper()
	intent := plan.ImmutableIntent
	data, err := renderProjectConfig(intent.ProjectID, intent.Strategy, intent.GitFlowDialect, intent.BranchParameters(), intent.PrefixParameters())
	if err != nil || contentDigest(data) != intent.ProjectConfigBytesDigest {
		t.Fatalf("rendering %v", err)
	}
	return data
}

// T090/AC-011/AC-024: the configuration is canonical, credential-free and
// portable: its bytes depend only on the project ID and the explicit
// strategy, never on the physical path, so a clone keeps its identity
// (F008-FR-072–074, 185).
func TestProjectConfigRendering(t *testing.T) {
	f := newPlanFixture(t)
	mustWrite(t, filepath.Join(f.home, ".gitconfig"), "[remote \"origin\"]\n\turl = https://user:secret@example.org/r.git\n")
	plan := mustPlan(t, f, gitFlowRequest("avh")).Plan
	data := renderedConfig(t, plan)
	if _, valid := classifyProjectConfig(data); !valid {
		t.Fatalf("the rendering is not a valid configuration: %s", data)
	}
	for _, forbidden := range []string{"secret", "example.org", f.root, f.home} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("the configuration contains %q: %s", forbidden, data)
		}
	}
	if !strings.HasPrefix(string(data), `{"project_id":"`) || !strings.Contains(string(data), `"git_flow_dialect":"avh"`) {
		t.Fatalf("configuration %s", data)
	}
	// A clone at another path with the same configuration plans a verified
	// no-op and keeps the project ID.
	clone := newPlanFixture(t)
	mustWrite(t, filepath.Join(clone.root, ProjectConfigPath), string(data))
	again := mustPlan(t, clone, gitFlowRequest("avh")).Plan
	if again.ImmutableIntent.ProjectID != plan.ImmutableIntent.ProjectID || !bytes.Equal(renderedConfig(t, again), data) {
		t.Fatal("the clone does not reproduce the configuration")
	}
	for _, operation := range again.DerivedOperations() {
		if operation.Kind == OperationKindProjectConfigWrite || operation.Kind == OperationKindCreateProjectConfigDirectory {
			t.Fatalf("an equal configuration is rewritten: %s", operation.Kind)
		}
	}
	// A different strategy against the same file is a conflict.
	_, rejection := clone.plan(basicNewRepo())
	expectRejection(t, rejection, ReasonConfigurationConflict, 1)
}

// T090/AC-011: the directory is created with exactly 0755 whatever the
// umask, and the configuration is published without replacement with
// exactly 0644; nothing else remains in .ai-dev (F008-FR-076).
func TestProjectConfigWrite(t *testing.T) {
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)
	root := physicalTempDir(t)
	if err := createConfigDirectory(root); err != nil {
		t.Fatal(err)
	}
	if err := createConfigDirectory(root); err == nil {
		t.Fatal("an existing directory was created again")
	}
	var owned *configTemporary
	own := func(temporary *configTemporary) error { owned = temporary; return nil }
	if err := writeProjectConfig(root, []byte("{}\n"), newFakeRandomSource(make([]byte, 64)), own); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{".ai-dev": os.ModeDir | 0o755, ProjectConfigPath: 0o644} {
		if info, err := os.Lstat(filepath.Join(root, path)); err != nil || info.Mode() != want {
			t.Fatalf("%s: %v %v", path, info.Mode(), err)
		}
	}
	if entries, _ := os.ReadDir(filepath.Join(root, ".ai-dev")); len(entries) != 1 || owned == nil || owned.release(context.Background()) != nil {
		t.Fatalf("entries %v, temporary %+v", entries, owned)
	}
	// An existing configuration is never overwritten; the temporary is left
	// to the audited release, which removes only the owned inode.
	err := writeProjectConfig(root, []byte("{\"other\":1}\n"), newFakeRandomSource(bytes.Repeat([]byte{1}, 64)), own)
	if !errors.Is(err, errConfigExists) {
		t.Fatalf("overwrite: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(root, ProjectConfigPath)); string(data) != "{}\n" {
		t.Fatalf("the configuration was replaced: %q", data)
	}
	if err := owned.release(context.Background()); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(filepath.Join(root, ".ai-dev")); len(entries) != 1 {
		t.Fatalf("the temporary was not released: %v", entries)
	}
	// A directory replaced by a symlink is not written through.
	other := physicalTempDir(t)
	if err := os.Symlink(other, filepath.Join(other, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	if err := writeProjectConfig(filepath.Join(root, "alias"), []byte("{}\n"), newFakeRandomSource(make([]byte, 64)), own); err == nil {
		t.Fatal("a configuration was written beneath a symlinked root without .ai-dev")
	}
}

// T090/AC-011: a configuration that appears after revalidation is a failed
// operation and stays untouched.
func TestDispatcherConfigurationWrite(t *testing.T) {
	f := newDispatcherFixture(t, basicNewRepo())
	f.d.deps.Random = rand.Reader
	mkdir, write := f.operation(OperationKindCreateProjectConfigDirectory), f.operation(OperationKindProjectConfigWrite)
	if outcome := f.d.Execute(context.Background(), plannedPermit(t, f.plan, mkdir.OperationID), f.plan, mkdir); outcome != OperationCompleted {
		t.Fatalf("directory %v", outcome)
	}
	mustWrite(t, filepath.Join(f.root, ProjectConfigPath), "foreign\n")
	if outcome := f.d.Execute(context.Background(), plannedPermit(t, f.plan, write.OperationID), f.plan, write); outcome != OperationFailed {
		t.Fatalf("write over a foreign configuration %v", outcome)
	}
	if data, _ := os.ReadFile(filepath.Join(f.root, ProjectConfigPath)); string(data) != "foreign\n" {
		t.Fatal("a foreign configuration was overwritten")
	}
	os.Remove(filepath.Join(f.root, ProjectConfigPath))
	if outcome := f.d.Execute(context.Background(), plannedPermit(t, f.plan, write.OperationID), f.plan, write); outcome != OperationCompleted {
		t.Fatalf("write %v", outcome)
	}
	if data, _ := os.ReadFile(filepath.Join(f.root, ProjectConfigPath)); !bytes.Equal(data, renderedConfig(t, f.plan)) {
		t.Fatalf("written %q", data)
	}
}
