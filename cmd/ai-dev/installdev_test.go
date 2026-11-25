//go:build darwin || linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestInstallDevScript drives scripts/install-dev.sh end to end in
// temporary directories: install, sandbox with fake agents, uninstall, and
// the refusals that keep it from touching anything else.
func TestInstallDevScript(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(repoRoot, "scripts", "install-dev.sh")
	prefix := filepath.Join(t.TempDir(), "bin")
	sandbox := filepath.Join(t.TempDir(), "sandbox")
	run := func(dir string, env []string, name string, args ...string) (string, int) {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir, cmd.Env = dir, env
		output, err := cmd.CombinedOutput()
		if exit, ok := err.(*exec.ExitError); ok {
			return string(output), exit.ExitCode()
		}
		if err != nil {
			t.Fatalf("%s %q: %v", name, args, err)
		}
		return string(output), 0
	}

	output, code := run(repoRoot, os.Environ(), "bash", script, "--prefix", prefix)
	if code != 0 || !strings.Contains(output, "Installed "+filepath.Join(prefix, "ai-dev")) {
		t.Fatalf("install: exit %d\n%s", code, output)
	}
	bin := filepath.Join(prefix, "ai-dev")
	if output, code := run(repoRoot, os.Environ(), bin, "version"); code != 0 || !strings.Contains(output, "v0.3.1-dev+") {
		t.Fatalf("installed version: exit %d %q", code, output)
	}

	if output, code := run(repoRoot, os.Environ(), "bash", script, "--sandbox", sandbox); code != 0 || !strings.Contains(output, "Sandbox created") {
		t.Fatalf("sandbox: exit %d\n%s", code, output)
	}
	if _, code := run(repoRoot, os.Environ(), "bash", script, "--sandbox", sandbox); code == 0 {
		t.Fatal("an existing sandbox directory was reused")
	}
	env := []string{"HOME=" + t.TempDir(), "PATH=" + filepath.Join(sandbox, "fake-agents") + ":/usr/bin:/bin"}
	output, code = run(filepath.Join(sandbox, "project"), env, bin, "agent", "run", "--", "claude-code", `-x "a b"`)
	if code != 0 || !strings.Contains(output, "fake claude received:\n  [--print]\n  [--]\n  [-x \"a b\"]") {
		t.Fatalf("fake agent in the sandbox: exit %d\n%s", code, output)
	}

	if output, code := run(repoRoot, os.Environ(), "bash", script, "--uninstall", "--prefix", prefix); code != 0 || !strings.Contains(output, "Removed") {
		t.Fatalf("uninstall: exit %d\n%s", code, output)
	}
	if _, err := os.Stat(bin); !os.IsNotExist(err) {
		t.Fatalf("binary still present: %v", err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, code := run(repoRoot, os.Environ(), "bash", script, "--uninstall", "--prefix", prefix); code == 0 {
		t.Fatal("uninstall removed a file that is not ai-dev")
	}
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("the foreign file was removed: %v", err)
	}
}
