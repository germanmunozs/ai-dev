//go:build darwin || linux

package editor

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// project builds a temporary project with files, a directory, a FIFO and
// links, and returns its resolved root and a sibling directory outside it.
func project(t *testing.T) (root, outside string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root = filepath.Join(base, "project")
	outside = filepath.Join(base, "outside")
	for _, dir := range []string{root, filepath.Join(root, "dir"), filepath.Join(root, "sub"), outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"a.txt", "dir/b.go", "with space.txt", "ünï-código.md", "-leading.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("s\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	links := map[string]string{
		"inside-link":  filepath.Join(root, "a.txt"),
		"outside-link": filepath.Join(outside, "secret.txt"),
		"dangling":     filepath.Join(root, "missing.txt"),
		"dir-link":     filepath.Join(root, "dir"),
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := syscall.Mkfifo(filepath.Join(root, "fifo"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, outside
}

func reason(err error) string {
	var pathErr *PathError
	if errors.As(err, &pathErr) {
		return pathErr.Reason
	}
	if err != nil {
		return "unexpected: " + err.Error()
	}
	return ""
}

func TestResolveAccepts(t *testing.T) {
	root, _ := project(t)
	cases := map[string]string{
		"a.txt":          "a.txt",
		"dir/b.go":       "dir/b.go",
		"./dir/../a.txt": "a.txt",
		"with space.txt": "with space.txt",
		"ünï-código.md":  "ünï-código.md",
		"-leading.txt":   "-leading.txt",
		"inside-link":    "a.txt",
	}
	for path, target := range cases {
		loc, err := Resolve(root, path, 0, 0)
		if err != nil {
			t.Errorf("Resolve(%q): %v", path, err)
			continue
		}
		if loc.Rel != path || loc.Abs != filepath.Join(root, target) || loc.Line != 1 || loc.Column != 1 {
			t.Errorf("Resolve(%q) = %+v", path, loc)
		}
	}
	loc, err := Resolve(root, "a.txt", 12, 3)
	if err != nil || loc.Line != 12 || loc.Column != 3 {
		t.Errorf("Resolve with position = %+v, %v", loc, err)
	}
}

// Every refusal names its reason, checked in the documented order
// (data-model.md Editor Location, F014-FR-002, SC-002).
func TestResolveRefuses(t *testing.T) {
	root, outside := project(t)
	cases := map[string]string{
		filepath.Join(root, "a.txt"):         "absolute_path",
		filepath.Join(outside, "secret.txt"): "absolute_path",
		"/etc/hosts":                         "absolute_path",
		"missing.txt":                        "not_found",
		"dangling":                           "not_found",
		"../outside/secret.txt":              "outside_project",
		"dir/../../outside/secret.txt":       "outside_project",
		"outside-link":                       "outside_project",
		"dir":                                "not_regular_file",
		"fifo":                               "not_regular_file",
		"dir-link":                           "not_regular_file",
		".":                                  "not_regular_file",
		"":                                   "not_found",
	}
	for path, want := range cases {
		_, err := Resolve(root, path, 1, 1)
		if got := reason(err); got != want {
			t.Errorf("Resolve(%q) reason %q, want %q", path, got, want)
		}
	}
}

func TestResolveThroughLinkedRoot(t *testing.T) {
	root, _ := project(t)
	link := filepath.Join(filepath.Dir(root), "project-link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	loc, err := Resolve(link, "dir/b.go", 1, 1)
	if err != nil || loc.Abs != filepath.Join(root, "dir", "b.go") {
		t.Fatalf("Resolve through a linked root = %+v, %v", loc, err)
	}
	if _, err := Resolve(link, "../outside/secret.txt", 1, 1); reason(err) != "outside_project" {
		t.Fatalf("escape through a linked root: %v", err)
	}
}
