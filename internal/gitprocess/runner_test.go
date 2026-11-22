package gitprocess

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/germanmunozs/ai-dev/internal/gitmanager"
)

// hostileFixture is a repository whose configuration points every
// command-executing mechanism at a sentinel script. A sentinel appends its
// mechanism name to marker, so an empty marker proves nothing ran.
type hostileFixture struct {
	git     string
	project string
	marker  string
	ctx     Context
	blobOID gitmanager.ObjectOID
}

func requireGit(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("Git runner tests need a supported POSIX platform")
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	return git
}

func physicalTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
}

// rawGit runs Git the unprotected way, for fixture setup and negative
// controls only.
func rawGit(t *testing.T, git, dir string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command(git, args...)
	cmd.Dir = dir
	cmd.Env = append([]string{"HOME=" + dir, "GIT_CONFIG_NOSYSTEM=1", "GIT_AUTHOR_NAME=a", "GIT_AUTHOR_EMAIL=a@x",
		"GIT_COMMITTER_NAME=a", "GIT_COMMITTER_EMAIL=a@x", "PATH=/usr/bin:/bin"}, env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func newHostileFixture(t *testing.T) hostileFixture {
	t.Helper()
	git := requireGit(t)
	base := physicalTempDir(t)
	project := filepath.Join(base, "project")
	sentinels := filepath.Join(base, "sentinels")
	stateRoot := filepath.Join(base, "state", "ai-dev", "git")
	for _, dir := range []string{project, sentinels, filepath.Join(stateRoot, "environment"), filepath.Join(sentinels, "hooks")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	marker := filepath.Join(base, "marker")
	sentinel := func(mechanism, tail string) string {
		path := filepath.Join(sentinels, mechanism)
		writeExecutable(t, path, "printf '%s\\n' "+mechanism+" >> '"+marker+"'\n"+tail)
		return path
	}
	rawGit(t, git, project, nil, "init", "-q", "--initial-branch=main")
	if err := os.WriteFile(filepath.Join(project, "tracked.txt"), []byte("tracked\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rawGit(t, git, project, nil, "add", "tracked.txt")
	rawGit(t, git, project, nil, "commit", "-q", "-m", "init")
	blob := rawGit(t, git, project, nil, "rev-parse", "HEAD:tracked.txt")

	// Install the hostile configuration only after setup, so setup never
	// trips it.
	if err := os.WriteFile(filepath.Join(project, ".gitattributes"), []byte("* filter=evil\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(sentinels, "hooks", "post-index-change")
	writeExecutable(t, hook, "printf 'hook\\n' >> '"+marker+"'\n")
	for key, value := range map[string]string{
		"core.pager":          sentinel("pager", "cat"),
		"core.editor":         sentinel("editor", ""),
		"sequence.editor":     sentinel("sequence-editor", ""),
		"credential.helper":   "!" + sentinel("credential", ""),
		"core.askPass":        sentinel("askpass", ""),
		"core.sshCommand":     sentinel("ssh", ""),
		"core.fsmonitor":      sentinel("fsmonitor", "exit 1"),
		"core.hooksPath":      filepath.Join(sentinels, "hooks"),
		"diff.external":       sentinel("external-diff", ""),
		"filter.evil.clean":   sentinel("clean-filter", "cat"),
		"filter.evil.smudge":  sentinel("smudge-filter", "cat"),
		"filter.evil.process": sentinel("process-filter", ""),
		"gpg.program":         sentinel("gpg", ""),
		"commit.gpgSign":      "true",
		"alias.status":        "!" + sentinel("alias", ""),
	} {
		rawGit(t, git, project, nil, "config", "--local", key, value)
	}

	root := gitmanager.NewObservedPath([]byte(stateRoot))
	environment, err := gitmanager.NewEnvironmentProfile(root)
	if err != nil {
		t.Fatal(err)
	}
	tool, err := identifyTool(git, environment, []byte(project))
	if err != nil {
		t.Fatalf("identify: %v", err)
	}
	return hostileFixture{git: git, project: project, marker: marker, blobOID: gitmanager.ObjectOID(blob),
		ctx: Context{Tool: tool, Environment: environment, Directory: []byte(project)}}
}

func (f hostileFixture) markers(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(f.marker)
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func (f hostileFixture) run(t *testing.T, op Operation, p Parameters) gitmanager.GitRunResult {
	t.Helper()
	invocation, err := Build(op, f.ctx, p)
	if err != nil {
		t.Fatalf("build %s: %v", op, err)
	}
	result, err := Runner{}.Run(context.Background(), invocation)
	if err != nil {
		t.Fatalf("run %s: %v (stderr %q)", op, err, result.Stderr)
	}
	return result
}

// Negative control: the fixture is hostile. Unprotected Git under the same
// configuration runs the sentinels, so their absence below is meaningful.
func TestHostileFixtureFiresUnprotected(t *testing.T) {
	f := newHostileFixture(t)
	if err := os.WriteFile(filepath.Join(f.project, "tracked.txt"), []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(f.git, "add", "tracked.txt")
	cmd.Dir, cmd.Env = f.project, []string{"HOME=" + f.project, "GIT_CONFIG_NOSYSTEM=1", "PATH=/usr/bin:/bin"}
	_, _ = cmd.CombinedOutput()
	got := f.markers(t)
	if !strings.Contains(got, "fsmonitor") {
		t.Errorf("unprotected git did not run the fsmonitor sentinel (markers %q)", got)
	}
	// A configured process filter takes precedence over the clean filter.
	if !strings.Contains(got, "process-filter") && !strings.Contains(got, "clean-filter") {
		t.Errorf("unprotected git did not run a filter sentinel (markers %q)", got)
	}
}

func TestRunnerRunsMatrixRowsWithoutConfiguredCode(t *testing.T) {
	f := newHostileFixture(t)
	// A stat-dirty tracked file makes status compare contents through the
	// clean filter unless the driver is neutralized.
	if err := os.WriteFile(filepath.Join(f.project, "tracked.txt"), []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.ctx.Overrides.FilterDrivers = []string{"evil"}

	status := f.run(t, OpStatus, Parameters{})
	if status.ExitCode != 0 || !bytes.Contains(status.Stdout, []byte("tracked.txt")) {
		t.Fatalf("status exit %d stdout %q stderr %q", status.ExitCode, status.Stdout, status.Stderr)
	}
	for _, op := range []Operation{OpDiscoverRepository, OpHeadSymbolicRef, OpHeadCommit, OpShowRefs, OpWorktreeDiff, OpIndexDiff, OpConfigReadLocal} {
		if result := f.run(t, op, Parameters{}); result.ExitCode != 0 {
			t.Errorf("%s exit %d stderr %q", op, result.ExitCode, result.Stderr)
		}
	}
	if result := f.run(t, OpObjectContent, Parameters{ObjectOID: f.blobOID}); string(result.Stdout) != "tracked\n" {
		t.Fatalf("cat-file -p %q", result.Stdout)
	}
	blob := f.run(t, OpHashBlob, Parameters{Stdin: []byte("exact bytes\n")})
	if blob.ExitCode != 0 {
		t.Fatalf("hash-object exit %d stderr %q", blob.ExitCode, blob.Stderr)
	}
	treeOID := gitmanager.ObjectOID(rawGit(t, f.git, f.project, nil, "rev-parse", "HEAD^{tree}"))
	if listing := f.run(t, OpObjectContent, Parameters{ObjectOID: treeOID}); !bytes.Contains(listing.Stdout, []byte("tracked.txt")) {
		t.Fatalf("tree listing %q", listing.Stdout)
	}
	when := time.Date(2026, 11, 17, 10, 30, 0, 0, time.FixedZone("", 2*3600))
	identity := gitmanager.GitIdentity{Name: "A U Thor", Email: "author@example.test"}
	commit := f.run(t, OpCommitTree, Parameters{TreeOID: treeOID, Author: identity, Committer: identity, AuthorDate: when, CommitterDate: when, Stdin: []byte("chore: frozen\n")})
	commitOID := gitmanager.ObjectOID(strings.TrimSpace(string(commit.Stdout)))
	if commit.ExitCode != 0 || commitOID.Validate() != nil {
		t.Fatalf("commit-tree exit %d stdout %q stderr %q", commit.ExitCode, commit.Stdout, commit.Stderr)
	}
	body := string(f.run(t, OpObjectContent, Parameters{ObjectOID: commitOID}).Stdout)
	if !strings.Contains(body, "author A U Thor <author@example.test> 1794904200 +0200") || strings.Contains(body, "gpgsig") {
		t.Fatalf("commit not frozen or signed:\n%s", body)
	}
	if result := f.run(t, OpUpdateRef, Parameters{RefName: []byte("refs/heads/frozen"), ObjectOID: commitOID}); result.ExitCode != 0 {
		t.Fatalf("update-ref exit %d stderr %q", result.ExitCode, result.Stderr)
	}
	// The same CAS again fails: the ref is no longer absent.
	if result := f.run(t, OpUpdateRef, Parameters{RefName: []byte("refs/heads/frozen"), ObjectOID: commitOID}); result.ExitCode == 0 {
		t.Fatal("absent-old CAS succeeded against an existing ref")
	}
	if _, err := os.Stat(filepath.Join(f.project, ".git", "logs", "refs", "heads", "frozen")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reflog created for the CAS: %v", err)
	}
	if result := f.run(t, OpConfigWriteLocal, Parameters{ConfigKey: "gitflow.branch.master", ConfigValue: "main"}); result.ExitCode != 0 {
		t.Fatalf("config write exit %d stderr %q", result.ExitCode, result.Stderr)
	}
	if got := f.markers(t); got != "" {
		t.Fatalf("configured code ran: %q", got)
	}
}

func TestRunnerInitIsDeterministic(t *testing.T) {
	f := newHostileFixture(t)
	target := filepath.Join(physicalTempDir(t), "fresh")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	f.ctx.Directory = []byte(target)
	if result := f.run(t, OpInit, Parameters{BranchName: []byte("trunk"), PhysicalRoot: []byte(target)}); result.ExitCode != 0 {
		t.Fatalf("init exit %d stderr %q", result.ExitCode, result.Stderr)
	}
	head, err := os.ReadFile(filepath.Join(target, ".git", "HEAD"))
	if err != nil || string(head) != "ref: refs/heads/trunk\n" {
		t.Fatalf("HEAD %q, %v", head, err)
	}
	hooks, err := os.ReadDir(filepath.Join(target, ".git", "hooks"))
	if err == nil && len(hooks) != 0 {
		t.Fatalf("template hooks copied: %d entries", len(hooks))
	}
	if got := f.markers(t); got != "" {
		t.Fatalf("configured code ran: %q", got)
	}
}

// Ambient GIT_* variables, HOME and global configuration of the parent
// process never reach Git.
func TestRunnerIgnoresAmbientEnvironment(t *testing.T) {
	f := newHostileFixture(t)
	hostileHome := physicalTempDir(t)
	if err := os.WriteFile(filepath.Join(hostileHome, ".gitconfig"), []byte("[core]\n\tfsmonitor = "+filepath.Join(filepath.Dir(f.marker), "sentinels", "fsmonitor")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", hostileHome)
	t.Setenv("GIT_DIR", "/nonexistent/git-dir")
	t.Setenv("GIT_CONFIG_PARAMETERS", "'core.pager=/nonexistent/pager'")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.fsmonitor")
	t.Setenv("GIT_CONFIG_VALUE_0", "true")
	t.Setenv("GIT_EXTERNAL_DIFF", "/nonexistent/diff")
	f.ctx.Overrides.FilterDrivers = []string{"evil"}
	if result := f.run(t, OpStatus, Parameters{}); result.ExitCode != 0 {
		t.Fatalf("status exit %d stderr %q", result.ExitCode, result.Stderr)
	}
	if got := f.markers(t); got != "" {
		t.Fatalf("ambient configuration reached Git: %q", got)
	}
}

// Shell metacharacters in an argument are bytes, never syntax.
func TestRunnerUsesExactArgvWithoutShell(t *testing.T) {
	f := newHostileFixture(t)
	pwned := filepath.Join(filepath.Dir(f.marker), "pwned")
	invocation, err := Build(OpCheckRefFormat, f.ctx, Parameters{BranchName: []byte("a;touch " + pwned + ";$(touch " + pwned + ")")})
	if err != nil {
		t.Fatal(err)
	}
	result, err := Runner{}.Run(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode == 0 {
		t.Fatal("check-ref-format accepted an invalid name")
	}
	if _, err := os.Stat(pwned); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("argument was interpreted by a shell")
	}
}

func TestRunnerBoundsCapturedOutput(t *testing.T) {
	f := newHostileFixture(t)
	invocation, err := Build(OpObjectContent, f.ctx, Parameters{ObjectOID: f.blobOID})
	if err != nil {
		t.Fatal(err)
	}
	invocation.StdoutLimit = 3
	result, err := Runner{}.Run(context.Background(), invocation)
	if !errors.Is(err, gitmanager.ErrCapturedOutputExceeded) {
		t.Fatalf("overflow not reported: %v", err)
	}
	if string(result.Stdout) != "tra" || result.StdoutObserved <= 3 {
		t.Fatalf("stdout %q observed %d", result.Stdout, result.StdoutObserved)
	}
	if limitErr := gitmanager.CheckCapturedStream(result.StdoutObserved, gitmanager.ReasonOperationFailed); limitErr != nil {
		t.Fatalf("observed count %d is not mappable within the catalogue bound: %v", result.StdoutObserved, limitErr)
	}
	invocation.StdoutLimit = 8
	result, err = Runner{}.Run(context.Background(), invocation)
	if err != nil || string(result.Stdout) != "tracked\n" || result.StdoutObserved != 8 {
		t.Fatalf("exact boundary: %q %d %v", result.Stdout, result.StdoutObserved, err)
	}
}

// The private HOME and XDG leaves must be absent before and after every
// invocation; the runner refuses otherwise.
func TestRunnerRequiresAbsentEnvironmentLeaves(t *testing.T) {
	f := newHostileFixture(t)
	home := string(f.ctx.Environment.Home.Bytes())

	// A process that creates the leaf is caught after it exits.
	creates := gitmanager.GitInvocation{Executable: "/bin/mkdir", Args: []string{home}, Directory: f.project,
		Environment: f.ctx.Environment.GitEnvironmentEntries(), StdoutLimit: 16, StderrLimit: 16}
	if _, err := (Runner{}).Run(context.Background(), creates); !errors.Is(err, ErrEnvironmentLeafPresent) {
		t.Fatalf("leaf created during the run not detected: %v", err)
	}
	// With the leaf present, nothing runs.
	invocation, err := Build(OpStatus, f.ctx, Parameters{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (Runner{}).Run(context.Background(), invocation); !errors.Is(err, ErrEnvironmentLeafPresent) {
		t.Fatalf("present leaf not refused: %v", err)
	}
}

func TestRunnerRejectsMalformedInvocations(t *testing.T) {
	f := newHostileFixture(t)
	valid, err := Build(OpStatus, f.ctx, Parameters{})
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*gitmanager.GitInvocation){
		"relative executable":  func(v *gitmanager.GitInvocation) { v.Executable = "git" },
		"relative directory":   func(v *gitmanager.GitInvocation) { v.Directory = "project" },
		"NUL in argument":      func(v *gitmanager.GitInvocation) { v.Args = append(v.Args, "a\x00b") },
		"missing HOME":         func(v *gitmanager.GitInvocation) { v.Environment = withoutEntry(v.Environment, "HOME=") },
		"missing XDG":          func(v *gitmanager.GitInvocation) { v.Environment = withoutEntry(v.Environment, "XDG_CONFIG_HOME=") },
		"duplicate variable":   func(v *gitmanager.GitInvocation) { v.Environment = append(v.Environment, "GIT_PAGER=less") },
		"malformed variable":   func(v *gitmanager.GitInvocation) { v.Environment = append(v.Environment, "NOEQUALS") },
		"zero stdout limit":    func(v *gitmanager.GitInvocation) { v.StdoutLimit = 0 },
		"oversized stderr cap": func(v *gitmanager.GitInvocation) { v.StderrLimit = gitmanager.LimitCapturedStreamBytes.Max() + 1 },
	} {
		invocation := valid
		invocation.Args = append([]string(nil), valid.Args...)
		invocation.Environment = append([]string(nil), valid.Environment...)
		mutate(&invocation)
		if _, err := (Runner{}).Run(context.Background(), invocation); !errors.Is(err, ErrInvalidInvocation) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if got := f.markers(t); got != "" {
		t.Fatalf("configured code ran: %q", got)
	}
}

func TestRunnerHonorsCancellation(t *testing.T) {
	f := newHostileFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	sleep := gitmanager.GitInvocation{Executable: "/bin/sleep", Args: []string{"10"}, Directory: f.project,
		Environment: f.ctx.Environment.GitEnvironmentEntries(), StdoutLimit: 16, StderrLimit: 16}
	started := time.Now()
	if _, err := (Runner{}).Run(ctx, sleep); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancellation not reported: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("cancellation took %s", elapsed)
	}
}

// identifyTool identifies git in the mode it actually supports: directly
// when it is the Git binary of its exec path, otherwise as a launcher (the
// macOS /usr/bin/git shim).
func identifyTool(git string, environment gitmanager.EnvironmentProfile, directory []byte) (gitmanager.GitToolIdentity, error) {
	tool, err := Identify(context.Background(), gitmanager.InvocationModeDirect, git, nil, environment, directory)
	if err == nil {
		return tool, nil
	}
	return Identify(context.Background(), gitmanager.InvocationModeLauncher, git, nil, environment, directory)
}

func TestIdentifyFreezesToolchain(t *testing.T) {
	f := newHostileFixture(t)
	tool := f.ctx.Tool
	if tool.InvocationPath != f.git || tool.ExecPath == "" || !strings.HasPrefix(tool.Version, "2.") || tool.Validate() != nil {
		t.Fatalf("tool identity %+v", tool)
	}
	for _, capability := range identifiedCapabilities {
		if !hasCapability(tool, capability) {
			t.Errorf("capability %s missing", capability)
		}
	}
	if _, err := Identify(context.Background(), gitmanager.InvocationModeDirect, "relative/git", nil, f.ctx.Environment, f.ctx.Directory); err == nil {
		t.Fatal("relative invocation path accepted")
	}
	if _, err := Identify(context.Background(), gitmanager.InvocationModeDirect, "/nonexistent/git", nil, f.ctx.Environment, f.ctx.Directory); err == nil {
		t.Fatal("missing executable accepted")
	}

	// The real Git binary of the exec path identifies directly, and as
	// itself; it can never pass as a launcher.
	binary, err := filepath.EvalSymlinks(filepath.Join(tool.ExecPath, "git"))
	if err != nil {
		t.Fatal(err)
	}
	direct, err := Identify(context.Background(), gitmanager.InvocationModeDirect, binary, nil, f.ctx.Environment, f.ctx.Directory)
	if err != nil {
		t.Fatalf("direct identify of %s: %v", binary, err)
	}
	if direct.EffectiveExecutablePath != binary || direct.EffectiveExecutableDigest != direct.InvocationPathDigest || len(direct.EnvironmentSelectors()) != 0 {
		t.Fatalf("direct identity %+v", direct)
	}
	expectRefusal := func(t *testing.T, err error) {
		t.Helper()
		var buildErr *BuildError
		if !errors.As(err, &buildErr) || buildErr.Reason != gitmanager.ReasonGitCapabilityMissing {
			t.Fatalf("got %v, want git_capability_missing", err)
		}
	}
	_, err = Identify(context.Background(), gitmanager.InvocationModeLauncher, binary, nil, f.ctx.Environment, f.ctx.Directory)
	expectRefusal(t, err)

	if runtime.GOOS != "darwin" || f.git != "/usr/bin/git" {
		return
	}
	// The macOS shim is a launcher: direct mode would freeze the shim's
	// digest instead of the Git that actually runs.
	_, err = Identify(context.Background(), gitmanager.InvocationModeDirect, "/usr/bin/git", nil, f.ctx.Environment, f.ctx.Directory)
	expectRefusal(t, err)
	if tool.InvocationMode != gitmanager.InvocationModeLauncher || tool.EffectiveExecutablePath != binary ||
		tool.EffectiveExecutableDigest == tool.InvocationPathDigest {
		t.Fatalf("launcher identity %+v", tool)
	}
	developerDir := strings.TrimSuffix(tool.ExecPath, developerDirSuffix)
	if got := tool.EnvironmentSelectors()["DEVELOPER_DIR"]; got != developerDir || developerDir == tool.ExecPath {
		t.Fatalf("DEVELOPER_DIR frozen as %q, want %q", got, developerDir)
	}
	invocation, err := Build(OpStatus, f.ctx, Parameters{})
	if err != nil || !contains(invocation.Environment, "DEVELOPER_DIR="+developerDir) {
		t.Fatalf("frozen DEVELOPER_DIR not passed to Git: %v", err)
	}
	_, err = Identify(context.Background(), gitmanager.InvocationModeLauncher, "/usr/bin/git",
		map[string]string{"DEVELOPER_DIR": "/Library/Developer/Elsewhere"}, f.ctx.Environment, f.ctx.Directory)
	if err == nil {
		t.Fatal("a DEVELOPER_DIR naming another developer directory was accepted")
	}
}

func TestParseGitVersion(t *testing.T) {
	for output, want := range map[string]string{
		"git version 2.54.0 (Apple Git-157)\n": "2.54.0",
		"git version 2.45.0\n":                 "2.45.0",
		"git version 2.47.1.windows.1\n":       "2.47.1",
	} {
		got, err := parseGitVersion([]byte(output))
		if err != nil || got != want {
			t.Errorf("%q: %q, %v", output, got, err)
		}
	}
	for _, output := range []string{"git version 2.44.9\n", "git version 1.9.0\n", "hello\n", "git version 2.x\n", ""} {
		_, err := parseGitVersion([]byte(output))
		var buildErr *BuildError
		if !errors.As(err, &buildErr) || buildErr.Reason != gitmanager.ReasonGitCapabilityMissing {
			t.Errorf("%q: %v, want git_capability_missing", output, err)
		}
	}
}

func withoutEntry(entries []string, prefix string) []string {
	var kept []string
	for _, entry := range entries {
		if !strings.HasPrefix(entry, prefix) {
			kept = append(kept, entry)
		}
	}
	return kept
}

// A submodule carries its own configuration, whose filters the planner
// never parsed. Status and diff must not descend into it (RUN-10).
func TestRunnerDoesNotDescendIntoSubmodules(t *testing.T) {
	f := newHostileFixture(t)
	base := filepath.Dir(f.marker)
	source := filepath.Join(base, "submodule-source")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	rawGit(t, f.git, source, nil, "init", "-q", "--initial-branch=main")
	for name, body := range map[string]string{"s.txt": "s\n", ".gitattributes": "* filter=subevil\n"} {
		if err := os.WriteFile(filepath.Join(source, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rawGit(t, f.git, source, nil, "add", "s.txt", ".gitattributes")
	rawGit(t, f.git, source, nil, "commit", "-q", "-m", "submodule")
	// Setup runs with the superproject's hostile mechanisms disabled.
	rawGit(t, f.git, f.project, nil, "-c", "protocol.file.allow=always", "-c", "filter.evil.process=", "-c", "filter.evil.clean=cat",
		"-c", "core.fsmonitor=false", "submodule", "add", "-q", source, "mod")
	module := filepath.Join(f.project, "mod")
	filter := filepath.Join(base, "submodule-filter")
	writeExecutable(t, filter, "printf 'submodule-filter\\n' >> '"+f.marker+"'\ncat\n")
	rawGit(t, f.git, module, nil, "config", "--local", "filter.subevil.clean", filter)

	// Same size and a new mtime: Git can only decide by filtered content.
	touch := func(offset time.Duration) {
		path := filepath.Join(module, "s.txt")
		if err := os.WriteFile(path, []byte("t\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		when := time.Now().Add(offset)
		if err := os.Chtimes(path, when, when); err != nil {
			t.Fatal(err)
		}
		if err := os.Truncate(f.marker, 0); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
	}
	touch(2 * time.Second)
	// Negative control: plain status inside the submodule runs its filter.
	cmd := exec.Command(f.git, "status", "--porcelain")
	cmd.Dir, cmd.Env = module, []string{"HOME=" + module, "GIT_CONFIG_NOSYSTEM=1", "PATH=/usr/bin:/bin"}
	_, _ = cmd.CombinedOutput()
	if !strings.Contains(f.markers(t), "submodule-filter") {
		t.Fatalf("negative control: the submodule filter did not run (markers %q)", f.markers(t))
	}

	f.ctx.Overrides.FilterDrivers = []string{"evil"}
	for i, op := range []Operation{OpStatus, OpWorktreeDiff, OpIndexDiff} {
		touch(time.Duration(i+3) * time.Second)
		if result := f.run(t, op, Parameters{}); result.ExitCode != 0 {
			t.Fatalf("%s exit %d stderr %q", op, result.ExitCode, result.Stderr)
		}
		if got := f.markers(t); got != "" {
			t.Fatalf("%s ran a submodule's configured code: %q", op, got)
		}
	}
}

// T130: the mutating rows run no hook, configured command or filter, in a
// repository whose own hooks directory and hostile core.hooksPath both
// hold sentinel hooks, including reference-transaction, which Git runs on
// every reference update. A raw update-ref first proves the hooks are live.
func TestRunnerMutatingRowsRunNoHook(t *testing.T) {
	f := newHostileFixture(t)
	tree := gitmanager.ObjectOID(rawGit(t, f.git, f.project, nil, "rev-parse", "HEAD^{tree}"))
	head := rawGit(t, f.git, f.project, nil, "rev-parse", "HEAD")
	hooksPath := rawGit(t, f.git, f.project, nil, "config", "--local", "core.hooksPath")
	for _, dir := range []string{filepath.Join(f.project, ".git", "hooks"), hooksPath} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		for _, hook := range []string{"reference-transaction", "post-index-change", "pre-commit", "post-commit", "post-checkout", "pre-auto-gc"} {
			writeExecutable(t, filepath.Join(dir, hook), "printf '%s\\n' hook-"+hook+" >> '"+f.marker+"'\n")
		}
	}
	rawGit(t, f.git, f.project, nil, "update-ref", "refs/heads/control", head)
	if got := f.markers(t); !strings.Contains(got, "hook-reference-transaction") {
		t.Fatalf("the negative control ran no reference-transaction hook: %q", got)
	}
	if err := os.Remove(f.marker); err != nil {
		t.Fatal(err)
	}

	identity := gitmanager.GitIdentity{Name: "Ada", Email: "ada@example.org"}
	if result := f.run(t, OpConfigWriteLocal, Parameters{ConfigKey: "gitflow.branch.master", ConfigValue: "main"}); result.ExitCode != 0 {
		t.Fatalf("config write exit %d %q", result.ExitCode, result.Stderr)
	}
	if result := f.run(t, OpHashBlob, Parameters{Stdin: []byte("blob\n")}); result.ExitCode != 0 {
		t.Fatalf("hash-object exit %d %q", result.ExitCode, result.Stderr)
	}
	commit := f.run(t, OpCommitTree, Parameters{TreeOID: tree, Author: identity, Committer: identity,
		AuthorDate: time.Unix(1, 0), CommitterDate: time.Unix(1, 0), Stdin: []byte("chore: x\n")})
	if commit.ExitCode != 0 {
		t.Fatalf("commit-tree exit %d %q", commit.ExitCode, commit.Stderr)
	}
	oid := gitmanager.ObjectOID(strings.TrimSpace(string(commit.Stdout)))
	if result := f.run(t, OpUpdateRef, Parameters{RefName: []byte("refs/heads/new"), ObjectOID: oid}); result.ExitCode != 0 {
		t.Fatalf("update-ref exit %d %q", result.ExitCode, result.Stderr)
	}
	index := filepath.Join(physicalTempDir(t), "index")
	if result := f.run(t, OpPreparedIndex, Parameters{TreeOID: tree, IndexFile: []byte(index)}); result.ExitCode != 0 {
		t.Fatalf("read-tree exit %d %q", result.ExitCode, result.Stderr)
	}
	if got := f.markers(t); got != "" {
		t.Fatalf("a mutating row ran configured code: %q", got)
	}
}

// R-04: a write never lands in an enclosing repository. The project is a
// repository nested in another one; when its .git vanishes before the
// write, Git fails instead of discovering and changing the parent.
func TestWriteRowsNeverDiscoverAnEnclosingRepository(t *testing.T) {
	git := requireGit(t)
	base := physicalTempDir(t)
	parent, project := filepath.Join(base, "parent"), filepath.Join(base, "parent", "project")
	stateRoot := filepath.Join(base, "state", "ai-dev", "git")
	for _, dir := range []string{project, filepath.Join(stateRoot, "environment")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	rawGit(t, git, parent, nil, "init", "-q", "--initial-branch=main")
	rawGit(t, git, project, nil, "init", "-q", "--initial-branch=main")
	before, err := os.ReadFile(filepath.Join(parent, ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}
	objects := func() int {
		n := 0
		filepath.WalkDir(filepath.Join(parent, ".git", "objects"), func(_ string, entry os.DirEntry, err error) error {
			if err == nil && entry.Type().IsRegular() {
				n++
			}
			return err
		})
		return n
	}
	objectsBefore := objects()
	environment, err := gitmanager.NewEnvironmentProfile(gitmanager.NewObservedPath([]byte(stateRoot)))
	if err != nil {
		t.Fatal(err)
	}
	tool, err := identifyTool(git, environment, []byte(project))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(project, ".git")); err != nil {
		t.Fatal(err)
	}
	ctx := Context{Tool: tool, Environment: environment, Directory: []byte(project)}
	for op, params := range map[Operation]Parameters{
		OpConfigWriteLocal: {ConfigKey: "gitflow.branch.master", ConfigValue: "main"},
		OpHashBlob:         {Stdin: []byte("blob\n")},
	} {
		invocation, err := Build(op, ctx, params)
		if err != nil {
			t.Fatal(err)
		}
		if result, err := (Runner{}).Run(context.Background(), invocation); err == nil && result.ExitCode == 0 {
			t.Fatalf("%s succeeded without the project's repository", op)
		}
	}
	after, _ := os.ReadFile(filepath.Join(parent, ".git", "config"))
	if !bytes.Equal(before, after) {
		t.Fatalf("the enclosing repository was changed:\n%s", after)
	}
	if objects() != objectsBefore {
		t.Fatal("an object was written to the enclosing repository")
	}
}

// Debian and Ubuntu install the Git binary twice, as separate files with
// identical bytes. Direct mode accepts a byte-identical copy of the exec
// path's Git, which is the same program, and still refuses a launcher
// script, whose bytes always differ.
func TestIdentifyAcceptsIdenticalCopyAndRefusesLauncher(t *testing.T) {
	git := requireGit(t)
	base := physicalTempDir(t)
	stateRoot := filepath.Join(base, "state", "ai-dev", "git")
	if err := os.MkdirAll(filepath.Join(stateRoot, "environment"), 0o700); err != nil {
		t.Fatal(err)
	}
	environment, err := gitmanager.NewEnvironmentProfile(gitmanager.NewObservedPath([]byte(stateRoot)))
	if err != nil {
		t.Fatal(err)
	}
	execPath := rawGit(t, git, base, nil, "--exec-path")
	real := filepath.Join(execPath, "git")
	data, err := os.ReadFile(real)
	if err != nil {
		t.Skipf("the exec path's Git cannot be read: %v", err)
	}
	copied := filepath.Join(base, "bin", "git")
	if err := os.MkdirAll(filepath.Dir(copied), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(copied, data, 0o755); err != nil {
		t.Fatal(err)
	}
	if relocated := rawGit(t, copied, base, nil, "--exec-path"); relocated != execPath {
		t.Skipf("this Git is relocatable: the copy reports exec path %s", relocated)
	}
	tool, err := Identify(context.Background(), gitmanager.InvocationModeDirect, copied, nil, environment, []byte(base))
	if err != nil {
		t.Fatalf("a byte-identical copy was refused: %v", err)
	}
	if tool.EffectiveExecutablePath != copied {
		t.Fatalf("effective executable %s", tool.EffectiveExecutablePath)
	}
	launcher := filepath.Join(base, "bin", "launcher")
	writeExecutable(t, launcher, "exec '"+real+"' \"$@\"\n")
	if _, err := Identify(context.Background(), gitmanager.InvocationModeDirect, launcher, nil, environment, []byte(base)); err == nil ||
		!strings.Contains(err.Error(), "launcher") {
		t.Fatalf("a launcher script was accepted as Git: %v", err)
	}
}
