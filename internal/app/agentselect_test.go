package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/germanmunozs/ai-dev/internal/agent"
	"github.com/germanmunozs/ai-dev/internal/system"
)

// errWriter always fails on the first Write call.
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

// fsSnapshotEntry captures everything about one filesystem entry that a
// filesystem-mutation test cares about: its kind (via Mode's type bits), its
// permission bits, a regular file's content, and a symlink's target. Two
// entries are equal (via reflect.DeepEqual on the containing map) only if
// none of these differ.
type fsSnapshotEntry struct {
	Mode       os.FileMode
	Content    string
	LinkTarget string
}

// snapshotDir captures the relative-path -> fsSnapshotEntry map of every
// entry under dir (files, empty directories, and symlinks), so a caller can
// assert a directory tree is unchanged before and after an operation. It
// never follows symlinks (filepath.Walk uses lstat semantics), so a
// symlink's own target is recorded rather than the content of what it
// points to.
func snapshotDir(t *testing.T, dir string) map[string]fsSnapshotEntry {
	t.Helper()
	snap := make(map[string]fsSnapshotEntry)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == dir {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		entry := fsSnapshotEntry{Mode: info.Mode() & (os.ModePerm | os.ModeDir | os.ModeSymlink)}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			entry.LinkTarget = target
		case info.IsDir():
			// Presence and permission bits only; no content to capture.
		default:
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			entry.Content = string(data)
		}
		snap[rel] = entry
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", dir, err)
	}
	return snap
}

// controlledTempDir builds a small, fixed file tree in a controlled temporary
// directory (never the developer's real repository) and chdirs the test into
// it for the duration of the test.
func controlledTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fixture.txt"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nested", "file.txt"), []byte("nested\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	return dir
}

// TestSnapshotDirDetectsFileContentDeletionAndCreation proves the helper
// itself, not just the higher-level feature tests that use it, notices a
// regular file's content changing, a file disappearing, and a new file
// appearing.
func TestSnapshotDirDetectsFileContentDeletionAndCreation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := snapshotDir(t, dir)

	t.Run("content change", func(t *testing.T) {
		if err := os.WriteFile(path, []byte("changed"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.WriteFile(path, []byte("original"), 0o644) })
		if reflect.DeepEqual(base, snapshotDir(t, dir)) {
			t.Fatal("snapshot did not detect a file content change")
		}
	})

	t.Run("file deletion", func(t *testing.T) {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.WriteFile(path, []byte("original"), 0o644) })
		if reflect.DeepEqual(base, snapshotDir(t, dir)) {
			t.Fatal("snapshot did not detect a file deletion")
		}
	})

	t.Run("file creation", func(t *testing.T) {
		newPath := filepath.Join(dir, "extra.txt")
		if err := os.WriteFile(newPath, []byte("extra"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(newPath) })
		if reflect.DeepEqual(base, snapshotDir(t, dir)) {
			t.Fatal("snapshot did not detect a file creation")
		}
	})
}

