// Package nvim finds the Neovim that tests run against. It is imported only
// from test files (Feature 014, research R-08).
package nvim

import (
	"os"
	"os/exec"
	"regexp"
	"strconv"
)

// RequireEnv makes a missing or unsupported Neovim a test failure instead
// of a skip; CI sets it in the gates job.
const RequireEnv = "AI_DEV_REQUIRE_NVIM"

// Minimum is the oldest supported Neovim (research R-01).
var Minimum = [3]int{0, 11, 7}

// TB is the part of testing.TB that Require uses.
type TB interface {
	Helper()
	Skipf(format string, args ...any)
	Fatalf(format string, args ...any)
}

var versionLine = regexp.MustCompile(`^NVIM v(\d+)\.(\d+)\.(\d+)`)

// Require returns the path of nvim from PATH. When it is missing or older
// than Minimum, the test is skipped, or fails when RequireEnv is "1". It
// returns "" after a skip or failure reported to a TB that does not stop
// the test.
func Require(t TB) string {
	t.Helper()
	stop := t.Skipf
	if os.Getenv(RequireEnv) == "1" {
		stop = t.Fatalf
	}
	path, err := exec.LookPath("nvim")
	if err != nil {
		stop("nvim not found on PATH")
		return ""
	}
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		stop("nvim --version: %v", err)
		return ""
	}
	m := versionLine.FindSubmatch(out)
	if m == nil {
		stop("nvim --version: unrecognized output %q", firstLine(out))
		return ""
	}
	var got [3]int
	for i := range got {
		got[i], _ = strconv.Atoi(string(m[i+1]))
	}
	if less(got, Minimum) {
		stop("nvim %d.%d.%d is older than %d.%d.%d", got[0], got[1], got[2], Minimum[0], Minimum[1], Minimum[2])
		return ""
	}
	return path
}

func less(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

func firstLine(out []byte) string {
	for i, c := range out {
		if c == '\n' {
			return string(out[:i])
		}
	}
	return string(out)
}
