//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func outputFixture(t *testing.T) (*planFixture, PlanOutputRequest, []byte) {
	f := newPlanFixture(t)
	f.existingRepository()
	result := mustPlan(t, f, StrategyRequest{Strategy: "trunk-based", Branches: map[string]string{"trunk-branch": "main"}})
	data, err := EncodePlanFile(result.Plan)
	if err != nil {
		t.Fatal(err)
	}
	return f, PlanOutputRequest{WorkingDir: "/", Inspection: f.inspect(), ObserveFilesystem: f.inspectFixture.deps.ObserveFilesystem}, data
}

func TestWritePlanOutput(t *testing.T) {
	f, req, data := outputFixture(t)
	outside := physicalTempDir(t)
	req.Path = filepath.Join(outside, "plan.json")
	written, rejection := WritePlanOutput(req, data)
	if rejection != nil || written != req.Path {
		t.Fatalf("write %q %+v", written, rejection)
	}
	info, err := os.Lstat(written)
	got, _ := os.ReadFile(written)
	if err != nil || info.Mode().Perm() != 0o600 || !bytes.Equal(got, data) {
		t.Fatalf("output %v %v", info, err)
	}
	// Never overwritten.
	_, rejection = WritePlanOutput(req, []byte("other"))
	expectRejection(t, rejection, ReasonOutputExists, 2)
	if again, _ := os.ReadFile(written); !bytes.Equal(again, data) {
		t.Fatal("existing plan overwritten")
	}
	// A dangling symlink at the destination also exists.
	dangling := filepath.Join(outside, "dangling.json")
	if err := os.Symlink(filepath.Join(outside, "nowhere"), dangling); err != nil {
		t.Fatal(err)
	}
	req.Path = dangling
	_, rejection = WritePlanOutput(req, data)
	expectRejection(t, rejection, ReasonOutputExists, 2)
	_ = f
}

func TestWritePlanOutputRefusesUnsafeDestinations(t *testing.T) {
	f, req, data := outputFixture(t)
	mustMkdir(t, filepath.Join(f.root, "sub"))
	aliasDir := physicalTempDir(t)
	alias := filepath.Join(aliasDir, "project-alias")
	if err := os.Symlink(f.root, alias); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{
		"project root":           filepath.Join(f.root, "plan.json"),
		"project subdirectory":   filepath.Join(f.root, "sub", "plan.json"),
		"git directory":          filepath.Join(f.root, ".git", "plan.json"),
		"symlink to the project": filepath.Join(alias, "plan.json"),
		"directory name":         physicalTempDir(t) + "/",
		"empty":                  "",
	} {
		before, _ := os.ReadDir(f.root)
		req.Path = path
		_, rejection := WritePlanOutput(req, data)
		expectRejection(t, rejection, ReasonUnsafeOutputPath, 2)
		if after, _ := os.ReadDir(f.root); len(after) != len(before) {
			t.Fatalf("%s: refusal wrote into the project", name)
		}
	}
	// A destination on an unsupported filesystem is refused before writing.
	req.ObserveFilesystem = func(string) (FilesystemObservation, error) {
		return FilesystemObservation{GOOS: "linux", FilesystemType: "tmpfs", MountIdentity: "tmp", DeviceDecimal: "1", InodeDecimal: "2", Local: true}, nil
	}
	outside := physicalTempDir(t)
	req.Path = filepath.Join(outside, "plan.json")
	_, rejection := WritePlanOutput(req, data)
	expectRejection(t, rejection, ReasonUnsafeOutputPath, 2)
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatal("refusal created a file")
	}
}

// A failure after creation removes only the file this call created.
func TestWritePlanOutputPartialFailure(t *testing.T) {
	_, req, data := outputFixture(t)
	outside := physicalTempDir(t)
	req.Path = filepath.Join(outside, "plan.json")
	original := planOutputWrite
	t.Cleanup(func() { planOutputWrite = original })
	planOutputWrite = func(file *os.File, data []byte) error {
		_, _ = file.Write(data[:len(data)/2])
		return errors.New("disk full")
	}
	_, rejection := WritePlanOutput(req, data)
	expectRejection(t, rejection, ReasonOutputFailed, 2)
	if _, err := os.Lstat(req.Path); !os.IsNotExist(err) {
		t.Fatal("partial plan file left behind")
	}
}