// TestSnapshotDirDetectsAddedEmptyDirectory proves the helper notices a new
// empty directory even though it has no content of its own to compare.
func TestSnapshotDirDetectsAddedEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	before := snapshotDir(t, dir)

	if err := os.Mkdir(filepath.Join(dir, "newdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	after := snapshotDir(t, dir)

	if reflect.DeepEqual(before, after) {
		t.Fatal("snapshot did not detect an added empty directory")
	}
	if _, ok := after["newdir"]; !ok {
		t.Fatalf("snapshot is missing the newdir entry: %v", after)
	}
}

// TestSnapshotDirDetectsFileModeChange proves the helper notices a
// permission-bit change on an otherwise-untouched file.
func TestSnapshotDirDetectsFileModeChange(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningfully enforced on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := snapshotDir(t, dir)

	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	after := snapshotDir(t, dir)

	if reflect.DeepEqual(before, after) {
		t.Fatal("snapshot did not detect a file mode change")
	}
}

// TestSnapshotDirDetectsSymlinkTargetChange proves the helper records a
// symlink's own target path rather than following the link and recording
// what it points to. Both targets hold byte-identical file content on
// purpose: an implementation that followed the link and captured target
// content (instead of the link itself) would see identical content before
// and after the retarget and would incorrectly report no change — so this
// test only passes for an implementation that genuinely inspects the
// symlink, not what it resolves to.
func TestSnapshotDirDetectsSymlinkTargetChange(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}
	dir := t.TempDir()
	targetA := filepath.Join(dir, "a.txt")
	targetB := filepath.Join(dir, "b.txt")
	const identicalContent = "identical content\n"
	if err := os.WriteFile(targetA, []byte(identicalContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetB, []byte(identicalContent), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(targetA, link); err != nil {
		t.Skipf("symlinks unsupported in this environment: %v", err)
	}
	before := snapshotDir(t, dir)

	beforeLink, ok := before["link"]
	if !ok {
		t.Fatalf("snapshot is missing the link entry: %v", before)
	}
	if beforeLink.Mode&os.ModeSymlink == 0 {
		t.Fatalf("link entry is not recorded as a symlink: %#v", beforeLink)
	}
	if beforeLink.LinkTarget != targetA {
		t.Fatalf("LinkTarget = %q, want %q", beforeLink.LinkTarget, targetA)
	}

	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetB, link); err != nil {
		t.Fatal(err)
	}
	after := snapshotDir(t, dir)

	afterLink, ok := after["link"]
	if !ok {
		t.Fatalf("snapshot is missing the link entry after retargeting: %v", after)
	}
	if afterLink.Mode&os.ModeSymlink == 0 {
		t.Fatalf("retargeted link entry is not recorded as a symlink: %#v", afterLink)
	}
	if afterLink.LinkTarget != targetB {
		t.Fatalf("LinkTarget = %q, want %q", afterLink.LinkTarget, targetB)
	}
	if afterLink.LinkTarget == beforeLink.LinkTarget {
		t.Fatalf("LinkTarget did not change: still %q", afterLink.LinkTarget)
	}

	if reflect.DeepEqual(before, after) {
		t.Fatal("snapshot did not detect a symlink target change even though LinkTarget differs " +
			"(targets hold identical content, so a link-following implementation would incorrectly report no change)")
	}
}

// decodeExactlyOneJSONObjectOrError parses data as exactly one JSON object
// and returns an error unless the stream contains that one value followed by
// nothing but optional insignificant whitespace. Decoder.More() is
// deliberately not used here: it only reports whether another token is
// available to read, which is a weaker property than proving the stream is
// actually exhausted — a second Decode call is the only way to force the
// decoder to confirm io.EOF (or surface a decode error for malformed
// trailing content such as a stray "}"/"]" or extra non-whitespace text).
func decodeExactlyOneJSONObjectOrError(data []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var got map[string]any
	if err := decoder.Decode(&got); err != nil {
		return nil, fmt.Errorf("decode JSON object: %w", err)
	}
	var trailing any
	switch err := decoder.Decode(&trailing); {
	case err == io.EOF:
		return got, nil
	case err == nil:
		return nil, fmt.Errorf("stdout contains more than one JSON value")
	default:
		return nil, fmt.Errorf("stdout contains trailing content after one JSON value: %w", err)
	}
}

// decodeExactlyOneJSONObject decodes data as exactly one JSON object and
// fails the test if there is any trailing value after it, proving stdout
// carries one isolated report rather than extra or partial content.
func decodeExactlyOneJSONObject(t *testing.T, data []byte) map[string]any {
	t.Helper()
	got, err := decodeExactlyOneJSONObjectOrError(data)
	if err != nil {
		t.Fatalf("%v: %q", err, data)
	}
	return got
}

// TestDecodeExactlyOneJSONObjectHelper is a focused self-test of the helper
// itself: it must accept exactly one JSON value (optionally followed only by
// insignificant whitespace) and reject every other shape of trailing content.
func TestDecodeExactlyOneJSONObjectHelper(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"single object", `{"a":1}`, false},
		{"single object followed by trailing whitespace", "{\"a\":1}\n  \t\n", false},
		{"object followed by another object", `{"a":1}{"b":2}`, true},
		{"object followed by a scalar", `{"a":1}42`, true},
		{"object followed by a stray close-brace", `{"a":1}}`, true},
		{"object followed by a stray close-bracket", `{"a":1}]`, true},
		{"object followed by non-whitespace text", `{"a":1}garbage`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeExactlyOneJSONObjectOrError([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Fatalf("decodeExactlyOneJSONObjectOrError(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

// assertExactJSONFields fails the test unless got has exactly the keys and
// values in want: no missing key, no extra key, no wrong value.
func assertExactJSONFields(t *testing.T, got map[string]any, want map[string]string) {
	t.Helper()
	wantAny := make(map[string]any, len(want))
	for k, v := range want {
		wantAny[k] = v
	}
	if !reflect.DeepEqual(got, wantAny) {
		t.Fatalf("JSON fields = %#v, want %#v", got, wantAny)
	}
}

// assertExactlyOneTextLine fails the test unless stdout is exactly one
// newline-terminated line.
func assertExactlyOneTextLine(t *testing.T, stdout string) {
	t.Helper()
	if strings.Count(stdout, "\n") != 1 || !strings.HasSuffix(stdout, "\n") {
		t.Fatalf("stdout is not exactly one line: %q", stdout)
	}
}

func agentSelectExecutorBothAvailable() *listingExecutor {
	return &listingExecutor{paths: map[string]string{"claude": "/tools/claude", "codex": "/tools/codex"}}
}

func agentSelectExecutorCodexUnavailable() *listingExecutor {
	return &listingExecutor{paths: map[string]string{"claude": "/tools/claude"}}
}

func TestRunAgentSelectRoleSelectedAcrossAllFiveRoles(t *testing.T) {
	tests := []struct {
		role      string
		wantAgent string
	}{
		{"specification", "claude-code"},
		{"planning", "claude-code"},
		{"implementation", "codex-cli"},
		{"review", "claude-code"},
		{"remediation", "codex-cli"},
	}
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			executor := agentSelectExecutorBothAvailable()
			wantText := "ai-dev: outcome=role_selected role=" + tt.role + " agent=" + tt.wantAgent + " source=built_in\n"

			var firstOut, firstErr bytes.Buffer
			code := Run(context.Background(), []string{"agent", "select", tt.role}, executor, nil, readyFS(), bytes.NewReader(nil), &firstOut, &firstErr)
			if code != 0 {
				t.Fatalf("text exit code = %d, want 0; stderr=%q", code, firstErr.String())
			}
			if firstOut.String() != wantText {
				t.Fatalf("stdout = %q, want %q", firstOut.String(), wantText)
			}
			assertExactlyOneTextLine(t, firstOut.String())
			if firstErr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", firstErr.String())
			}

			var secondOut, secondErr bytes.Buffer
			code = Run(context.Background(), []string{"agent", "select", tt.role}, executor, nil, readyFS(), bytes.NewReader(nil), &secondOut, &secondErr)
			if code != 0 || secondOut.String() != firstOut.String() || secondErr.Len() != 0 {
				t.Fatalf("repeated call changed output: code=%d first=%q second=%q secondErr=%q", code, firstOut.String(), secondOut.String(), secondErr.String())
			}

			var jsonOut, jsonErr bytes.Buffer
			code = Run(context.Background(), []string{"agent", "select", "--format", "json", tt.role}, executor, nil, readyFS(), bytes.NewReader(nil), &jsonOut, &jsonErr)
			if code != 0 {
				t.Fatalf("json exit code = %d, want 0; stderr=%q", code, jsonErr.String())
			}
			if jsonErr.Len() != 0 {
				t.Fatalf("json stderr = %q, want empty", jsonErr.String())
			}
			got := decodeExactlyOneJSONObject(t, jsonOut.Bytes())
			assertExactJSONFields(t, got, map[string]string{
				"supplied_role":     tt.role,
				"requested_role":    tt.role,
				"selected_agent_id": tt.wantAgent,
				"outcome":           "role_selected",
				"source":            "built_in",
			})
		})
	}
}

func TestRunAgentSelectRoleSelectedDoesNotMutateFilesystem(t *testing.T) {
	dir := controlledTempDir(t)
	before := snapshotDir(t, dir)

	registry := agent.NewRegistry(agentSelectExecutorBothAvailable())
	var stdout, stderr bytes.Buffer
	if code := RunAgentSelect([]string{"specification"}, registry, fstest.MapFS{}, &stdout, &stderr); code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}

	after := snapshotDir(t, dir)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("filesystem changed: before=%v after=%v", before, after)
	}
}

