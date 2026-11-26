package app

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/germanmunozs/ai-dev/internal/system"
	"github.com/germanmunozs/ai-dev/internal/watch"
)

// The source is built from the project's read-only file system and the
// executor only (F015-FR-019, owner decision 1).
var _ func(fs.FS, system.Executor) watch.Source = newWatchSource

func runWatchArgs(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := RunWatch(context.Background(), args, Dependencies{Executor: &fakeExecutor{}, Root: fstest.MapFS{}, Stdout: &stdout, Stderr: &stderr})
	return code, stdout.String(), stderr.String()
}

func TestWatchUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"--zz-undefined"}, {"extra"}, {"--format", "xml"},
		{"--debounce", "49ms"}, {"--debounce", "6s"}, {"--debounce", "soon"},
		{"--heartbeat", "4s"}, {"--heartbeat", "301s"},
		{"--environment-interval", "4s"}, {"--environment-interval", "3601s"}, {"--environment-interval", "-1s"},
		{"--feature", ".."}, {"--feature", "a/b"}, {"--feature", ""},
		{"--once=maybe"},
	} {
		code, stdout, stderr := runWatchArgs(t, args...)
		if code != 2 || stdout != "" || stderr != watchUsage+"\n" {
			t.Errorf("%q: exit %d, stdout %q, stderr %q; want the usage line", args, code, stdout, stderr)
		}
	}
}

// RunWatch needs no dependency other than the project's file system, the
// executor and the output (owner decision 1).
func TestWatchNeedsOnlyRootAndExecutor(t *testing.T) {
	root := fstest.MapFS{"specs/001-a/spec.md": {Data: []byte("# a\n")}}
	var stdout, stderr bytes.Buffer
	code := RunWatch(context.Background(), []string{"--once", "--format", "json"},
		Dependencies{Executor: &fakeExecutor{}, Root: root, Stdout: &stdout, Stderr: &stderr})
	if code != 0 || !bytes.HasPrefix(stdout.Bytes(), []byte(`{"schema_version":"ai-dev.watch/v1","seq":1,"type":"snapshot"`)) || stderr.Len() != 0 {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}

func TestWatchOptionDefaultsAndLimits(t *testing.T) {
	options, format, ok := parseWatchOptions(nil)
	if !ok || format != "text" || options.Debounce != 250*time.Millisecond || options.Heartbeat != 15*time.Second ||
		options.EnvironmentInterval != 60*time.Second || options.Once || len(options.Features) != 0 {
		t.Fatalf("defaults = %+v, %q, %v", options, format, ok)
	}
	options, format, ok = parseWatchOptions([]string{"--format", "json", "--feature", "a", "--feature", "b",
		"--debounce", "50ms", "--heartbeat", "300s", "--environment-interval", "0", "--once"})
	if !ok || format != "json" || options.Debounce != 50*time.Millisecond || options.Heartbeat != 300*time.Second ||
		options.EnvironmentInterval != 0 || !options.Once || len(options.Features) != 2 || options.Features[1] != "b" {
		t.Fatalf("parsed = %+v, %q, %v", options, format, ok)
	}
	for _, args := range [][]string{{"--debounce", "5s"}, {"--heartbeat", "5s"}, {"--environment-interval", "5s"}, {"--environment-interval", "3600s"}} {
		if _, _, ok := parseWatchOptions(args); !ok {
			t.Errorf("%q refused; it is a limit", args)
		}
	}
}

// watchExecutor finds the tools named in paths and records every call.
type watchExecutor struct {
	mu      sync.Mutex
	paths   map[string]string
	runs    []string
	lookups []string
}

func (r *watchExecutor) LookPath(name string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lookups = append(r.lookups, name)
	if path := r.paths[name]; path != "" {
		return path, nil
	}
	return "", errors.New("not found")
}

func (r *watchExecutor) Run(_ context.Context, name string, args ...string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs = append(r.runs, strings.Join(append([]string{name}, args...), " "))
	return "true\n", nil
}

func (r *watchExecutor) forget(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.paths, name)
}

// Revalidation sees an agent leave the search path through the commands'
// own functions (US4 scenario 3); no agent runs.
func TestWatchSourceSeesAgentLeavePath(t *testing.T) {
	dir := writeWatchFixture(t, watchAgentsConfig)
	executor := &watchExecutor{paths: map[string]string{"git": "/usr/bin/git", "claude": "/tools/claude", "codex": "/tools/codex", "specify": "/tools/specify"}}
	source := newWatchSource(os.DirFS(dir), executor)
	envBefore, setupBefore := source.Environment(context.Background()), source.Setup()
	executor.forget("claude")
	executor.forget("codex")
	envAfter, setupAfter := source.Environment(context.Background()), source.Setup()
	if bytes.Equal(setupBefore, setupAfter) {
		t.Errorf("setup unchanged after the agents left the path:\n%s", setupAfter)
	}
	status := func(r watch.EnvironmentResult) string {
		for _, c := range r.Checks {
			if c.ID == "coding-agent-tool" {
				return c.Status
			}
		}
		return "missing"
	}
	if status(envBefore) != "pass" || status(envAfter) != "fail" {
		t.Errorf("coding-agent-tool check %s then %s, want pass then fail", status(envBefore), status(envAfter))
	}
}

// The source runs nothing but the doctor check's git rev-parse, once per
// environment evaluation (F015-FR-020, analysis C1).
func TestWatchSourceRunsOnlyGitRevParse(t *testing.T) {
	dir := writeWatchFixture(t, watchAgentsConfig)
	executor := &watchExecutor{paths: map[string]string{"git": "/usr/bin/git", "claude": "/tools/claude", "codex": "/tools/codex", "specify": "/tools/specify"}}
	source := newWatchSource(os.DirFS(dir), executor)
	features, err := source.Features()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range features {
		source.State(id)
		source.Next(id)
	}
	source.Setup()
	source.Environment(context.Background())
	source.Environment(context.Background())
	want := []string{"git rev-parse --is-inside-work-tree", "git rev-parse --is-inside-work-tree"}
	if !slices.Equal(executor.runs, want) {
		t.Fatalf("runs %q, want %q", executor.runs, want)
	}
}

// A project reached through a symbolic link is observed at its real path;
// the adapters never follow a link, so the link itself would be skipped
// (review F4).
func TestWatchProjectResolvesSymbolicLinks(t *testing.T) {
	real, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	t.Chdir(link)
	project, identity := watchProject()
	if project != real {
		t.Fatalf("project %q, want the real path %q", project, real)
	}
	if _, err := identity(); err != nil {
		t.Fatalf("identity: %v", err)
	}
}
