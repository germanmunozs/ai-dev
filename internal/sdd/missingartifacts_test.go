package sdd

import (
	"errors"
	"io/fs"
	"reflect"
	"testing"
	"testing/fstest"

	"ai-dev/internal/workflow"
)

func TestMissingArtifactsExportedWrapper(t *testing.T) {
	dir, _ := workflow.FeatureDir(sddFeatureID)
	cases := []struct {
		name  string
		root  fs.FS
		id    string
		state workflow.State
	}{
		{"draft", sddFixture(nil), sddFeatureID, workflow.Draft},
		{"specified present", sddFixture(nil, SpecArtifact), sddFeatureID, workflow.Specified},
		{"planned missing", sddFixture(nil, SpecArtifact), sddFeatureID, workflow.Planned},
		{"case variant", caseInsensitiveFS{inner: sddFixture(nil, SpecArtifact, "Plan.md")}, sddFeatureID, workflow.Planned},
		{"directory", fstestDirectoryArtifact(dir), sddFeatureID, workflow.Planned},
		{"symlink", fstestSymlinkArtifact(dir), sddFeatureID, workflow.Planned},
		{"invalid id", sddFixture(nil), "../x", workflow.Planned},
		{"abandoned", sddFixture(nil), sddFeatureID, workflow.Abandoned},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MissingArtifacts(tc.root, tc.id, tc.state)
			want := missingArtifacts(tc.root, tc.id, tc.state)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("MissingArtifacts = %v, want %v", got, want)
			}
		})
	}

	fault := faultReadDirFS{inner: sddFixture(nil, SpecArtifact), err: errors.New("read dir")}
	if got, want := MissingArtifacts(fault, sddFeatureID, workflow.Planned), missingArtifacts(fault, sddFeatureID, workflow.Planned); !reflect.DeepEqual(got, want) {
		t.Fatalf("fault result = %v, want %v", got, want)
	}
}

func TestMissingArtifactsExportedWrapperCalls(t *testing.T) {
	dir, _ := workflow.FeatureDir(sddFeatureID)
	rec := &recordingSDDFS{inner: sddFixture(nil, SpecArtifact)}
	if got := MissingArtifacts(rec, sddFeatureID, workflow.Planned); !reflect.DeepEqual(got, []string{PlanArtifact}) {
		t.Fatalf("missing = %v", got)
	}
	if want := []string{"ReadDir:" + dir}; !reflect.DeepEqual(rec.calls, want) {
		t.Fatalf("calls = %v, want %v", rec.calls, want)
	}
	for _, state := range []workflow.State{workflow.Draft, workflow.Abandoned} {
		rec.calls = nil
		if got := MissingArtifacts(rec, sddFeatureID, state); got != nil || len(rec.calls) != 0 {
			t.Fatalf("state %s: missing=%v calls=%v", state, got, rec.calls)
		}
	}
}

func fstestDirectoryArtifact(dir string) fs.FS {
	fixture := sddFixture(nil, SpecArtifact)
	fixture[dir+"/"+PlanArtifact] = &fstest.MapFile{Mode: fs.ModeDir | 0o755}
	return fixture
}

func fstestSymlinkArtifact(dir string) fs.FS {
	fixture := sddFixture(nil, SpecArtifact)
	fixture[dir+"/"+PlanArtifact] = &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("target")}
	return fixture
}
