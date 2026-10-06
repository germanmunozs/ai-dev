package sdd

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"testing/fstest"

	"ai-dev/internal/role"
	"ai-dev/internal/workflow"
)

const sddFeatureID = "005-sdd-workflow"

type sddTestFS interface {
	fs.FS
	fs.StatFS
	fs.ReadLinkFS
	fs.ReadFileFS
	fs.ReadDirFS
}

type recordingSDDFS struct {
	inner sddTestFS
	calls []string
}

func (r *recordingSDDFS) Open(name string) (fs.File, error) {
	r.calls = append(r.calls, "Open:"+name)
	return r.inner.Open(name)
}

func (r *recordingSDDFS) Stat(name string) (fs.FileInfo, error) {
	r.calls = append(r.calls, "Stat:"+name)
	return r.inner.Stat(name)
}

func (r *recordingSDDFS) Lstat(name string) (fs.FileInfo, error) {
	r.calls = append(r.calls, "Lstat:"+name)
	return r.inner.Lstat(name)
}

func (r *recordingSDDFS) ReadLink(name string) (string, error) {
	r.calls = append(r.calls, "ReadLink:"+name)
	return r.inner.ReadLink(name)
}

func (r *recordingSDDFS) ReadFile(name string) ([]byte, error) {
	r.calls = append(r.calls, "ReadFile:"+name)
	return r.inner.ReadFile(name)
}

func (r *recordingSDDFS) ReadDir(name string) ([]fs.DirEntry, error) {
	r.calls = append(r.calls, "ReadDir:"+name)
	return r.inner.ReadDir(name)
}

type faultReadDirFS struct {
	inner   fs.FS
	entries []fs.DirEntry
	err     error
}

func (f faultReadDirFS) Open(name string) (fs.File, error) { return f.inner.Open(name) }
func (f faultReadDirFS) ReadDir(string) ([]fs.DirEntry, error) {
	return f.entries, f.err
}

func sddFixture(record *string, files ...string) fstest.MapFS {
	dir, ok := workflow.FeatureDir(sddFeatureID)
	if !ok {
		panic("invalid shared SDD fixture ID")
	}
	fixture := fstest.MapFS{dir: &fstest.MapFile{Mode: fs.ModeDir | 0o755}}
	if record != nil {
		fixture[dir+"/"+workflow.RecordFileName] = &fstest.MapFile{Data: []byte(*record), Mode: 0o644}
	}
	for _, name := range files {
		fixture[dir+"/"+name] = &fstest.MapFile{Data: []byte("fixture"), Mode: 0o644}
	}
	return fixture
}

func TestNextStepTable(t *testing.T) {
	states := []workflow.State{workflow.Draft, workflow.Specified, workflow.Planned, workflow.Tasked, workflow.Implemented, workflow.Approved, workflow.Abandoned}
	want := map[workflow.State]struct {
		activity Activity
		role     role.Role
	}{
		workflow.Draft:       {ActivitySpecify, role.Specification},
		workflow.Specified:   {ActivityPlan, role.Planning},
		workflow.Planned:     {ActivityTasks, role.Planning},
		workflow.Tasked:      {ActivityImplement, role.Implementation},
		workflow.Implemented: {ActivityReview, role.Review},
	}
	stateTokens := make(map[string]bool)
	for _, state := range states {
		stateTokens[string(state)] = true
		activity, gotRole, ok := nextStep(state)
		if ok != !workflow.Terminal(state) {
			t.Errorf("nextStep(%q) ok = %v, want %v", state, ok, !workflow.Terminal(state))
		}
		entry, expected := want[state]
		if expected && (!ok || activity != entry.activity || gotRole != entry.role) {
			t.Errorf("nextStep(%q) = (%q,%q,%v), want (%q,%q,true)", state, activity, gotRole, ok, entry.activity, entry.role)
		}
		if !expected && (activity != "" || gotRole != "" || ok) {
			t.Errorf("nextStep(%q) = (%q,%q,%v), want empty", state, activity, gotRole, ok)
		}
		if ok {
			if recognized, valid := role.Recognized(string(gotRole)); !valid || recognized != gotRole {
				t.Errorf("nextStep(%q) returned noncanonical role %q", state, gotRole)
			}
			if gotRole == role.Remediation {
				t.Errorf("nextStep(%q) returned remediation", state)
			}
			if stateTokens[string(activity)] {
				t.Errorf("activity %q duplicates a workflow state token", activity)
			}
		}
	}
}