func TestRunAgentSelectRoleUnsupported(t *testing.T) {
	tests := []struct {
		name         string
		suppliedRole string
	}{
		{"wrong case", "Specification"},
		{"nonsense", "nonsense"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := agentSelectExecutorBothAvailable()

			var textOut, textErr bytes.Buffer
			code := Run(context.Background(), []string{"agent", "select", tt.suppliedRole}, executor, nil, readyFS(), bytes.NewReader(nil), &textOut, &textErr)
			if code != 1 {
				t.Fatalf("text exit code = %d, want 1; stderr=%q", code, textErr.String())
			}
			wantText := "ai-dev: outcome=role_unsupported role=" + strconv.Quote(tt.suppliedRole) + " source=built_in\n"
			if textOut.String() != wantText {
				t.Fatalf("stdout = %q, want %q", textOut.String(), wantText)
			}
			assertExactlyOneTextLine(t, textOut.String())
			if textErr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", textErr.String())
			}

			var jsonOut, jsonErr bytes.Buffer
			code = Run(context.Background(), []string{"agent", "select", "--format", "json", tt.suppliedRole}, executor, nil, readyFS(), bytes.NewReader(nil), &jsonOut, &jsonErr)
			if code != 1 {
				t.Fatalf("json exit code = %d, want 1; stderr=%q", code, jsonErr.String())
			}
			if jsonErr.Len() != 0 {
				t.Fatalf("json stderr = %q, want empty", jsonErr.String())
			}
			got := decodeExactlyOneJSONObject(t, jsonOut.Bytes())
			assertExactJSONFields(t, got, map[string]string{
				"supplied_role": tt.suppliedRole,
				"outcome":       "role_unsupported",
				"source":        "built_in",
			})

			if len(executor.lookups) != 0 {
				t.Fatalf("LookPath calls = %v, want none", executor.lookups)
			}
		})
	}
}

