//go:build darwin || linux

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/editor"
	"github.com/germanmunozs/ai-dev/internal/system"
)

type openLauncher struct {
	specs     []system.Spec
	status    system.ExitStatus
	launchErr error
}

func (l *openLauncher) Launch(_ context.Context, spec system.Spec) (system.Handle, error) {
	l.specs = append(l.specs, spec)
	if l.launchErr != nil {
		return nil, l.launchErr
	}
	return appRunHandle{status: l.status}, nil
}

// openFixture is a project and the fakes of one ai-dev open run.
type openFixture struct {
	root      string
	env       map[string]string
	terminals bool
	nvimPath  string
	endpoint  error
	parent    error
	launcher  openLauncher
	lookups   int
	checked   []string
	opened    []editor.Location
}

func newOpenFixture(t *testing.T) *openFixture {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(project, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"src/main.go", "-x b.txt", "esc\x1b[31m.txt"} {
		if err := os.WriteFile(filepath.Join(project, name), []byte("package main\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "outside.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return &openFixture{root: project, env: map[string]string{"HOME": root}, nvimPath: "/fake/bin/nvim",
		launcher: openLauncher{status: system.ExitStatus{Exited: true}}}
}

func (f *openFixture) deps() openDependencies {
	return openDependencies{
		Getenv: func(key string) string { return f.env[key] },
		Getwd:  func() (string, error) { return f.root, nil },
		LookPath: func(name string) (string, error) {
			f.lookups++
			if name == "nvim" && f.nvimPath != "" {
				return f.nvimPath, nil
			}
			return "", errors.New("not found")
		},
		CheckEndpoint: func(addr string) error { f.checked = append(f.checked, addr); return f.endpoint },
		OpenParent: func(_ string, loc editor.Location) error {
			f.opened = append(f.opened, loc)
			return f.parent
		},
		Terminals: f.terminals,
		Launcher:  &f.launcher,
	}
}

func (f *openFixture) run(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := RunOpen(context.Background(), args, f.deps(), strings.NewReader(""), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// The sensitive values no report may contain (F014-FR-025).
func (f *openFixture) assertNoSecrets(t *testing.T, outputs ...string) {
	t.Helper()
	for _, out := range outputs {
		for _, secret := range []string{f.root, f.env["HOME"], f.env["NVIM"]} {
			if secret != "" && strings.Contains(out, secret) {
				t.Errorf("output %q contains %q", out, secret)
			}
		}
	}
}

func TestOpenUsageFailures(t *testing.T) {
	f := newOpenFixture(t)
	for _, args := range [][]string{
		nil, {"--"}, {"--", "a", "b"}, {"README.md"}, {"-x"}, {"--line", "3", "src/main.go"},
		{"--line", "0", "--", "src/main.go"}, {"--line", "-1", "--", "src/main.go"}, {"--line", "x", "--", "src/main.go"},
		{"--column", "0", "--", "src/main.go"}, {"--zz", "--", "src/main.go"}, {"--format", "yaml", "--", "src/main.go"},
	} {
		code, stdout, stderr := f.run(args...)
		if code != 2 || stdout != "" || stderr != openUsage+"\n" {
			t.Errorf("%q: exit %d, stdout %q, stderr %q", args, code, stdout, stderr)
		}
	}
	if len(f.launcher.specs) != 0 || len(f.opened) != 0 || len(f.checked) != 0 {
		t.Fatal("a usage failure reached the editor")
	}
}

type openReport struct {
	SchemaVersion string `json:"schema_version"`
	Outcome       string `json:"outcome"`
	Reason        string `json:"reason"`
	Target        string `json:"target"`
	Path          string `json:"path"`
	Line          int    `json:"line"`
	Column        int    `json:"column"`
	ExitCode      *int   `json:"exit_code"`
	Message       string `json:"message"`
}

func decodeOpen(t *testing.T, stdout string) openReport {
	t.Helper()
	var r openReport
	if err := json.Unmarshal([]byte(stdout), &r); err != nil || r.SchemaVersion != "ai-dev.open/v1" {
		t.Fatalf("JSON report %q: %v", stdout, err)
	}
	return r
}

func TestOpenPathRefused(t *testing.T) {
	f := newOpenFixture(t)
	f.env["NVIM"] = "/run/nvim.sock"
	cases := map[string]string{
		filepath.Join(f.root, "src/main.go"): "absolute_path",
		"missing.go":                         "not_found",
		"../outside.txt":                     "outside_project",
		"src":                                "not_regular_file",
	}
	for path, reason := range cases {
		code, stdout, stderr := f.run("--", path)
		if code != 1 || stdout != "" || !strings.HasPrefix(stderr, "Refused: ") || !strings.HasSuffix(stderr, " ("+reason+").\n") ||
			strings.Contains(stderr, f.root) {
			t.Errorf("%q: exit %d, stdout %q, stderr %q", path, code, stdout, stderr)
		}
		code, stdout, stderr = f.run("--format", "json", "--", path)
		r := decodeOpen(t, stdout)
		want := path
		if reason == "absolute_path" {
			want = "" // never echoed (F014-FR-025)
		}
		if code != 1 || stderr != "" || r.Outcome != "path_refused" || r.Reason != reason || r.Path != want || r.Target != "" {
			t.Errorf("%q JSON: exit %d, %+v, stderr %q", path, code, r, stderr)
		}
	}
	if len(f.checked) != 0 || len(f.opened) != 0 || len(f.launcher.specs) != 0 {
		t.Fatal("a refused path reached the editor")
	}
	code, stdout, stderr := f.run("--", "outside_project_but_named_\x1b[31m")
	if code != 1 || stdout != "" || strings.Contains(stderr, "\x1b") || !strings.Contains(stderr, `\x1b[31m`) {
		t.Errorf("escape in a path: stderr %q", stderr)
	}
}

func TestOpenInParentOutcomes(t *testing.T) {
	f := newOpenFixture(t)
	f.env["NVIM"] = "/run/user/nvim.sock"
	code, stdout, stderr := f.run("--line", "12", "--", "src/main.go")
	if code != 0 || stderr != "" || stdout != "Opened src/main.go at line 12, column 1 in the parent Neovim.\n" {
		t.Fatalf("parent: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if !slices.Equal(f.checked, []string{"/run/user/nvim.sock"}) || len(f.opened) != 1 ||
		f.opened[0].Abs != filepath.Join(f.root, "src/main.go") || f.opened[0].Line != 12 || f.opened[0].Column != 1 {
		t.Fatalf("checked %v, opened %+v", f.checked, f.opened)
	}
	code, stdout, _ = f.run("--format", "json", "--column", "4", "--", "-x b.txt")
	r := decodeOpen(t, stdout)
	if code != 0 || r.Outcome != "opened" || r.Target != "parent" || r.Path != "-x b.txt" || r.Line != 1 || r.Column != 4 {
		t.Fatalf("parent JSON: exit %d, %+v", code, r)
	}
	f.assertNoSecrets(t, stdout)

	for _, reason := range []string{"not_local_socket", "not_socket", "not_owned"} {
		f.endpoint = &editor.EndpointError{Reason: reason}
		f.opened = nil
		code, stdout, stderr := f.run("--", "src/main.go")
		if code != 1 || stdout != "" || !strings.HasSuffix(stderr, "("+reason+"). Nothing was opened.\n") || len(f.opened) != 0 {
			t.Errorf("%s: exit %d, stdout %q, stderr %q", reason, code, stdout, stderr)
		}
		f.assertNoSecrets(t, stderr)
	}
	f.endpoint = nil
	for _, reason := range []string{"no_answer", "not_neovim", "version_unsupported"} {
		f.parent = &editor.EndpointError{Reason: reason}
		code, stdout, stderr := f.run("--format", "json", "--", "src/main.go")
		r := decodeOpen(t, stdout)
		if code != 1 || stderr != "" || r.Outcome != "editor_refused" || r.Reason != reason || r.Message != "" {
			t.Errorf("%s: exit %d, %+v", reason, code, r)
		}
		f.assertNoSecrets(t, stdout)
	}
	f.parent = &editor.EndpointError{Reason: "editor_error", Message: "E37: " + filepath.Join(f.root, "src/main.go") + " \x1b[31m"}
	code, stdout, _ = f.run("--format", "json", "--", "src/main.go")
	r = decodeOpen(t, stdout)
	if code != 1 || r.Outcome != "editor_refused" || r.Reason != "editor_error" || r.Message != `E37: src/main.go \x1b[31m` {
		t.Errorf("editor_error: exit %d, %+v", code, r)
	}
	f.assertNoSecrets(t, stdout)
	code, _, stderr = f.run("--", "src/main.go")
	if code != 1 || !strings.Contains(stderr, `E37: src/main.go \x1b[31m`) || strings.Contains(stderr, "\x1b") {
		t.Errorf("editor_error text: %q", stderr)
	}
	// No fallback: a refused $NVIM never starts another editor
	// (F014-FR-003).
	if len(f.launcher.specs) != 0 || f.lookups != 0 {
		t.Fatalf("fell back: %d launches, %d lookups", len(f.launcher.specs), f.lookups)
	}
}

func TestOpenInForeground(t *testing.T) {
	f := newOpenFixture(t)
	code, stdout, stderr := f.run("--", "src/main.go")
	if code != 1 || stdout != "" || !strings.HasSuffix(stderr, "(no_terminal). Nothing was opened.\n") || f.lookups != 0 {
		t.Fatalf("no terminal: exit %d, stderr %q", code, stderr)
	}
	f.terminals = true
	f.nvimPath = ""
	code, stdout, _ = f.run("--format", "json", "--", "src/main.go")
	if r := decodeOpen(t, stdout); code != 1 || r.Outcome != "editor_unavailable" || r.Reason != "nvim_not_found" {
		t.Fatalf("no nvim: exit %d, %+v", code, r)
	}
	f.nvimPath = "/fake/bin/nvim"
	code, stdout, stderr = f.run("--line", "7", "--column", "2", "--", "-x b.txt")
	if code != 0 || stdout != "" || stderr != "" || len(f.launcher.specs) != 1 {
		t.Fatalf("foreground: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	spec := f.launcher.specs[0]
	want := []string{"+call cursor(7, 2)", "--", filepath.Join(f.root, "-x b.txt")}
	if spec.Name != "/fake/bin/nvim" || !slices.Equal(spec.Args, want) || spec.Stdin == nil || spec.Stdout == nil || spec.Stderr == nil {
		t.Fatalf("launch spec %+v", spec)
	}
	code, stdout, _ = f.run("--format", "json", "--", "src/main.go")
	r := decodeOpen(t, stdout)
	if code != 0 || r.Outcome != "opened" || r.Target != "foreground" || r.ExitCode != nil {
		t.Fatalf("foreground JSON: exit %d, %+v", code, r)
	}
	f.assertNoSecrets(t, stdout)
	f.launcher.status = system.ExitStatus{Exited: true, ExitCode: 3}
	code, stdout, stderr = f.run("--", "src/main.go")
	if code != 1 || stdout != "" || stderr != "Neovim exited with code 3 (editor_failed).\n" {
		t.Fatalf("editor_failed: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	code, stdout, _ = f.run("--format", "json", "--", "src/main.go")
	r = decodeOpen(t, stdout)
	if code != 1 || r.Outcome != "editor_failed" || r.Target != "foreground" || r.ExitCode == nil || *r.ExitCode != 3 {
		t.Fatalf("editor_failed JSON: exit %d, %+v", code, r)
	}
	f.launcher.launchErr = errors.New("exec format error")
	code, stdout, _ = f.run("--format", "json", "--", "src/main.go")
	r = decodeOpen(t, stdout)
	if code != 1 || r.Outcome != "editor_failed" || r.ExitCode != nil {
		t.Fatalf("launch failure JSON: exit %d, %+v", code, r)
	}
	if len(f.checked) != 0 || len(f.opened) != 0 {
		t.Fatal("the foreground path used the parent instance")
	}
}
