package gitstore

import (
	"errors"
	"go/build"
	"testing"

	"ai-dev/internal/gitmanager"
)

// TestPlatformSelection proves which files each target compiles: the POSIX
// implementation exactly on (darwin && !ios) || (linux && !android), and
// the fail-closed fallback everywhere else (F008-FR-133–136, AC-019). It
// evaluates build constraints in process with go/build, because Feature 008
// tests may not start processes outside gitprocess.
func TestPlatformSelection(t *testing.T) {
	posix := []string{"filesystem_posix.go", "record_posix.go", "store_posix.go", "authorization.go", "audit.go", "lock_posix.go"}
	for _, tc := range []struct {
		goos, goarch string
		supported    bool
		osFile       string
	}{
		{"darwin", "arm64", true, "filesystem_darwin.go"},
		{"linux", "amd64", true, "filesystem_linux.go"},
		{"linux", "arm64", true, "filesystem_linux.go"},
		{"ios", "arm64", false, ""},
		{"android", "arm64", false, ""},
		{"windows", "amd64", false, ""},
		{"freebsd", "amd64", false, ""},
	} {
		t.Run(tc.goos+"/"+tc.goarch, func(t *testing.T) {
			context := build.Default
			context.GOOS, context.GOARCH, context.CgoEnabled = tc.goos, tc.goarch, false
			pkg, err := context.ImportDir(".", 0)
			if err != nil {
				t.Fatalf("import for %s/%s: %v", tc.goos, tc.goarch, err)
			}
			files := pkg.GoFiles
			for _, name := range posix {
				if contains(files, name) != tc.supported {
					t.Errorf("%s selected=%t, want %t (files %v)", name, contains(files, name), tc.supported, files)
				}
			}
			if contains(files, "unsupported_other.go") == tc.supported {
				t.Errorf("fallback selected=%t on a supported=%t target", !tc.supported, tc.supported)
			}
			if tc.osFile != "" && !contains(files, tc.osFile) {
				t.Errorf("%s not selected", tc.osFile)
			}
			for _, name := range []string{"filesystem_darwin.go", "filesystem_linux.go"} {
				if name != tc.osFile && contains(files, name) {
					t.Errorf("%s selected for %s", name, tc.goos)
				}
			}
			for _, name := range []string{"unsupported.go", "store.go", "record.go"} {
				if !contains(files, name) {
					t.Errorf("shared file %s not selected", name)
				}
			}
		})
	}
}

// The fallback implementations are compiled on every platform, so their
// fail-closed behavior is tested here even on a supported host.
func TestUnsupportedPlatformFailsClosed(t *testing.T) {
	var platformErr *PlatformError
	if _, err := unsupportedObserveFilesystem("/tmp"); !errors.As(err, &platformErr) || platformErr.Reason != gitmanager.ReasonPlatformUnsupported {
		t.Errorf("observe: %v", err)
	}
	if _, err := unsupportedProbePrimitives("/tmp"); !errors.As(err, &platformErr) || platformErr.Reason != gitmanager.ReasonPlatformUnsupported {
		t.Errorf("probe: %v", err)
	}
	if _, err := unsupportedOpen(gitmanager.NewObservedPath([]byte("/tmp/state"))); !errors.As(err, &platformErr) || platformErr.Reason != gitmanager.ReasonPlatformUnsupported {
		t.Errorf("open: %v", err)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
