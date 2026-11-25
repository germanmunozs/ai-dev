package app

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

const workflowTestFeature = "004-workflow-state"

// recordingRootFS records every fs.FS and fs.ReadLinkFS method call. It
// implements and forwards fs.ReadLinkFS, so production code never rejects it
// for lacking no-follow capability: a zero count genuinely means no access.
type recordingRootFS struct {
	inner fs.ReadLinkFS
	calls []string
}

func (r *recordingRootFS) Open(name string) (fs.File, error) {
	r.calls = append(r.calls, "Open:"+name)
	return r.inner.Open(name)
}

func (r *recordingRootFS) Lstat(name string) (fs.FileInfo, error) {
	r.calls = append(r.calls, "Lstat:"+name)
	return r.inner.Lstat(name)
}

func (r *recordingRootFS) ReadLink(name string) (string, error) {
	r.calls = append(r.calls, "ReadLink:"+name)
	return r.inner.ReadLink(name)
}

// workflowFixture returns an fstest.MapFS with a real feature directory and,
// when record is non-nil, a Workflow State Record with that content.
func workflowFixture(record *string) fstest.MapFS {
	fsys := fstest.MapFS{
		"specs/" + workflowTestFeature: &fstest.MapFile{Mode: fs.ModeDir | 0o755},
	}
	if record != nil {
		fsys["specs/"+workflowTestFeature+"/workflow-state.json"] = &fstest.MapFile{Data: []byte(*record)}
	}
	return fsys
}

func strPtr(s string) *string { return &s }

