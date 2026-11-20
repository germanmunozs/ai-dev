// Package version reports which build of ai-dev is running (Feature 009).
// It reads only the build information Go embeds in the binary: it starts no
// process and reads no file, environment value or network resource.
package version

import (
	"runtime"
	"runtime/debug"
)

// Version is the release version, set at link time with
// -ldflags "-X ai-dev/internal/version.Version=vX.Y.Z". When empty, the
// main module version Go stamped at build time is reported instead.
var Version = ""

// Info is the build information of the running binary. Commit, CommitTime
// and Modified are empty when the build embedded no VCS information.
type Info struct {
	Version    string
	Commit     string
	CommitTime string
	Modified   bool
	GoVersion  string
	Platform   string
}

// Read returns the running binary's build information.
func Read() Info {
	info, ok := debug.ReadBuildInfo()
	return fromBuildInfo(Version, info, ok)
}

func fromBuildInfo(linked string, info *debug.BuildInfo, ok bool) Info {
	result := Info{Version: linked, GoVersion: runtime.Version(), Platform: runtime.GOOS + "/" + runtime.GOARCH}
	if ok && info != nil {
		if result.Version == "" {
			result.Version = info.Main.Version
		}
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				result.Commit = setting.Value
			case "vcs.time":
				result.CommitTime = setting.Value
			case "vcs.modified":
				result.Modified = setting.Value == "true"
			}
		}
	}
	if result.Version == "" {
		result.Version = "unknown"
	}
	return result
}
