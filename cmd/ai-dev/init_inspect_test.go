package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// treeDigest fingerprints every file under dir: path, mode, size, mtime and
// content, so any write, including a refreshed index, is detected.
func treeDigest(t *testing.T, dir string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		line := fmt.Sprintf("%s %v %d %d", path, info.Mode(), info.Size(), info.ModTime().UnixNano())
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			line += fmt.Sprintf(" %x", sha256.Sum256(data))
		}
		lines = append(lines, line)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func gitCommand(t *testing.T, dir string, env []string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir, cmd.Env = dir, env
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func TestInitInspectSubprocess(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("inspection is supported on macOS and Linux")
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available")
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(base, "ai-dev")
	build := exec.Command("go", "build", "-o", bin, "./cmd/ai-dev")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}

	home := filepath.Join(base, "home")
	stateHome := filepath.Join(base, "state")
	project := filepath.Join(base, "project")
	marker := filepath.Join(base, "configured-command-ran")
	for _, dir := range []string{home, project} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	env := []string{"HOME=" + home, "XDG_STATE_HOME=" + stateHome, "PATH=" + filepath.Dir(gitPath) + ":/usr/bin:/bin"}
	gitEnv := append([]string{"GIT_CONFIG_NOSYSTEM=1"}, env...)
	gitCommand(t, project, gitEnv, "init", "-q", "-b", "main")
	gitCommand(t, project, gitEnv, "config", "user.email", "t@example.com")
	gitCommand(t, project, gitEnv, "config", "user.name", "t")
	write := func(name, content string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(project, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(project, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".gitignore", "build/\n*.log\n")
	write("tracked.txt", "one\n")
	gitCommand(t, project, gitEnv, "add", "-A")
	gitCommand(t, project, gitEnv, "commit", "-q", "-m", "init")
	// Hostile configuration that would run a command if Git honored it.
	write(".gitattributes", "*.txt filter=evil\n")
	gitCommand(t, project, gitEnv, "config", "filter.evil.clean", "touch "+marker)
	gitCommand(t, project, gitEnv, "config", "core.fsmonitor", "touch "+marker)
	gitCommand(t, project, gitEnv, "config", "remote.origin.url", "https://alice:s3cr3t-token@example.com/r.git")
	write("tracked.txt", "two\n")
	write("untracked.txt", "u\n")
	write("build/out.bin", "o\n")
	write("debug.log", "l\n")

	before := treeDigest(t, project)
	run := func(args ...string) (string, string, int) {
		cmd := exec.Command(bin, args...)
		cmd.Dir, cmd.Env = project, env
		return runCommand(cmd)
	}

	stdout, stderr, code := run("init", "inspect", "--format", "json")
	if code != 0 {
		t.Fatalf("json exit %d: %s", code, stderr)
	}
	if strings.Count(strings.TrimSpace(stdout), "\n") != 0 || strings.Contains(stdout, "s3cr3t") || strings.Contains(stdout, "alice") {
		t.Fatalf("output is not one redacted line: %s", stdout)
	}
	var output struct {
		Schema     string `json:"schema_version"`
		Inspection struct {
			Kind string `json:"repository_kind"`
			Head struct {
				State string `json:"state"`
			} `json:"head"`
			Changes []struct {
				Kind string `json:"kind"`
			} `json:"worktree_changes"`
			Convert struct {
				Risk string `json:"risk"`
			} `json:"conversion_settings"`
			Remotes []struct {
				Fetch []struct {
					Display string `json:"display_redacted"`
				} `json:"fetch_urls"`
			} `json:"remotes_redacted"`
		} `json:"inspection"`
		Incompatibilities []struct {
			Reason string `json:"reason"`
		} `json:"incompatibilities"`
	}
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout)
	}
	inspection := output.Inspection
	kinds := map[string]bool{}
	for _, change := range inspection.Changes {
		kinds[change.Kind] = true
	}
	if output.Schema != "ai-dev.git-inspect-output/v1" || inspection.Kind != "normal" || inspection.Head.State != "attached" ||
		!kinds["unstaged"] || !kinds["untracked"] || !kinds["ignored"] || inspection.Convert.Risk != "external_code" ||
		len(inspection.Remotes) != 1 || inspection.Remotes[0].Fetch[0].Display != "https://<redacted>@example.com/r.git" {
		t.Fatalf("inspection %+v", output)
	}
	if _, _, code := run("init", "inspect"); code != 0 {
		t.Fatalf("text exit %d", code)
	}

	// Inspection is read-only: no project byte changed, nothing configured
	// ran, and no state was created.
	if after := treeDigest(t, project); after != before {
		t.Fatal("inspection changed the project or repository")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("a configured command ran during inspection")
	}
	if _, err := os.Stat(stateHome); !os.IsNotExist(err) {
		t.Fatal("inspection created Git state")
	}

	empty := filepath.Join(base, "empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	stdout, _, code = run("init", "inspect", "--project-dir", empty, "--format", "json")
	if code != 0 || !strings.Contains(stdout, `"repository_kind":"absent"`) {
		t.Fatalf("absent project: %d %s", code, stdout)
	}
	for _, args := range [][]string{
		{"init", "inspect", "--format", "yaml"}, {"init", "inspect", "extra"}, {"init"}, {"init", "apply"},
	} {
		if _, _, code := run(args...); code != 2 {
			t.Errorf("%v exit %d, want 2", args, code)
		}
	}
	if _, _, code := run("init", "inspect", "--project-dir", filepath.Join(base, "missing")); code != 1 {
		t.Errorf("missing project exit %d, want 1", code)
	}
}