// workflowTempRoot builds a controlled temporary tree containing one real
// feature directory and, when record is non-nil, its Workflow State Record.
func workflowTempRoot(t *testing.T, record *string) string {
	t.Helper()
	dir := controlledTempDir(t)
	featureDir := filepath.Join(dir, "specs", workflowTestFeature)
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if record != nil {
		if err := os.WriteFile(filepath.Join(featureDir, "workflow-state.json"), []byte(*record), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// assertExactWorkflowJSONFields fails unless got has exactly the keys in
// want with identical typed values; unlike assertExactJSONFields it supports
// non-string values such as JSON booleans.
func assertExactWorkflowJSONFields(t *testing.T, got, want map[string]any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON fields = %#v, want %#v", got, want)
	}
}

func TestRunWorkflowStateUsageFailures(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"no feature id", []string{}},
		{"whitespace-only feature id", []string{"   "}},
		{"extra positional", []string{workflowTestFeature, "extra"}},
		{"unknown flag", []string{"--verbose", workflowTestFeature}},
		{"unsupported format", []string{"--format", "xml", workflowTestFeature}},
		{"format flag missing value", []string{"--format"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := &recordingRootFS{inner: workflowFixture(strPtr(`{"state":"planned"}`))}
			var stdout, stderr bytes.Buffer
			code := RunWorkflowState(tt.args, root, &stdout, &stderr)
			if code != 2 {
				t.Fatalf("exit = %d, want 2", code)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
			if stderr.String() != "Usage: ai-dev workflow state [--format text|json] <feature-id>\n" {
				t.Fatalf("stderr = %q, want usage", stderr.String())
			}
			if strings.Contains(stderr.String(), "outcome") {
				t.Fatalf("stderr contains a semantic report: %q", stderr.String())
			}
			if len(root.calls) != 0 {
				t.Fatalf("usage failure accessed the filesystem: %v", root.calls)
			}
		})
	}

	t.Run("positive control records access", func(t *testing.T) {
		root := &recordingRootFS{inner: workflowFixture(strPtr(`{"state":"planned"}`))}
		var stdout, stderr bytes.Buffer
		if code := RunWorkflowState([]string{workflowTestFeature}, root, &stdout, &stderr); code != 0 {
			t.Fatalf("exit = %d, want 0 (stderr %q)", code, stderr.String())
		}
		if len(root.calls) == 0 {
			t.Fatal("recording wrapper observed no access for a valid request")
		}
	})
}

func TestRunWorkflowStateOutcomesTextAndJSON(t *testing.T) {
	tests := []struct {
		name      string
		featureID string
		record    *string
		wantCode  int
		wantText  string
		wantJSON  map[string]any
	}{
		{"state_reported", workflowTestFeature, strPtr(`{"state":"planned"}`), 0,
			"ai-dev: outcome=state_reported feature=\"004-workflow-state\" state=planned terminal=false\n",
			map[string]any{"feature_id": workflowTestFeature, "state": "planned", "terminal": false, "outcome": "state_reported"}},
		{"state_reported default draft", workflowTestFeature, nil, 0,
			"ai-dev: outcome=state_reported feature=\"004-workflow-state\" state=draft terminal=false\n",
			map[string]any{"feature_id": workflowTestFeature, "state": "draft", "terminal": false, "outcome": "state_reported"}},
		{"state_reported terminal", workflowTestFeature, strPtr(`{"state":"approved"}`), 0,
			"ai-dev: outcome=state_reported feature=\"004-workflow-state\" state=approved terminal=true\n",
			map[string]any{"feature_id": workflowTestFeature, "state": "approved", "terminal": true, "outcome": "state_reported"}},
		{"feature_unknown", "does-not-exist", nil, 1,
			"ai-dev: outcome=feature_unknown feature=\"does-not-exist\"\n",
			map[string]any{"feature_id": "does-not-exist", "outcome": "feature_unknown"}},
		{"feature_unknown traversal", "../x", nil, 1,
			"ai-dev: outcome=feature_unknown feature=\"../x\"\n",
			map[string]any{"feature_id": "../x", "outcome": "feature_unknown"}},
		{"state_invalid", workflowTestFeature, strPtr(`{"state":"Planned"}`), 1,
			"ai-dev: outcome=state_invalid feature=\"004-workflow-state\"\n",
			map[string]any{"feature_id": workflowTestFeature, "outcome": "state_invalid"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := workflowFixture(tt.record)

			var stdout, stderr bytes.Buffer
			if code := RunWorkflowState([]string{tt.featureID}, fixture, &stdout, &stderr); code != tt.wantCode {
				t.Fatalf("text exit = %d, want %d", code, tt.wantCode)
			}
			if stdout.String() != tt.wantText || stderr.Len() != 0 {
				t.Fatalf("text stdout=%q stderr=%q, want stdout=%q and empty stderr", stdout.String(), stderr.String(), tt.wantText)
			}
			assertExactlyOneTextLine(t, stdout.String())

			stdout.Reset()
			stderr.Reset()
			if code := RunWorkflowState([]string{"--format", "json", tt.featureID}, fixture, &stdout, &stderr); code != tt.wantCode {
				t.Fatalf("json exit = %d, want %d", code, tt.wantCode)
			}
			if stderr.Len() != 0 {
				t.Fatalf("json stderr = %q, want empty", stderr.String())
			}
			assertExactWorkflowJSONFields(t, decodeExactlyOneJSONObject(t, stdout.Bytes()), tt.wantJSON)
		})
	}
}

func assertWorkflowStateDoesNotMutate(t *testing.T, record *string, featureID string, wantCode int) {
	t.Helper()
	dir := workflowTempRoot(t, record)
	before := snapshotDir(t, dir)
	for _, format := range []string{"text", "json"} {
		var stdout, stderr bytes.Buffer
		if code := RunWorkflowState([]string{"--format", format, featureID}, os.DirFS(dir), &stdout, &stderr); code != wantCode {
			t.Fatalf("%s exit = %d, want %d (stderr %q)", format, code, wantCode, stderr.String())
		}
	}
	if after := snapshotDir(t, dir); !reflect.DeepEqual(before, after) {
		t.Fatalf("filesystem changed:\nbefore=%v\nafter=%v", before, after)
	}
}

func TestRunWorkflowStateStateReportedDoesNotMutateFilesystem(t *testing.T) {
	assertWorkflowStateDoesNotMutate(t, strPtr(`{"state":"planned"}`), workflowTestFeature, 0)
}

func TestRunWorkflowStateFeatureUnknownDoesNotMutateFilesystem(t *testing.T) {
	assertWorkflowStateDoesNotMutate(t, nil, "does-not-exist", 1)
}

func TestRunWorkflowStateStateInvalidDoesNotMutateFilesystem(t *testing.T) {
	assertWorkflowStateDoesNotMutate(t, strPtr(`not json`), workflowTestFeature, 1)
}

func TestRunWorkflowStateIsDeterministic(t *testing.T) {
	fixture := workflowFixture(strPtr(`{"state":"tasked"}`))
	for _, format := range []string{"text", "json"} {
		var first, second, stderr bytes.Buffer
		RunWorkflowState([]string{"--format", format, workflowTestFeature}, fixture, &first, &stderr)
		RunWorkflowState([]string{"--format", format, workflowTestFeature}, fixture, &second, &stderr)
		if first.String() != second.String() {
			t.Fatalf("%s output differs: %q vs %q", format, first.String(), second.String())
		}
	}
}

func TestRunWorkflowStateWriteFailureIsNotReclassified(t *testing.T) {
	for _, format := range []string{"text", "json"} {
		var stderr bytes.Buffer
		code := RunWorkflowState([]string{"--format", format, workflowTestFeature}, workflowFixture(nil), errWriter{}, &stderr)
		if code != 2 {
			t.Fatalf("%s exit = %d, want 2", format, code)
		}
		if !strings.Contains(stderr.String(), "write failed") || strings.Contains(stderr.String(), "outcome=") {
			t.Fatalf("%s stderr = %q, want a write diagnostic without an outcome", format, stderr.String())
		}
	}
}

// --- Dispatcher-level assertions (T018) ---

func runDispatcher(t *testing.T, args []string, root fs.FS) (int, string, string, *fakeExecutor, *appRunLauncher) {
	t.Helper()
	exec := &fakeExecutor{}
	launcher := &appRunLauncher{}
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), args, exec, launcher, root, bytes.NewReader(nil), &stdout, &stderr)
	return code, stdout.String(), stderr.String(), exec, launcher
}