func TestRequiredArtifacts(t *testing.T) {
	cases := []struct {
		state workflow.State
		want  []string
	}{
		{workflow.Draft, nil},
		{workflow.Specified, []string{SpecArtifact}},
		{workflow.Planned, []string{SpecArtifact, PlanArtifact}},
		{workflow.Tasked, []string{SpecArtifact, PlanArtifact, TasksArtifact}},
		{workflow.Implemented, []string{SpecArtifact, PlanArtifact, TasksArtifact}},
		{workflow.Approved, []string{SpecArtifact, PlanArtifact, TasksArtifact}},
		{workflow.Abandoned, nil},
	}
	for _, tc := range cases {
		t.Run(string(tc.state), func(t *testing.T) {
			first := requiredArtifacts(tc.state)
			if !reflect.DeepEqual(first, tc.want) {
				t.Fatalf("requiredArtifacts(%q) = %v, want %v", tc.state, first, tc.want)
			}
			if len(first) > 0 {
				first[0] = "mutated"
				if got := requiredArtifacts(tc.state); !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("requiredArtifacts(%q) reused mutable result: %v", tc.state, got)
				}
			}
		})
	}
}

func TestMissingArtifactsEverySubsetAndAheadArtifacts(t *testing.T) {
	for _, state := range []workflow.State{workflow.Specified, workflow.Planned, workflow.Tasked, workflow.Implemented, workflow.Approved} {
		required := requiredArtifacts(state)
		for mask := 1; mask < 1<<len(required); mask++ {
			var present, wantMissing []string
			for i, name := range required {
				if mask&(1<<i) != 0 {
					wantMissing = append(wantMissing, name)
				} else {
					present = append(present, name)
				}
			}
			present = append(present, "extra.md")
			got := missingArtifacts(sddFixture(nil, present...), sddFeatureID, state)
			if !reflect.DeepEqual(got, wantMissing) {
				t.Fatalf("state %q mask %d: missing = %v, want %v", state, mask, got, wantMissing)
			}
		}
	}

	if got := missingArtifacts(sddFixture(nil, SpecArtifact, PlanArtifact, TasksArtifact), sddFeatureID, workflow.Specified); len(got) != 0 {
		t.Fatalf("ahead-of-state artifacts produced missing list %v", got)
	}
}

func TestMissingArtifactsExactNameAndRegularFileOnly(t *testing.T) {
	dir, _ := workflow.FeatureDir(sddFeatureID)
	cases := map[string]fstest.MapFS{
		"case variant": sddFixture(nil, SpecArtifact, "Plan.md"),
		"directory": {
			dir:                      &fstest.MapFile{Mode: fs.ModeDir | 0o755},
			dir + "/" + SpecArtifact: &fstest.MapFile{Mode: 0o644},
			dir + "/" + PlanArtifact: &fstest.MapFile{Mode: fs.ModeDir | 0o755},
		},
		"symlink to file": {
			dir:                      &fstest.MapFile{Mode: fs.ModeDir | 0o755},
			dir + "/" + SpecArtifact: &fstest.MapFile{Mode: 0o644},
			dir + "/" + PlanArtifact: &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("target")},
			dir + "/target":          &fstest.MapFile{Mode: 0o644},
		},
		"dangling symlink": {
			dir:                      &fstest.MapFile{Mode: fs.ModeDir | 0o755},
			dir + "/" + SpecArtifact: &fstest.MapFile{Mode: 0o644},
			dir + "/" + PlanArtifact: &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("missing")},
		},
		"symlink to directory": {
			dir:                      &fstest.MapFile{Mode: fs.ModeDir | 0o755},
			dir + "/" + SpecArtifact: &fstest.MapFile{Mode: 0o644},
			dir + "/" + PlanArtifact: &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("target-dir")},
			dir + "/target-dir":      &fstest.MapFile{Mode: fs.ModeDir | 0o755},
		},
	}
	for name, fixture := range cases {
		t.Run(name, func(t *testing.T) {
			rec := &recordingSDDFS{inner: fixture}
			got := missingArtifacts(rec, sddFeatureID, workflow.Planned)
			if !reflect.DeepEqual(got, []string{PlanArtifact}) {
				t.Fatalf("missingArtifacts = %v, want [%s]", got, PlanArtifact)
			}
			if want := []string{"ReadDir:" + dir}; !reflect.DeepEqual(rec.calls, want) {
				t.Fatalf("calls = %v, want %v; artifact entries must not be opened or followed", rec.calls, want)
			}
		})
	}
}

func TestMissingArtifactsRealFilesystemSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}
	for _, targetKind := range []string{"file", "dangling", "directory"} {
		t.Run(targetKind, func(t *testing.T) {
			rootDir := t.TempDir()
			dir, _ := workflow.FeatureDir(sddFeatureID)
			featureDir := filepath.Join(rootDir, filepath.FromSlash(dir))
			if err := os.MkdirAll(featureDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(featureDir, SpecArtifact), nil, 0o644); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(featureDir, "target")
			switch targetKind {
			case "file":
				if err := os.WriteFile(target, nil, 0o644); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(target, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(target, filepath.Join(featureDir, PlanArtifact)); err != nil {
				t.Skipf("symlink unsupported: %v", err)
			}
			if got := missingArtifacts(os.DirFS(rootDir), sddFeatureID, workflow.Planned); !reflect.DeepEqual(got, []string{PlanArtifact}) {
				t.Fatalf("missingArtifacts = %v, want [%s]", got, PlanArtifact)
			}
		})
	}
}

func TestMissingArtifactsReadDirErrors(t *testing.T) {
	injected := errors.New("injected ReadDir failure")
	fixture := sddFixture(nil, SpecArtifact, PlanArtifact)
	entries, err := fs.ReadDir(fixture, "specs/"+sddFeatureID)
	if err != nil {
		t.Fatal(err)
	}
	for name, partial := range map[string][]fs.DirEntry{"no entries": nil, "partial entries": entries[:1]} {
		t.Run(name, func(t *testing.T) {
			root := faultReadDirFS{inner: fixture, entries: partial, err: injected}
			want := []string{SpecArtifact, PlanArtifact}
			if got := missingArtifacts(root, sddFeatureID, workflow.Planned); !reflect.DeepEqual(got, want) {
				t.Fatalf("missingArtifacts = %v, want %v", got, want)
			}
		})
	}
}

func TestMissingArtifactsPermissionsAreMetadataOnly(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("POSIX permissions test is not meaningful on this host")
	}

	t.Run("unlistable directory", func(t *testing.T) {
		rootDir := t.TempDir()
		dir, _ := workflow.FeatureDir(sddFeatureID)
		featureDir := filepath.Join(rootDir, filepath.FromSlash(dir))
		if err := os.MkdirAll(featureDir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{SpecArtifact, PlanArtifact} {
			if err := os.WriteFile(filepath.Join(featureDir, name), nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Chmod(featureDir, 0o311); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(featureDir, 0o755) })
		want := []string{SpecArtifact, PlanArtifact}
		if got := missingArtifacts(os.DirFS(rootDir), sddFeatureID, workflow.Planned); !reflect.DeepEqual(got, want) {
			t.Fatalf("missingArtifacts = %v, want %v", got, want)
		}
	})

	t.Run("unreadable regular file counts present", func(t *testing.T) {
		rootDir := t.TempDir()
		dir, _ := workflow.FeatureDir(sddFeatureID)
		featureDir := filepath.Join(rootDir, filepath.FromSlash(dir))
		if err := os.MkdirAll(featureDir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{SpecArtifact, PlanArtifact} {
			if err := os.WriteFile(filepath.Join(featureDir, name), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Chmod(filepath.Join(featureDir, PlanArtifact), 0); err != nil {
			t.Fatal(err)
		}
		if got := missingArtifacts(os.DirFS(rootDir), sddFeatureID, workflow.Planned); len(got) != 0 {
			t.Fatalf("missingArtifacts = %v, want none", got)
		}
	})
}

func TestMissingArtifactsCallBoundary(t *testing.T) {
	for _, state := range []workflow.State{workflow.Draft, workflow.Abandoned, workflow.Specified, workflow.Planned, workflow.Tasked, workflow.Implemented, workflow.Approved} {
		t.Run(string(state), func(t *testing.T) {
			required := requiredArtifacts(state)
			rec := &recordingSDDFS{inner: sddFixture(nil, required...)}
			if got := missingArtifacts(rec, sddFeatureID, state); len(got) != 0 {
				t.Fatalf("missingArtifacts = %v, want none", got)
			}
			if len(required) == 0 {
				if len(rec.calls) != 0 {
					t.Fatalf("calls = %v, want none", rec.calls)
				}
				return
			}
			want := []string{"ReadDir:specs/" + sddFeatureID}
			if !reflect.DeepEqual(rec.calls, want) {
				t.Fatalf("calls = %v, want %v", rec.calls, want)
			}
		})
	}
}