func TestRunAgentSelectUsageFailuresAreDistinctFromRoleUnsupported(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"missing role", []string{"agent", "select"}},
		{"explicit empty role", []string{"agent", "select", ""}},
		{"whitespace-only role", []string{"agent", "select", "   "}},
		{"extra positional", []string{"agent", "select", "implementation", "extra"}},
		{"unknown flag", []string{"agent", "select", "--bogus", "implementation"}},
		{"invalid format value", []string{"agent", "select", "--format", "yaml", "implementation"}},
		{"format flag missing value", []string{"agent", "select", "--format"}},
	}
	const wantUsage = "Usage: ai-dev agent select [--format text|json] <role>\n"
	sharedLauncher := &appRunLauncher{status: system.ExitStatus{Exited: true, ExitCode: 0}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := agentSelectExecutorBothAvailable()
			var stdout, stderr bytes.Buffer
			code := Run(context.Background(), tt.args, executor, sharedLauncher, readyFS(), bytes.NewReader(nil), &stdout, &stderr)
			if code != 2 {
				t.Fatalf("code = %d, want 2; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
			if stderr.String() != wantUsage {
				t.Fatalf("stderr = %q, want exactly %q", stderr.String(), wantUsage)
			}
			if strings.Contains(stdout.String(), "outcome=") || strings.Contains(stderr.String(), "outcome=") ||
				strings.Contains(stdout.String(), `"outcome"`) || strings.Contains(stderr.String(), `"outcome"`) {
				t.Fatalf("usage failure emitted a selection report: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			if len(executor.lookups) != 0 {
				t.Fatalf("usage failure performed registry discovery: %v", executor.lookups)
			}
		})
	}
	if sharedLauncher.launches != 0 {
		t.Fatalf("usage failures launched %d processes", sharedLauncher.launches)
	}
}