func TestRunDispatchesWorkflowState(t *testing.T) {
	code, stdout, stderr, exec, launcher := runDispatcher(t, []string{"workflow", "state", "--format", "json", workflowTestFeature}, workflowFixture(strPtr(`{"state":"implemented"}`)))
	if code != 0 || stderr != "" {
		t.Fatalf("exit=%d stderr=%q, want 0 and empty", code, stderr)
	}
	assertExactWorkflowJSONFields(t, decodeExactlyOneJSONObject(t, []byte(stdout)),
		map[string]any{"feature_id": workflowTestFeature, "state": "implemented", "terminal": false, "outcome": "state_reported"})
	if exec.lookups != 0 || exec.runs != 0 || launcher.launches != 0 {
		t.Fatalf("workflow state reached an executor or launcher: lookups=%d runs=%d launches=%d", exec.lookups, exec.runs, launcher.launches)
	}

	code, stdout, stderr, _, _ = runDispatcher(t, []string{"workflow", "state", "does-not-exist"}, workflowFixture(nil))
	if code != 1 || stderr != "" || stdout != "ai-dev: outcome=feature_unknown feature=\"does-not-exist\"\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestRunTopLevelUsageStringsAreIdenticalAndNameWorkflow(t *testing.T) {
	_, _, noArgs, _, _ := runDispatcher(t, nil, workflowFixture(nil))
	_, _, unknown, _, _ := runDispatcher(t, []string{"bogus"}, workflowFixture(nil))
	if noArgs != unknown {
		t.Fatalf("usage strings differ: %q vs %q", noArgs, unknown)
	}
	if noArgs != "Usage: ai-dev <doctor|agents|agent|workflow|init|git|setup|tui|open|version|help|completion> [options]\n" {
		t.Fatalf("usage = %q, want workflow listed", noArgs)
	}
}

func TestRunWorkflowDispatchUsageFailures(t *testing.T) {
	for _, args := range [][]string{{"workflow"}, {"workflow", "bogus"}} {
		root := &recordingRootFS{inner: workflowFixture(strPtr(`{"state":"planned"}`))}
		code, stdout, stderr, exec, launcher := runDispatcher(t, args, root)
		if code != 2 {
			t.Fatalf("%v: exit = %d, want 2", args, code)
		}
		if stdout != "" {
			t.Fatalf("%v: stdout = %q, want empty", args, stdout)
		}
		if stderr != "Usage: ai-dev workflow <state|transition|next|guide|approve|abandon> [options]\n" {
			t.Fatalf("%v: stderr = %q, want workflow usage", args, stderr)
		}
		if len(root.calls) != 0 {
			t.Fatalf("%v: accessed the filesystem: %v", args, root.calls)
		}
		if exec.lookups != 0 || exec.runs != 0 || launcher.launches != 0 {
			t.Fatalf("%v: reached an executor or launcher", args)
		}
	}
}

// --- US3 end-to-end failure outcomes through Run (T029) ---

var workflowSuccessMarkers = []string{
	"outcome=state_reported", "outcome=transition_valid",
	`"outcome":"state_reported"`, `"outcome":"transition_valid"`,
}

func assertNoWorkflowSuccessMarker(t *testing.T, output string) {
	t.Helper()
	for _, marker := range workflowSuccessMarkers {
		if strings.Contains(output, marker) {
			t.Fatalf("failure output contains success marker %q: %q", marker, output)
		}
	}
}

func TestRunWorkflowStateFailureOutcomesNeverResembleSuccess(t *testing.T) {
	cases := map[string]struct {
		root      fs.FS
		featureID string
		want      string
	}{
		"feature_unknown":           {workflowFixture(nil), "does-not-exist", "feature_unknown"},
		"state_invalid wrong case":  {workflowFixture(strPtr(`{"state":"Planned"}`)), workflowTestFeature, "state_invalid"},
		"state_invalid empty field": {workflowFixture(strPtr(`{"state":""}`)), workflowTestFeature, "state_invalid"},
		"state_invalid unparseable": {workflowFixture(strPtr(`{`)), workflowTestFeature, "state_invalid"},
	}
	for name, tc := range cases {
		for _, format := range []string{"text", "json"} {
			code, stdout, stderr, _, _ := runDispatcher(t, []string{"workflow", "state", "--format", format, tc.featureID}, tc.root)
			if code != 1 || stderr != "" {
				t.Fatalf("%s/%s: exit=%d stderr=%q, want 1 and empty", name, format, code, stderr)
			}
			if !strings.Contains(stdout, tc.want) {
				t.Fatalf("%s/%s: stdout %q lacks %q", name, format, stdout, tc.want)
			}
			assertNoWorkflowSuccessMarker(t, stdout)
		}
	}
}
