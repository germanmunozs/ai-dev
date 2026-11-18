//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"testing"
)

// physicalTempDir is a private temporary directory named by its physical
// path, so expectations do not depend on platform symlinks such as /tmp.
func physicalTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func statIdentity(t *testing.T, path string) (string, string) {
	t.Helper()
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		t.Fatal(err)
	}
	return strconv.FormatUint(deviceNumber(&st), 10), strconv.FormatUint(uint64(st.Ino), 10)
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	mustMkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeGitDir lays out the minimal files that identify a Git directory.
func fakeGitDir(t *testing.T, dir string) {
	t.Helper()
	mustWrite(t, filepath.Join(dir, "HEAD"), "ref: refs/heads/main\n")
	mustMkdir(t, filepath.Join(dir, "objects"))
	mustMkdir(t, filepath.Join(dir, "refs"))
}

func expectReason(t *testing.T, err error, reason Reason) {
	t.Helper()
	var inspectErr *InspectError
	if !errors.As(err, &inspectErr) || inspectErr.Reason != reason {
		t.Fatalf("got %v, want %s", err, reason)
	}
}

func TestResolveProjectRootIdentity(t *testing.T) {
	parent := physicalTempDir(t)
	project := filepath.Join(parent, "project")
	mustMkdir(t, project)
	root, err := ResolveProjectRoot("project", parent)
	if err != nil {
		t.Fatal(err)
	}
	device, inode := statIdentity(t, project)
	identity := root.Identity
	if string(identity.ReversiblePathBytes()) != project || identity.DeviceDecimal() != device || identity.InodeDecimal() != inode ||
		identity.Digest() != ComputeProjectIdentityDigest([]byte(project), device, inode) || root.Aliased || !root.OwnedByCurrentUser {
		t.Fatalf("root %+v", root)
	}
	// The identity ignores content and timestamps.
	mustWrite(t, filepath.Join(project, "file"), "x")
	again, err := ResolveProjectRoot(project, "/")
	if err != nil || again.Identity.Digest() != identity.Digest() {
		t.Fatalf("identity changed with content: %v", err)
	}
}

// A symlinked project path or prefix resolves to the physical directory and
// is reported as an alias; the identity is the physical one.
func TestResolveProjectRootSymlinks(t *testing.T) {
	parent := physicalTempDir(t)
	project := filepath.Join(parent, "real")
	mustMkdir(t, project)
	link := filepath.Join(physicalTempDir(t), "link")
	if err := os.Symlink(project, link); err != nil {
		t.Fatal(err)
	}
	for _, requested := range []string{link, filepath.Join(link, "."), filepath.Join(parent, "real", "..", "real")} {
		root, err := ResolveProjectRoot(requested, "/")
		if err != nil {
			t.Fatalf("%s: %v", requested, err)
		}
		if string(root.Identity.ReversiblePathBytes()) != project {
			t.Fatalf("%s resolved to %s", requested, root.Identity.ReversiblePathBytes())
		}
		if requested == link && !root.Aliased {
			t.Fatal("symlinked project not reported as an alias")
		}
	}
}

func TestResolveProjectRootRefusals(t *testing.T) {
	parent := physicalTempDir(t)
	mustWrite(t, filepath.Join(parent, "file"), "x")
	dangling := filepath.Join(parent, "dangling")
	if err := os.Symlink(filepath.Join(parent, "missing"), dangling); err != nil {
		t.Fatal(err)
	}
	for name, requested := range map[string]string{
		"missing":   filepath.Join(parent, "missing"),
		"file":      filepath.Join(parent, "file"),
		"dangling":  dangling,
		"empty":     "",
		"nul":       parent + "\x00x",
		"overlong":  parent + "/" + string(bytes.Repeat([]byte("a"), 5000)),
		"long name": filepath.Join(parent, string(bytes.Repeat([]byte("b"), 300))),
	} {
		_, err := ResolveProjectRoot(requested, "/")
		expectReason(t, err, ReasonProjectIdentityUnavailable)
		_ = name
	}
	if _, err := ResolveProjectRoot("relative", "not-absolute"); err == nil {
		t.Fatal("relative working directory accepted")
	}
}