func TestRunAgentSelectRoleUnsupportedAndUsageFailureDoNotMutateFilesystem(t *testing.T) {
	t.Run("role_unsupported", func(t *testing.T) {
		dir := controlledTempDir(t)
		before := snapshotDir(t, dir)
		registry := agent.NewRegistry(agentSelectExecutorBothAvailable())
		var stdout, stderr bytes.Buffer
		if code := RunAgentSelect([]string{"nonsense"}, registry, fstest.MapFS{}, &stdout, &stderr); code != 1 {
			t.Fatalf("code = %d, stderr = %q", code, stderr.String())
		}
		after := snapshotDir(t, dir)
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("filesystem changed: before=%v after=%v", before, after)
		}
	})
	t.Run("usage failure", func(t *testing.T) {
		dir := controlledTempDir(t)
		before := snapshotDir(t, dir)
		registry := agent.NewRegistry(agentSelectExecutorBothAvailable())
		var stdout, stderr bytes.Buffer
		if code := RunAgentSelect([]string{""}, registry, fstest.MapFS{}, &stdout, &stderr); code != 2 {
			t.Fatalf("code = %d, stderr = %q", code, stderr.String())
		}
		after := snapshotDir(t, dir)
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("filesystem changed: before=%v after=%v", before, after)
		}
	})
}

