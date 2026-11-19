package gitmanager

import "strings"

// toolchainLayout is where a frozen toolchain reads its own configuration
// and attributes, lowest precedence first. Git compiles these paths in and
// does not report them, so only verified installation layouts are known;
// any other layout cannot be evaluated exactly (TOOL-01).
type toolchainLayout struct {
	ConfigFiles    [][]byte
	AttributeFiles [][]byte
	// MustBeAbsent are files whose use by the toolchain is unverified: if
	// one exists, the evaluation cannot be proven equivalent.
	MustBeAbsent [][]byte
}

// resolveToolchainLayout derives the layout from the frozen exec path
// <prefix>/libexec/git-core.
func resolveToolchainLayout(tool GitToolIdentity) (toolchainLayout, bool) {
	if tool.ExecPath == "/usr/lib/git-core" {
		// Debian and Ubuntu: prefix /usr with the exec path in lib, and
		// sysconfdir /etc, as the /usr layout below.
		return toolchainLayout{
			ConfigFiles:    [][]byte{[]byte("/etc/gitconfig")},
			AttributeFiles: [][]byte{[]byte("/etc/gitattributes")},
		}, true
	}
	prefix, ok := strings.CutSuffix(tool.ExecPath, "/libexec/git-core")
	if !ok || prefix == "" || !strings.HasPrefix(prefix, "/") {
		return toolchainLayout{}, false
	}
	switch {
	case strings.HasSuffix(prefix, "/Contents/Developer/usr") || strings.HasSuffix(prefix, "/CommandLineTools/usr"):
		// Apple Git reads its bundled configuration and attributes, then
		// /etc/gitconfig (verified against Apple Git 2.54).
		return toolchainLayout{
			ConfigFiles:    [][]byte{[]byte(prefix + "/share/git-core/gitconfig"), []byte("/etc/gitconfig")},
			AttributeFiles: [][]byte{[]byte(prefix + "/share/git-core/gitattributes")},
			MustBeAbsent:   [][]byte{[]byte("/etc/gitattributes")},
		}, true
	case prefix == "/usr":
		return toolchainLayout{
			ConfigFiles:    [][]byte{[]byte("/etc/gitconfig")},
			AttributeFiles: [][]byte{[]byte("/etc/gitattributes")},
		}, true
	case prefix == "/usr/local":
		return toolchainLayout{
			ConfigFiles:    [][]byte{[]byte("/usr/local/etc/gitconfig")},
			AttributeFiles: [][]byte{[]byte("/usr/local/etc/gitattributes")},
		}, true
	}
	base, _, homebrew := strings.Cut(prefix, "/Cellar/git/")
	if !homebrew {
		// Homebrew's stable path <homebrew prefix>/opt/git, which its Git
		// reports as the exec path's prefix.
		base, homebrew = strings.CutSuffix(prefix, "/opt/git")
	}
	if homebrew && base != "" {
		// Homebrew configures sysconfdir as <homebrew prefix>/etc.
		return toolchainLayout{
			ConfigFiles:    [][]byte{[]byte(base + "/etc/gitconfig")},
			AttributeFiles: [][]byte{[]byte(base + "/etc/gitattributes")},
		}, true
	}
	return toolchainLayout{}, false
}
