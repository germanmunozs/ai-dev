package app

import (
	"bytes"
	"context"
	"io/fs"
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