func TestRunAgentSelectAgentUnavailable(t *testing.T) {
	t.Run("text", func(t *testing.T) {
		executor := agentSelectExecutorCodexUnavailable()
		var stdout, stderr bytes.Buffer
		code := Run(context.Background(), []string{"agent", "select", "implementation"}, executor, nil, readyFS(), bytes.NewReader(nil), &stdout, &stderr)
		if code != 1 {
			t.Fatalf("exit code = %d, want 1; stderr=%q", code, stderr.String())
		}
		wantText := "ai-dev: outcome=agent_unavailable role=implementation agent=codex-cli source=built_in\n"
		if stdout.String() != wantText {
			t.Fatalf("stdout = %q, want %q", stdout.String(), wantText)
		}
		assertExactlyOneTextLine(t, stdout.String())
		if stderr.Len() != 0 {
			t.Fatalf("stderr = %q, want empty", stderr.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		executor := agentSelectExecutorCodexUnavailable()
		var stdout, stderr bytes.Buffer
		code := Run(context.Background(), []string{"agent", "select", "--format", "json", "implementation"}, executor, nil, readyFS(), bytes.NewReader(nil), &stdout, &stderr)
		if code != 1 {
			t.Fatalf("exit code = %d, want 1; stderr=%q", code, stderr.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("stderr = %q, want empty", stderr.String())
		}
		got := decodeExactlyOneJSONObject(t, stdout.Bytes())
		assertExactJSONFields(t, got, map[string]string{
			"supplied_role":   "implementation",
			"requested_role":  "implementation",
			"mapped_agent_id": "codex-cli",
			"outcome":         "agent_unavailable",
			"source":          "built_in",
		})
	})
}

func TestRunAgentSelectSelectionInconsistentTextMode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunAgentSelect([]string{"implementation"}, agent.Registry{}, fstest.MapFS{}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	want := "ai-dev: outcome=selection_inconsistent role=implementation agent=codex-cli source=built_in\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	assertExactlyOneTextLine(t, stdout.String())
}

func TestRunAgentSelectSelectionInconsistentJSONMode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunAgentSelect([]string{"--format", "json", "implementation"}, agent.Registry{}, fstest.MapFS{}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	got := decodeExactlyOneJSONObject(t, stdout.Bytes())
	assertExactJSONFields(t, got, map[string]string{
		"supplied_role":   "implementation",
		"requested_role":  "implementation",
		"mapped_agent_id": "codex-cli",
		"outcome":         "selection_inconsistent",
		"source":          "built_in",
	})
}

func TestNoFailureOutcomeResemblesRoleSelected(t *testing.T) {
	assertNoRoleSelected := func(t *testing.T, stdout, stderr string) {
		t.Helper()
		if strings.Contains(stdout, "outcome=role_selected") || strings.Contains(stderr, "outcome=role_selected") ||
			strings.Contains(stdout, `"outcome":"role_selected"`) || strings.Contains(stderr, `"outcome":"role_selected"`) {
			t.Fatalf("failure outcome resembles role_selected: stdout=%q stderr=%q", stdout, stderr)
		}
		if strings.Contains(stdout, "selected_agent_id") {
			t.Fatalf("failure outcome carries a selected-agent identity: stdout=%q", stdout)
		}
	}

	t.Run("role_unsupported", func(t *testing.T) {
		registry := agent.NewRegistry(agentSelectExecutorBothAvailable())
		var stdout, stderr bytes.Buffer
		RunAgentSelect([]string{"--format", "json", "nonsense"}, registry, fstest.MapFS{}, &stdout, &stderr)
		assertNoRoleSelected(t, stdout.String(), stderr.String())
	})
	t.Run("agent_unavailable", func(t *testing.T) {
		registry := agent.NewRegistry(agentSelectExecutorCodexUnavailable())
		var stdout, stderr bytes.Buffer
		RunAgentSelect([]string{"--format", "json", "implementation"}, registry, fstest.MapFS{}, &stdout, &stderr)
		assertNoRoleSelected(t, stdout.String(), stderr.String())
	})
	t.Run("selection_inconsistent", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		RunAgentSelect([]string{"--format", "json", "implementation"}, agent.Registry{}, fstest.MapFS{}, &stdout, &stderr)
		assertNoRoleSelected(t, stdout.String(), stderr.String())
	})
}

func TestNoFurtherAutomaticActionAfterNonSelectedOutcome(t *testing.T) {
	t.Run("role_unsupported performs zero registry access", func(t *testing.T) {
		executor := agentSelectExecutorBothAvailable()
		registry := agent.NewRegistry(executor)
		var stdout, stderr bytes.Buffer
		RunAgentSelect([]string{"nonsense"}, registry, fstest.MapFS{}, &stdout, &stderr)
		if len(executor.lookups) != 0 {
			t.Fatalf("LookPath calls = %v, want none", executor.lookups)
		}
	})
	t.Run("agent_unavailable performs exactly one complete scan", func(t *testing.T) {
		executor := agentSelectExecutorCodexUnavailable()
		registry := agent.NewRegistry(executor)
		var stdout, stderr bytes.Buffer
		RunAgentSelect([]string{"implementation"}, registry, fstest.MapFS{}, &stdout, &stderr)
		if want := catalogExecutables; !reflect.DeepEqual(executor.lookups, want) {
			t.Fatalf("LookPath calls = %v, want %v", executor.lookups, want)
		}
	})
	t.Run("role_selected performs exactly one complete scan", func(t *testing.T) {
		executor := agentSelectExecutorBothAvailable()
		registry := agent.NewRegistry(executor)
		var stdout, stderr bytes.Buffer
		RunAgentSelect([]string{"specification"}, registry, fstest.MapFS{}, &stdout, &stderr)
		if want := catalogExecutables; !reflect.DeepEqual(executor.lookups, want) {
			t.Fatalf("LookPath calls = %v, want %v", executor.lookups, want)
		}
	})
	t.Run("selection_inconsistent never dereferences an executor", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := RunAgentSelect([]string{"implementation"}, agent.Registry{}, fstest.MapFS{}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("code = %d, want 1 (call completing without panic proves no LookPath was attempted)", code)
		}
	})
}

func TestRunAgentSelectReportWriteFailureIsNotReclassified(t *testing.T) {
	registry := agent.NewRegistry(agentSelectExecutorBothAvailable())
	var stderr bytes.Buffer
	code := RunAgentSelect([]string{"specification"}, registry, fstest.MapFS{}, errWriter{}, &stderr)
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if stderr.Len() == 0 {
		t.Fatalf("stderr is empty, want a diagnostic")
	}
	if strings.Contains(stderr.String(), "outcome=") || strings.Contains(stderr.String(), `"outcome"`) {
		t.Fatalf("write failure was reclassified as an outcome: %q", stderr.String())
	}
	if strings.Contains(stderr.String(), "selection_inconsistent") {
		t.Fatalf("write failure was reclassified as selection_inconsistent: %q", stderr.String())
	}
}

func TestRunAgentSelectNeverLaunchesOrExecutes(t *testing.T) {
	usageAndOutcomeCases := [][]string{
		{"agent", "select", "specification"},
		{"agent", "select", "nonsense"},
		{"agent", "select", "implementation"},
		{"agent", "select"},
		{"agent", "select", ""},
		{"agent", "select", "   "},
		{"agent", "select", "implementation", "extra"},
		{"agent", "select", "--bogus", "implementation"},
		{"agent", "select", "--format", "yaml", "implementation"},
		{"agent", "select", "--format"},
	}

	t.Run("system.Launcher.Launch stays zero", func(t *testing.T) {
		launcher := &appRunLauncher{status: system.ExitStatus{Exited: true, ExitCode: 0}}
		executor := &listingExecutor{paths: map[string]string{"claude": "/tools/claude"}}
		for _, args := range usageAndOutcomeCases {
			Run(context.Background(), args, executor, launcher, readyFS(), bytes.NewReader(nil), io.Discard, io.Discard)
		}
		if launcher.launches != 0 {
			t.Fatalf("agent select launched %d processes", launcher.launches)
		}
		var stdout, stderr bytes.Buffer
		code := Run(context.Background(), []string{"agent", "run", "claude-code", "do it"}, executor, launcher, readyFS(), bytes.NewReader(nil), &stdout, &stderr)
		if code != 0 || launcher.launches != 1 {
			t.Fatalf("positive control failed: code=%d launches=%d stderr=%q", code, launcher.launches, stderr.String())
		}
	})

	t.Run("system.Executor.Run stays zero", func(t *testing.T) {
		executor := readyExecutor()
		for _, args := range usageAndOutcomeCases {
			var stdout, stderr bytes.Buffer
			Run(context.Background(), args, executor, nil, readyFS(), bytes.NewReader(nil), &stdout, &stderr)
		}
		if executor.runs != 0 {
			t.Fatalf("agent select called Executor.Run %d times", executor.runs)
		}
		var stdout, stderr bytes.Buffer
		code := Run(context.Background(), []string{"doctor"}, executor, nil, readyFS(), bytes.NewReader(nil), &stdout, &stderr)
		if code != 0 || executor.runs == 0 {
			t.Fatalf("positive control failed: code=%d runs=%d stderr=%q", code, executor.runs, stderr.String())
		}
	})
}

func TestRunAgentSelectAgentUnavailableDoesNotMutateFilesystem(t *testing.T) {
	dir := controlledTempDir(t)
	before := snapshotDir(t, dir)
	registry := agent.NewRegistry(agentSelectExecutorCodexUnavailable())
	var stdout, stderr bytes.Buffer
	if code := RunAgentSelect([]string{"implementation"}, registry, fstest.MapFS{}, &stdout, &stderr); code != 1 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	after := snapshotDir(t, dir)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("filesystem changed: before=%v after=%v", before, after)
	}
}