// Identity components keep their decimal string form at every magnitude.
func TestProjectIdentityLargeInode(t *testing.T) {
	for _, inode := range []string{"9007199254740993", "18446744073709551615"} {
		identity, err := NewPhysicalProjectIdentity("/p", []byte("/p"), "1", inode)
		if err != nil {
			t.Fatal(err)
		}
		e := &canonicalEncoder{}
		data, err := MarshalCanonical(e.projectIdentity(identity))
		if err != nil || !bytes.Contains(data, []byte(`"inode_decimal":"`+inode+`"`)) {
			t.Fatalf("%s encoded as %s %v", inode, data, err)
		}
	}
}

func TestNonUTF8ProjectPathIsReversible(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("APFS refuses names that are not valid UTF-8")
	}
	parent := physicalTempDir(t)
	project := filepath.Join(parent, "caf\xe9")
	mustMkdir(t, project)
	root, err := ResolveProjectRoot(project, "/")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(root.Identity.ReversiblePathBytes(), []byte(project)) || root.Identity.PhysicalPathUTF8() != "" {
		t.Fatalf("non-UTF-8 identity %q %q", root.Identity.ReversiblePathBytes(), root.Identity.PhysicalPathUTF8())
	}
}

func TestOwnershipIncompatibility(t *testing.T) {
	if got := ownershipIncompatibilities(1000, 1000, 1000, true); len(got) != 0 {
		t.Fatalf("owned project reported %v", got)
	}
	for _, test := range []struct {
		root, gitDir uint32
		hasGitDir    bool
	}{{0, 1000, true}, {1000, 0, true}, {0, 0, false}} {
		got := ownershipIncompatibilities(test.root, test.gitDir, 1000, test.hasGitDir)
		if len(got) == 0 || got[0].Reason != ReasonDubiousOwnership {
			t.Fatalf("%+v: %v", test, got)
		}
	}
}

func TestLocateRepositoryKinds(t *testing.T) {
	type fixture func(t *testing.T, base string) string
	for name, test := range map[string]struct {
		build fixture
		kind  RepositoryKind
	}{
		"absent": {func(t *testing.T, base string) string {
			p := filepath.Join(base, "p")
			mustMkdir(t, p)
			return p
		}, RepositoryKindAbsent},
		"normal": {func(t *testing.T, base string) string {
			p := filepath.Join(base, "p")
			fakeGitDir(t, filepath.Join(p, ".git"))
			return p
		}, RepositoryKindNormal},
		"bare": {func(t *testing.T, base string) string {
			p := filepath.Join(base, "p.git")
			fakeGitDir(t, p)
			return p
		}, RepositoryKindBare},
		"ancestor": {func(t *testing.T, base string) string {
			fakeGitDir(t, filepath.Join(base, "outer", ".git"))
			p := filepath.Join(base, "outer", "sub", "p")
			mustMkdir(t, p)
			return p
		}, RepositoryKindAncestor},
		"nested": {func(t *testing.T, base string) string {
			fakeGitDir(t, filepath.Join(base, "outer", ".git"))
			p := filepath.Join(base, "outer", "p")
			fakeGitDir(t, filepath.Join(p, ".git"))
			return p
		}, RepositoryKindNested},
		"linked worktree": {func(t *testing.T, base string) string {
			main := filepath.Join(base, "main", ".git")
			fakeGitDir(t, main)
			admin := filepath.Join(main, "worktrees", "wt")
			mustWrite(t, filepath.Join(admin, "HEAD"), "ref: refs/heads/wt\n")
			mustWrite(t, filepath.Join(admin, "commondir"), "../..\n")
			p := filepath.Join(base, "wt")
			mustWrite(t, filepath.Join(p, ".git"), "gitdir: "+admin+"\n")
			mustWrite(t, filepath.Join(admin, "gitdir"), filepath.Join(p, ".git")+"\n")
			return p
		}, RepositoryKindLinkedWorktree},
		"submodule": {func(t *testing.T, base string) string {
			super := filepath.Join(base, "super")
			fakeGitDir(t, filepath.Join(super, ".git"))
			module := filepath.Join(super, ".git", "modules", "lib")
			fakeGitDir(t, module)
			p := filepath.Join(super, "lib")
			mustWrite(t, filepath.Join(p, ".git"), "gitdir: ../.git/modules/lib\n")
			return p
		}, RepositoryKindSubmodule},
		"gitfile to an unknown layout": {func(t *testing.T, base string) string {
			separate := filepath.Join(base, "separate")
			fakeGitDir(t, separate)
			p := filepath.Join(base, "p")
			mustWrite(t, filepath.Join(p, ".git"), "gitdir: "+separate+"\n")
			return p
		}, RepositoryKindAmbiguous},
		"malformed gitfile": {func(t *testing.T, base string) string {
			p := filepath.Join(base, "p")
			mustWrite(t, filepath.Join(p, ".git"), "not a gitfile\n")
			return p
		}, RepositoryKindAmbiguous},
		"symlinked .git": {func(t *testing.T, base string) string {
			fakeGitDir(t, filepath.Join(base, "elsewhere"))
			p := filepath.Join(base, "p")
			mustMkdir(t, p)
			if err := os.Symlink(filepath.Join(base, "elsewhere"), filepath.Join(p, ".git")); err != nil {
				t.Fatal(err)
			}
			return p
		}, RepositoryKindAmbiguous},
		"incomplete .git": {func(t *testing.T, base string) string {
			p := filepath.Join(base, "p")
			mustMkdir(t, filepath.Join(p, ".git"))
			return p
		}, RepositoryKindAmbiguous},
	} {
		t.Run(name, func(t *testing.T) {
			project := test.build(t, physicalTempDir(t))
			location := locateRepository([]byte(project))
			if location.Kind != test.kind {
				t.Fatalf("kind %s (%s), want %s", location.Kind, location.Detail, test.kind)
			}
			switch test.kind {
			case RepositoryKindNormal, RepositoryKindNested:
				if string(location.GitDir) != filepath.Join(project, ".git") || !bytes.Equal(location.GitDir, location.CommonDir) {
					t.Fatalf("location %+v", location)
				}
			case RepositoryKindBare:
				if string(location.GitDir) != project {
					t.Fatalf("bare git dir %s", location.GitDir)
				}
			case RepositoryKindLinkedWorktree:
				if bytes.Equal(location.GitDir, location.CommonDir) || filepath.Base(string(location.CommonDir)) != ".git" {
					t.Fatalf("worktree location %+v", location)
				}
			case RepositoryKindAncestor:
				if filepath.Base(filepath.Dir(string(location.GitDir))) != "outer" {
					t.Fatalf("ancestor git dir %s", location.GitDir)
				}
			}
		})
	}
}

