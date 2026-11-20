package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestVersionAndHelpBinary(t *testing.T) {
	bin := buildWorkflowNextBinary(t)

	stdout, stderr, code := runCommand(exec.Command(bin, "--version"))
	if code != 0 || stderr != "" || !strings.HasPrefix(stdout, "ai-dev ") || !strings.Contains(stdout, runtime.GOOS+"/"+runtime.GOARCH) {
		t.Fatalf("--version: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}

	stdout, stderr, code = runCommand(exec.Command(bin, "version", "--format", "json"))
	var info struct {
		SchemaVersion string `json:"schema_version"`
		Version       string `json:"version"`
		Commit        string `json:"commit"`
		Platform      string `json:"platform"`
	}
	if code != 0 || stderr != "" || json.Unmarshal([]byte(stdout), &info) != nil || info.SchemaVersion != "ai-dev.version/v1" || info.Version == "" {
		t.Fatalf("version json: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	// A build from this checkout embeds its commit (SC-001).
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	head := exec.Command("git", "rev-parse", "HEAD")
	head.Dir = repoRoot
	if out, err := head.Output(); err == nil && info.Commit != strings.TrimSpace(string(out)) {
		t.Fatalf("commit = %q, want HEAD %q", info.Commit, strings.TrimSpace(string(out)))
	}

	stdout, stderr, code = runCommand(exec.Command(bin, "--help"))
	if code != 0 || stderr != "" || !strings.Contains(stdout, "workflow guide") || !strings.Contains(stdout, "git commit validate") {
		t.Fatalf("--help: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

// TestReleaseVersionIsSetAtLinkTime proves the release mechanism the spec
// assumes: -ldflags -X sets the reported version.
func TestReleaseVersionIsSetAtLinkTime(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "ai-dev")
	build := exec.Command("go", "build", "-ldflags", "-X ai-dev/internal/version.Version=v9.9.9-test", "-o", bin, "./cmd/ai-dev")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	stdout, stderr, code := runCommand(exec.Command(bin, "version"))
	if code != 0 || stderr != "" || !strings.HasPrefix(stdout, "ai-dev v9.9.9-test (") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}