// F011-FR-015 to FR-017: agent select follows the project assignment and
// states its source; an invalid configuration never falls back.
func TestAgentSelectFollowsTheProjectAssignment(t *testing.T) {
	both := &setupExecutor{paths: map[string]string{"claude": "/tools/claude", "codex": "/tools/codex"}}
	for _, test := range []struct {
		name string
		root fstest.MapFS
		code int
		want string
	}{
		{"no configuration", fstest.MapFS{}, 0, "ai-dev: outcome=role_selected role=implementation agent=codex-cli source=built_in\n"},
		{"project", configFS(projectConfigImplementationClaude), 0, "ai-dev: outcome=role_selected role=implementation agent=claude-code source=project\n"},
		{"invalid", configFS("{}"), 1, "ai-dev: outcome=configuration_invalid role=implementation source=project\n"},
	} {
		var stdout, stderr bytes.Buffer
		code := RunWithDependencies(context.Background(), []string{"agent", "select", "implementation"}, Dependencies{Executor: both, Root: test.root, Stdout: &stdout, Stderr: &stderr})
		if code != test.code || stdout.String() != test.want || stderr.Len() != 0 {
			t.Errorf("%s: code=%d stdout=%q stderr=%q", test.name, code, stdout.String(), stderr.String())
		}
	}
}

