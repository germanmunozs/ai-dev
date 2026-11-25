package nvim

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type recorder struct{ skipped, failed string }

func (r *recorder) Helper()                        {}
func (r *recorder) Skipf(format string, a ...any)  { r.skipped = fmt.Sprintf(format, a...) }
func (r *recorder) Fatalf(format string, a ...any) { r.failed = fmt.Sprintf(format, a...) }

// fakeNvim puts on a private PATH an nvim that prints version, or no nvim
// when version is empty.
func fakeNvim(t *testing.T, version string) string {
	dir := t.TempDir()
	path := filepath.Join(dir, "nvim")
	if version != "" {
		script := "#!/bin/sh\necho 'NVIM " + version + "'\necho 'Build type: Release'\n"
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	return path
}

func TestRequire(t *testing.T) {
	cases := []struct {
		name, version, require string
		wantPath               bool
		wantSkip, wantFail     bool
	}{
		{"absent", "", "", false, true, false},
		{"too old", "v0.11.6", "", false, true, false},
		{"minimum", "v0.11.7", "", true, false, false},
		{"newer", "v0.12.6", "", true, false, false},
		{"absent and required", "", "1", false, false, true},
		{"too old and required", "v0.10.4", "1", false, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := fakeNvim(t, c.version)
			t.Setenv(RequireEnv, c.require)
			var r recorder
			got := Require(&r)
			if (got == path) != c.wantPath || (r.skipped != "") != c.wantSkip || (r.failed != "") != c.wantFail {
				t.Fatalf("Require = %q, skipped %q, failed %q", got, r.skipped, r.failed)
			}
		})
	}
}
