package gitprocess

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"ai-dev/internal/gitmanager"
)

// macOSLauncher is the Xcode command-line launcher, whose effective Git
// depends on the developer directory.
const macOSLauncher = "/usr/bin/git"

// LocateGit finds the git executable the user's PATH selects, without
// running anything, and chooses its invocation mode: the macOS launcher is
// invoked as a launcher, any other executable directly. Identify then
// freezes and verifies that choice; nothing falls back to another Git.
func LocateGit(pathList string) (string, gitmanager.InvocationMode, error) {
	for _, dir := range filepath.SplitList(pathList) {
		if !filepath.IsAbs(dir) {
			// A relative PATH entry depends on the working directory.
			continue
		}
		candidate := filepath.Join(dir, "git")
		info, err := os.Stat(candidate)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			continue
		}
		mode := gitmanager.InvocationModeDirect
		if runtime.GOOS == "darwin" && candidate == macOSLauncher {
			mode = gitmanager.InvocationModeLauncher
		}
		return candidate, mode, nil
	}
	if strings.TrimSpace(pathList) == "" {
		return "", "", errors.New("PATH is empty")
	}
	return "", "", errors.New("no git executable on PATH")
}
