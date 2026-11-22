package gitstore

import (
	"go/build"
	"testing"
)

// T114/F008-FR-133–136/AC-019: every Feature 008 package selects exactly
// its platform files for macOS, Linux, Windows, iOS and Android, so each
// supported target gets the POSIX descriptor, lock and identity code and
// every other target gets only the fail-closed fallback. Selection is
// checked with go/build; no unsupported binary is built or run.
func TestFeature008PlatformBuildSelection(t *testing.T) {
	posix := map[string][]string{
		"../gitmanager": {"allowlist_posix.go", "index_posix.go", "inspect_posix.go", "path_posix.go", "planoutput_posix.go",
			"projectconfig_posix.go", "source_posix.go"},
	}
	fallback := map[string][]string{"../gitmanager": {"path_other.go", "index_other.go"}, "../gitprocess": {"identity_other.go"}}
	perOS := map[string]map[string]string{
		"darwin": {"../gitmanager": "path_darwin.go", "../gitprocess": "identity_darwin.go"},
		"linux":  {"../gitmanager": "path_linux.go", "../gitprocess": "identity_linux.go"},
	}
	for _, target := range []struct {
		goos, goarch string
		supported    bool
	}{
		{"darwin", "arm64", true}, {"darwin", "amd64", true}, {"linux", "amd64", true}, {"linux", "arm64", true},
		{"windows", "amd64", false}, {"ios", "arm64", false}, {"android", "arm64", false},
	} {
		t.Run(target.goos+"/"+target.goarch, func(t *testing.T) {
			context := build.Default
			context.GOOS, context.GOARCH, context.CgoEnabled = target.goos, target.goarch, false
			for _, dir := range []string{"../gitmanager", "../gitprocess", "../conventionalcommit", "."} {
				pkg, err := context.ImportDir(dir, 0)
				if err != nil {
					t.Fatalf("%s: %v", dir, err)
				}
				selected := map[string]bool{}
				for _, name := range pkg.GoFiles {
					selected[name] = true
				}
				for _, name := range posix[dir] {
					if selected[name] != target.supported {
						t.Errorf("%s/%s selected=%t on a supported=%t target", dir, name, selected[name], target.supported)
					}
				}
				for _, name := range fallback[dir] {
					if selected[name] == target.supported {
						t.Errorf("fallback %s/%s selected=%t on a supported=%t target", dir, name, selected[name], target.supported)
					}
				}
				for goos, files := range perOS {
					if name, ok := files[dir]; ok && selected[name] != (goos == target.goos) {
						t.Errorf("%s/%s selected=%t for %s", dir, name, selected[name], target.goos)
					}
				}
			}
		})
	}
}