func TestLocateRepositoryInaccessible(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	project := filepath.Join(physicalTempDir(t), "p")
	fakeGitDir(t, filepath.Join(project, ".git"))
	if err := os.Chmod(filepath.Join(project, ".git"), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(filepath.Join(project, ".git"), 0o755) })
	if location := locateRepository([]byte(project)); location.Kind != RepositoryKindInaccessible {
		t.Fatalf("kind %s (%s)", location.Kind, location.Detail)
	}
}

// Path equivalence is proven only by exact bytes; where case folding or
// Unicode normalization may apply to non-ASCII names, nothing is assumed.
func TestPathEquivalence(t *testing.T) {
	nfc, nfd := []byte("café.txt"), []byte("café.txt")
	for name, test := range map[string]struct {
		paths                  [][]byte
		ignoreCase, precompose bool
		want                   Reason
	}{
		"distinct ASCII":                     {[][]byte{[]byte("a.txt"), []byte("b.txt")}, true, true, ""},
		"exact duplicate":                    {[][]byte{[]byte("a.txt"), []byte("a.txt")}, false, false, ReasonPathCollision},
		"ASCII case fold":                    {[][]byte{[]byte("README"), []byte("readme")}, true, false, ReasonPathCollision},
		"ASCII case on case-sensitive":       {[][]byte{[]byte("README"), []byte("readme")}, false, false, ""},
		"NFC and NFD on exact filesystem":    {[][]byte{nfc, nfd}, false, false, ""},
		"NFC and NFD with precompose":        {[][]byte{nfc, nfd}, false, true, ReasonPathNormalizationIncompatible},
		"single non-ASCII with case folding": {[][]byte{nfc}, true, false, ReasonPathNormalizationIncompatible},
		"directory case fold":                {[][]byte{[]byte("Docs/a"), []byte("docs/b")}, true, false, ReasonPathCollision},
	} {
		got := pathEquivalenceIncompatibility(test.paths, test.ignoreCase, test.precompose)
		if got != test.want {
			t.Errorf("%s: %q, want %q", name, got, test.want)
		}
	}
}
