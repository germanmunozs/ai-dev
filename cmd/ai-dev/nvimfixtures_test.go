package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The plugin's tests read ai-dev outputs saved in nvim/tests/fixtures. This
// test runs the binary built from this branch on the projects of
// projects.json and fails when a saved output or exit code differs, so the
// fixtures cannot drift from the real outputs (Feature 014, SC-003). With
// AI_DEV_UPDATE_FIXTURES=1 it rewrites them instead.

const fixturesDir = "../../nvim/tests/fixtures"

type pluginFixture struct {
	Name     string   `json:"name"`
	Project  string   `json:"project"`
	Args     []string `json:"args"`
	ExitCode *int     `json:"exit_code"`
}

// fixtureEnvironment is the controlled environment of
// nvim/tests/fixtures/README.md: PATH holds only fake git, specify and the
// fake claude and gemini, each a /bin/sh script that does nothing, and HOME
// and the XDG directories are private. Nothing of the person's environment
// is passed.
func fixtureEnvironment(t *testing.T) []string {
	t.Helper()
	bin := t.TempDir()
	for _, name := range []string{"git", "specify", "claude", "gemini"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	home := t.TempDir()
	return []string{
		"PATH=" + bin,
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, "config"),
		"XDG_DATA_HOME=" + filepath.Join(home, "data"),
		"XDG_STATE_HOME=" + filepath.Join(home, "state"),
	}
}

func makeFixtureProject(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestPluginFixturesMatchTheBinary(t *testing.T) {
	update := os.Getenv("AI_DEV_UPDATE_FIXTURES") == "1"
	var projects map[string]map[string]string
	readFixtureJSON(t, "projects.json", &projects)
	var index []pluginFixture
	readFixtureJSON(t, "index.json", &index)

	bin := buildWorkflowNextBinary(t)
	env := fixtureEnvironment(t)
	roots := map[string]string{}
	for name, files := range projects {
		roots[name] = makeFixtureProject(t, files)
	}
	for i, fixture := range index {
		root, ok := roots[fixture.Project]
		if !ok {
			t.Fatalf("%s: unknown project %q", fixture.Name, fixture.Project)
		}
		cmd := exec.Command(bin, fixture.Args...)
		cmd.Dir = root
		cmd.Env = env
		stdout, stderr, code := runCommand(cmd)
		if stderr != "" {
			t.Errorf("%s: unexpected standard error %q", fixture.Name, stderr)
		}
		path := filepath.Join(fixturesDir, fixture.Name+".json")
		if update {
			index[i].ExitCode = &code
			if err := os.WriteFile(path, []byte(stdout), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		saved, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v (regenerate with AI_DEV_UPDATE_FIXTURES=1)", fixture.Name, err)
		}
		if stdout != string(saved) {
			t.Errorf("%s: output differs from %s:\ngot  %s\nwant %s", fixture.Name, path, stdout, saved)
		}
		if fixture.ExitCode == nil || code != *fixture.ExitCode {
			t.Errorf("%s: exit code %d, index records %v", fixture.Name, code, fixture.ExitCode)
		}
	}
	if update {
		var out bytes.Buffer
		enc := json.NewEncoder(&out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(index); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(fixturesDir, "index.json"), out.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func readFixtureJSON(t *testing.T, name string, value any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixturesDir, name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, value); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}