// SC-004: every invalid configuration, for every role, is
// configuration_invalid and names no agent.
func TestAgentSelectEveryInvalidConfiguration(t *testing.T) {
	roles := `"roles":{"specification":"claude-code","planning":"claude-code","implementation":"codex-cli","review":"claude-code","remediation":"codex-cli"}`
	head := `"schema_version":"ai-dev.agents/v1","agents":["claude-code","codex-cli"]`
	invalid := map[string]fstest.MapFS{
		"directory is a file": {".ai-dev": &fstest.MapFile{Data: []byte("x")}},
		"file is a symlink":   {".ai-dev/agents.json": &fstest.MapFile{Data: []byte("../elsewhere.json"), Mode: fs.ModeSymlink}, "elsewhere.json": &fstest.MapFile{Data: []byte("{" + head + "," + roles + "}")}},
		"too large":           configFS(strings.Repeat(" ", 65537)),
		"malformed":           configFS("{"),
		"trailing data":       configFS("{" + head + "," + roles + "}{}"),
		"duplicate key":       configFS("{" + head + `,"agents":["claude-code","codex-cli"],` + roles + "}"),
		"unknown field":       configFS("{" + head + "," + roles + `,"command":"x"}`),
		"missing field":       configFS(`{"schema_version":"ai-dev.agents/v1"}`),
		"wrong type":          configFS(`{"schema_version":"ai-dev.agents/v1","agents":null,` + roles + "}"),
		"unsupported schema":  configFS(`{"schema_version":"v2","agents":["claude-code","codex-cli"],` + roles + "}"),
		"no agents":           configFS(`{"schema_version":"ai-dev.agents/v1","agents":[],` + roles + "}"),
		"duplicate agent":     configFS(`{"schema_version":"ai-dev.agents/v1","agents":["claude-code","codex-cli","codex-cli"],` + roles + "}"),
		"unknown agent":       configFS(`{"schema_version":"ai-dev.agents/v1","agents":["claude-code","codex-cli","gemini"],` + roles + "}"),
		"unknown role":        configFS("{" + head + `,"roles":{"specification":"claude-code","planning":"claude-code","implementation":"codex-cli","review":"claude-code","remediation":"codex-cli","deploy":"codex-cli"}}`),
		"missing role":        configFS("{" + head + `,"roles":{"specification":"claude-code"}}`),
		"agent not chosen":    configFS(`{"schema_version":"ai-dev.agents/v1","agents":["claude-code"],` + roles + "}"),
	}
	both := &setupExecutor{paths: map[string]string{"claude": "/tools/claude", "codex": "/tools/codex"}}
	for name, root := range invalid {
		for _, r := range []string{"specification", "planning", "implementation", "review", "remediation"} {
			var stdout, stderr bytes.Buffer
			code := RunWithDependencies(context.Background(), []string{"agent", "select", r}, Dependencies{Executor: both, Root: root, Stdout: &stdout, Stderr: &stderr})
			if code != 1 || stdout.String() != "ai-dev: outcome=configuration_invalid role="+r+" source=project\n" {
				t.Errorf("%s/%s: code=%d stdout=%q stderr=%q", name, r, code, stdout.String(), stderr.String())
			}
		}
	}
}
