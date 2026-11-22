//go:build darwin || linux

package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeRelease lays out a release as release.yml publishes it, under
// dir/download/<version>/, with a binary that reports version.
func fakeRelease(t *testing.T, version string) (dir, asset string) {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dir = t.TempDir()
	stage := t.TempDir()
	build := exec.Command("go", "build", "-trimpath",
		"-ldflags", "-X github.com/germanmunozs/ai-dev/internal/version.Version="+version,
		"-o", filepath.Join(stage, "ai-dev"), "./cmd/ai-dev")
	build.Dir = repoRoot
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	asset = fmt.Sprintf("ai-dev_%s_%s_%s.tar.gz", version, runtime.GOOS, runtime.GOARCH)
	releaseDir := filepath.Join(dir, "download", version)
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	archive, err := os.Create(filepath.Join(releaseDir, asset))
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(archive)
	tw := tar.NewWriter(gz)
	binary, err := os.ReadFile(filepath.Join(stage, "ai-dev"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []struct {
		name string
		data []byte
		mode int64
	}{{"ai-dev", binary, 0o755}, {"LICENSE", []byte("license\n"), 0o644}} {
		if err := tw.WriteHeader(&tar.Header{Name: file.name, Mode: file.mode, Size: int64(len(file.data)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(file.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(releaseDir, asset))
	if err != nil {
		t.Fatal(err)
	}
	sums := fmt.Sprintf("%x  %s\n", sha256.Sum256(data), asset)
	if err := os.WriteFile(filepath.Join(releaseDir, "SHA256SUMS"), []byte(sums), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, asset
}

// runInstaller runs scripts/install.sh under sh with a minimal environment:
// no Go and no Git on PATH, only the system tools and the given extra
// directory.
func runInstaller(t *testing.T, releases, extraPath string, args ...string) (string, int) {
	t.Helper()
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	path := "/usr/bin:/bin"
	if extraPath != "" {
		path = extraPath + ":" + path
	}
	cmd := exec.Command("/bin/sh", append([]string{filepath.Join(repoRoot, "scripts", "install.sh")}, args...)...)
	cmd.Env = []string{"HOME=" + t.TempDir(), "PATH=" + path, "AI_DEV_RELEASES_URL=file://" + releases}
	output, err := cmd.CombinedOutput()
	if exit, ok := err.(*exec.ExitError); ok {
		return string(output), exit.ExitCode()
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(output), 0
}

// fakeGH is a gh whose "attestation verify" exits with code.
func fakeGH(t *testing.T, code int) string {
	t.Helper()
	dir := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\n[ \"$1 $2\" = \"attestation verify\" ] || exit 64\nexit %d\n", code)
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestInstallScript drives scripts/install.sh against a local release with
// the layout release.yml publishes, without Go or Git on PATH.
func TestInstallScript(t *testing.T) {
	releases, asset := fakeRelease(t, "v9.9.9")

	t.Run("checksum only, without gh", func(t *testing.T) {
		prefix := filepath.Join(t.TempDir(), "bin")
		output, code := runInstaller(t, releases, "", "--version", "v9.9.9", "--prefix", prefix)
		if code != 0 || !strings.Contains(output, "Checksum verified.") || !strings.Contains(output, "only the checksum was verified") {
			t.Fatalf("exit %d\n%s", code, output)
		}
		version, err := exec.Command(filepath.Join(prefix, "ai-dev"), "version").Output()
		if err != nil || !strings.HasPrefix(string(version), "ai-dev v9.9.9 ") {
			t.Fatalf("installed binary: %q %v", version, err)
		}
		if output, code := runInstaller(t, releases, "", "--uninstall", "--prefix", prefix); code != 0 || !strings.Contains(output, "Removed") {
			t.Fatalf("uninstall: exit %d\n%s", code, output)
		}
	})

	t.Run("attestation verified with gh", func(t *testing.T) {
		prefix := filepath.Join(t.TempDir(), "bin")
		output, code := runInstaller(t, releases, fakeGH(t, 0), "--version", "v9.9.9", "--prefix", prefix)
		if code != 0 || !strings.Contains(output, "Provenance attestation verified.") {
			t.Fatalf("exit %d\n%s", code, output)
		}
	})

	refusals := []struct {
		name, gh string
		args     []string
		want     string
	}{
		{"attestation fails", fakeGH(t, 1), []string{"--version", "v9.9.9"}, "attestation of " + asset + " does not verify"},
		{"not a release version", "", []string{"--version", "1.0"}, "is not vMAJOR.MINOR.PATCH"},
		{"pre-release", "", []string{"--version", "v1.0.0-rc.1"}, "is not vMAJOR.MINOR.PATCH"},
		{"no such release", "", []string{"--version", "v1.2.3"}, "cannot download"},
	}
	for _, r := range refusals {
		t.Run(r.name, func(t *testing.T) {
			prefix := filepath.Join(t.TempDir(), "bin")
			output, code := runInstaller(t, releases, r.gh, append(r.args, "--prefix", prefix)...)
			if code == 0 || !strings.Contains(output, r.want) {
				t.Fatalf("exit %d, want a refusal with %q\n%s", code, r.want, output)
			}
			if _, err := os.Stat(filepath.Join(prefix, "ai-dev")); !os.IsNotExist(err) {
				t.Fatalf("something was installed: %v", err)
			}
		})
	}

	t.Run("tampered archive", func(t *testing.T) {
		path := filepath.Join(releases, "download", "v9.9.9", asset)
		file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = file.WriteString("tampered")
		file.Close()
		prefix := filepath.Join(t.TempDir(), "bin")
		output, code := runInstaller(t, releases, "", "--version", "v9.9.9", "--prefix", prefix)
		if code == 0 || !strings.Contains(output, "checksum mismatch") {
			t.Fatalf("exit %d\n%s", code, output)
		}
		if _, err := os.Stat(filepath.Join(prefix, "ai-dev")); !os.IsNotExist(err) {
			t.Fatalf("a tampered archive was installed: %v", err)
		}
	})
}
