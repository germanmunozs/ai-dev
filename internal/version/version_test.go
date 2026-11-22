package version

import (
	"go/parser"
	"go/token"
	"runtime"
	"runtime/debug"
	"strconv"
	"testing"
)

func TestFromBuildInfo(t *testing.T) {
	platform := runtime.GOOS + "/" + runtime.GOARCH
	vcs := func(revision, time, modified string) []debug.BuildSetting {
		return []debug.BuildSetting{{Key: "vcs", Value: "git"}, {Key: "vcs.revision", Value: revision}, {Key: "vcs.time", Value: time}, {Key: "vcs.modified", Value: modified}}
	}
	cases := []struct {
		name     string
		linked   string
		info     *debug.BuildInfo
		ok       bool
		expected Info
	}{
		{"no build information", "", nil, false, Info{Version: "unknown"}},
		{"devel with VCS", "", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}, Settings: vcs("d4c71b5e0a6b", "2026-11-20T16:07:05Z", "true")}, true,
			Info{Version: "(devel)", Commit: "d4c71b5e0a6b", CommitTime: "2026-11-20T16:07:05Z", Modified: true}},
		{"pseudo-version, clean tree", "", &debug.BuildInfo{Main: debug.Module{Version: "v0.0.0-20261120160705-d4c71b5e0a6b"}, Settings: vcs("d4c71b5e0a6b", "2026-11-20T16:07:05Z", "false")}, true,
			Info{Version: "v0.0.0-20261120160705-d4c71b5e0a6b", Commit: "d4c71b5e0a6b", CommitTime: "2026-11-20T16:07:05Z"}},
		{"link-time version wins", "v0.1.0", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}, Settings: vcs("abc", "", "false")}, true,
			Info{Version: "v0.1.0", Commit: "abc"}},
		{"link-time version without build information", "v0.1.0", nil, false, Info{Version: "v0.1.0"}},
		{"empty module version", "", &debug.BuildInfo{}, true, Info{Version: "unknown"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := fromBuildInfo(tc.linked, tc.info, tc.ok)
			want := tc.expected
			want.GoVersion, want.Platform = runtime.Version(), platform
			if got != want {
				t.Fatalf("fromBuildInfo = %+v, want %+v", got, want)
			}
		})
	}
}

func TestReadUsesTheLinkedVersion(t *testing.T) {
	saved := Version
	t.Cleanup(func() { Version = saved })
	Version = "v9.9.9"
	if got := Read(); got.Version != "v9.9.9" || got.Platform == "" || got.GoVersion == "" {
		t.Fatalf("Read = %+v", got)
	}
}

// TestNoProcessFileOrNetworkAccess enforces F009-FR-001 structurally.
func TestNoProcessFileOrNetworkAccess(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "version.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{"runtime": true, "runtime/debug": true}
	for _, spec := range file.Imports {
		path, _ := strconv.Unquote(spec.Path.Value)
		if !allowed[path] {
			t.Errorf("version.go imports %q; only runtime and runtime/debug are allowed", path)
		}
	}
}
