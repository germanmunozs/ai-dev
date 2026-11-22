package sdd

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/germanmunozs/ai-dev/internal/workflow"
)

type caseInsensitiveFS struct {
	inner fstest.MapFS
}

func (c caseInsensitiveFS) resolve(name string) string {
	for stored := range c.inner {
		if strings.EqualFold(stored, name) {
			return stored
		}
	}
	return name
}

func (c caseInsensitiveFS) Open(name string) (fs.File, error) {
	return c.inner.Open(c.resolve(name))
}

func (c caseInsensitiveFS) Stat(name string) (fs.FileInfo, error) {
	return c.inner.Stat(c.resolve(name))
}

func (c caseInsensitiveFS) Lstat(name string) (fs.FileInfo, error) {
	return c.inner.Lstat(c.resolve(name))
}

func (c caseInsensitiveFS) ReadLink(name string) (string, error) {
	return c.inner.ReadLink(c.resolve(name))
}

func (c caseInsensitiveFS) ReadFile(name string) ([]byte, error) {
	return c.inner.ReadFile(c.resolve(name))
}

func (c caseInsensitiveFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return c.inner.ReadDir(c.resolve(name))
}

func TestCaseInsensitiveFSSelfCheck(t *testing.T) {
	fixture := sddFixture(nil, "Plan.md")
	root := caseInsensitiveFS{inner: fixture}
	dir, _ := workflow.FeatureDir(sddFeatureID)
	if _, err := root.Lstat(dir + "/plan.md"); err != nil {
		t.Fatalf("case-insensitive Lstat did not resolve stored Plan.md: %v", err)
	}
}

func TestArtifactNamesRemainCaseExactOnSimulatedCaseInsensitiveFS(t *testing.T) {
	cases := []struct {
		name    string
		state   workflow.State
		files   []string
		missing []string
	}{
		{"Plan.md", workflow.Planned, []string{SpecArtifact, "Plan.md"}, []string{PlanArtifact}},
		{"SPEC.md", workflow.Specified, []string{"SPEC.md"}, []string{SpecArtifact}},
		{"Tasks.md", workflow.Tasked, []string{SpecArtifact, PlanArtifact, "Tasks.md"}, []string{TasksArtifact}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := sddFixture(nil, tc.files...)
			rec := &recordingSDDFS{inner: caseInsensitiveFS{inner: base}}
			got := missingArtifacts(rec, sddFeatureID, tc.state)
			if !reflect.DeepEqual(got, tc.missing) {
				t.Fatalf("missingArtifacts = %v, want %v", got, tc.missing)
			}
			if want := []string{"ReadDir:specs/" + sddFeatureID}; !reflect.DeepEqual(rec.calls, want) {
				t.Fatalf("calls = %v, want %v", rec.calls, want)
			}
		})
	}
}

func TestArtifactNamesRemainCaseExactOnRealHost(t *testing.T) {
	probeRoot := t.TempDir()
	probe := filepath.Join(probeRoot, "probe")
	if err := os.WriteFile(probe, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, probeErr := os.Lstat(filepath.Join(probeRoot, "PROBE"))
	t.Logf("host filesystem case-insensitive: %v", probeErr == nil)

	cases := []struct {
		name    string
		state   workflow.State
		files   []string
		missing []string
	}{
		{"Plan.md", workflow.Planned, []string{SpecArtifact, "Plan.md"}, []string{PlanArtifact}},
		{"SPEC.md", workflow.Specified, []string{"SPEC.md"}, []string{SpecArtifact}},
		{"Tasks.md", workflow.Tasked, []string{SpecArtifact, PlanArtifact, "Tasks.md"}, []string{TasksArtifact}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rootDir := t.TempDir()
			dir, _ := workflow.FeatureDir(sddFeatureID)
			featureDir := filepath.Join(rootDir, filepath.FromSlash(dir))
			if err := os.MkdirAll(featureDir, 0o755); err != nil {
				t.Fatal(err)
			}
			for _, name := range tc.files {
				if err := os.WriteFile(filepath.Join(featureDir, name), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := missingArtifacts(os.DirFS(rootDir), sddFeatureID, tc.state); !reflect.DeepEqual(got, tc.missing) {
				t.Fatalf("missingArtifacts = %v, want %v", got, tc.missing)
			}
		})
	}
}
