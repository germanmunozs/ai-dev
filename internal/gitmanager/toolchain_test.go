package gitmanager

import (
	"strings"
	"testing"
)

// TOOL-01: only verified installation layouts are known, each with the
// configuration and attribute files that toolchain compiles in.
func TestResolveToolchainLayout(t *testing.T) {
	for _, test := range []struct {
		execPath   string
		known      bool
		config     string
		attributes string
	}{
		{"/Library/Developer/CommandLineTools/usr/libexec/git-core", true, "/etc/gitconfig", "/Library/Developer/CommandLineTools/usr/share/git-core/gitattributes"},
		{"/Applications/Xcode.app/Contents/Developer/usr/libexec/git-core", true, "/etc/gitconfig", "/Applications/Xcode.app/Contents/Developer/usr/share/git-core/gitattributes"},
		{"/usr/libexec/git-core", true, "/etc/gitconfig", "/etc/gitattributes"},
		{"/usr/lib/git-core", true, "/etc/gitconfig", "/etc/gitattributes"},
		{"/usr/local/libexec/git-core", true, "/usr/local/etc/gitconfig", "/usr/local/etc/gitattributes"},
		{"/opt/homebrew/Cellar/git/2.55.0/libexec/git-core", true, "/opt/homebrew/etc/gitconfig", "/opt/homebrew/etc/gitattributes"},
		{"/opt/custom/lib/git-core", false, "", ""},
		{"/usr/lib/git-core/", false, "", ""},
	} {
		tool, err := NewGitToolIdentity(GitToolIdentityInput{
			InvocationMode: InvocationModeDirect, InvocationPath: "/usr/bin/git", InvocationPathDigest: testSHA256,
			InvocationDevice: "1", InvocationInode: "2", EffectiveExecutablePath: "/usr/bin/git", EffectiveExecutableDigest: testSHA256,
			Version: "2.55.0", ExecPath: test.execPath, Capabilities: []string{"init"}, EnvironmentSelectors: map[string]string{},
		})
		if err != nil {
			if !test.known {
				continue
			}
			t.Fatalf("%s: %v", test.execPath, err)
		}
		layout, known := resolveToolchainLayout(tool)
		if known != test.known {
			t.Fatalf("%s: known %v", test.execPath, known)
		}
		if !known {
			continue
		}
		config := string(layout.ConfigFiles[len(layout.ConfigFiles)-1])
		attributes := string(layout.AttributeFiles[0])
		if config != test.config || attributes != test.attributes {
			t.Fatalf("%s: config %q attributes %q", test.execPath, config, attributes)
		}
		for _, files := range [][][]byte{layout.ConfigFiles, layout.AttributeFiles} {
			for _, file := range files {
				if !strings.HasPrefix(string(file), "/") {
					t.Fatalf("%s: relative file %q", test.execPath, file)
				}
			}
		}
	}
}
