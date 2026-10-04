package app

import (
	"bytes"
	"io/fs"
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

const workflowTransitionUsage = "Usage: ai-dev workflow transition [--format text|json] <feature-id> <target-state>\n"

func assertWorkflowTransitionUsageFailure(t *testing.T, args []string) {
	t.Helper()
	root := &recordingRootFS{inner: workflowFixture(strPtr(`{"state":"planned"}`))}
	var stdout, stderr bytes.Buffer
	if code := RunWorkflowTransition(args, root, &stdout, &stderr); code != 2 {
		t.Fatalf("%q: exit = %d, want 2", args, code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("%q: stdout = %q, want empty", args, stdout.String())
	}
	if stderr.String() != workflowTransitionUsage {
		t.Fatalf("%q: stderr = %q, want usage", args, stderr.String())
	}
	if len(root.calls) != 0 {
		t.Fatalf("%q: usage failure accessed the filesystem: %v", args, root.calls)
	}
}

func TestRunWorkflowTransitionUsageFailures(t *testing.T) {
	for _, args := range [][]string{
		{},
		{workflowTestFeature},
		{"   ", "tasked"},
		{workflowTestFeature, "   "},
		{workflowTestFeature, "tasked", "extra"},
		{"--verbose", workflowTestFeature, "tasked"},
		{"--format", "xml", workflowTestFeature, "tasked"},
		{"--format"},
	} {
		assertWorkflowTransitionUsageFailure(t, args)
	}

	root := &recordingRootFS{inner: workflowFixture(strPtr(`{"state":"planned"}`))}
	var stdout, stderr bytes.Buffer
	if code := RunWorkflowTransition([]string{workflowTestFeature, "tasked"}, root, &stdout, &stderr); code != 0 {
		t.Fatalf("positive control exit = %d (stderr %q)", code, stderr.String())
	}
	if len(root.calls) == 0 {
		t.Fatal("recording wrapper observed no access for a valid request")
	}
}

// FR-029 step 1 (usage validation) precedes step 2 (candidate-target
// canonicality): a malformed request with a non-canonical target is a usage
// failure, never target_unsupported.
func TestRunWorkflowTransitionUsageFailureBeatsUnsupportedTarget(t *testing.T) {
	for _, args := range [][]string{
		{"--format", "xml", workflowTestFeature, "reviewing"},
		{workflowTestFeature, "reviewing", "extra"},
		{"   ", "reviewing"},
	} {
		assertWorkflowTransitionUsageFailure(t, args)
	}
}

func TestRunWorkflowTransitionOutcomesTextAndJSON(t *testing.T) {
	tests := []struct {
		name      string
		fixture   fstest.MapFS
		featureID string
		target    string
		wantCode  int
		wantText  string
		wantJSON  map[string]any
	}{
		{"transition_valid", workflowFixture(strPtr(`{"state":"planned"}`)), workflowTestFeature, "tasked", 0,
			"ai-dev: outcome=transition_valid feature=\"004-workflow-state\" current=planned target=tasked\n",
			map[string]any{"feature_id": workflowTestFeature, "current_state": "planned", "target": "tasked", "outcome": "transition_valid"}},
		{"transition_invalid", workflowFixture(strPtr(`{"state":"abandoned"}`)), workflowTestFeature, "specified", 1,
			"ai-dev: outcome=transition_invalid feature=\"004-workflow-state\" current=abandoned target=specified\n",
			map[string]any{"feature_id": workflowTestFeature, "current_state": "abandoned", "target": "specified", "outcome": "transition_invalid"}},
		{"target_unsupported", workflowFixture(strPtr(`{"state":"planned"}`)), workflowTestFeature, "reviewing", 1,
			"ai-dev: outcome=target_unsupported feature=\"004-workflow-state\" target=\"reviewing\"\n",
			map[string]any{"feature_id": workflowTestFeature, "supplied_target": "reviewing", "outcome": "target_unsupported"}},
		{"target_unsupported beats unknown feature", fstest.MapFS{}, "does-not-exist", "reviewing", 1,
			"ai-dev: outcome=target_unsupported feature=\"does-not-exist\" target=\"reviewing\"\n",
			map[string]any{"feature_id": "does-not-exist", "supplied_target": "reviewing", "outcome": "target_unsupported"}},
		{"target_unsupported beats malformed record", workflowFixture(strPtr(`not json`)), workflowTestFeature, "reviewing", 1,
			"ai-dev: outcome=target_unsupported feature=\"004-workflow-state\" target=\"reviewing\"\n",
			map[string]any{"feature_id": workflowTestFeature, "supplied_target": "reviewing", "outcome": "target_unsupported"}},
		{"feature_unknown", fstest.MapFS{}, "does-not-exist", "tasked", 1,
			"ai-dev: outcome=feature_unknown feature=\"does-not-exist\"\n",
			map[string]any{"feature_id": "does-not-exist", "outcome": "feature_unknown"}},
		{"state_invalid", workflowFixture(strPtr(`{"state":" planned "}`)), workflowTestFeature, "tasked", 1,
			"ai-dev: outcome=state_invalid feature=\"004-workflow-state\"\n",
			map[string]any{"feature_id": workflowTestFeature, "outcome": "state_invalid"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := RunWorkflowTransition([]string{tt.featureID, tt.target}, tt.fixture, &stdout, &stderr); code != tt.wantCode {
				t.Fatalf("text exit = %d, want %d", code, tt.wantCode)
			}
			if stdout.String() != tt.wantText || stderr.Len() != 0 {
				t.Fatalf("text stdout=%q stderr=%q, want stdout=%q and empty stderr", stdout.String(), stderr.String(), tt.wantText)
			}
			assertExactlyOneTextLine(t, stdout.String())

			stdout.Reset()
			stderr.Reset()
			if code := RunWorkflowTransition([]string{"--format", "json", tt.featureID, tt.target}, tt.fixture, &stdout, &stderr); code != tt.wantCode {
				t.Fatalf("json exit = %d, want %d", code, tt.wantCode)
			}
			if stderr.Len() != 0 {
				t.Fatalf("json stderr = %q, want empty", stderr.String())
			}
			assertExactWorkflowJSONFields(t, decodeExactlyOneJSONObject(t, stdout.Bytes()), tt.wantJSON)
		})
	}
}

func assertWorkflowTransitionDoesNotMutate(t *testing.T, record *string, featureID, target string, wantCode int) {
	t.Helper()
	dir := workflowTempRoot(t, record)
	before := snapshotDir(t, dir)
	for _, format := range []string{"text", "json"} {
		var stdout, stderr bytes.Buffer
		if code := RunWorkflowTransition([]string{"--format", format, featureID, target}, os.DirFS(dir), &stdout, &stderr); code != wantCode {
			t.Fatalf("%s exit = %d, want %d (stderr %q)", format, code, wantCode, stderr.String())
		}
	}
	if after := snapshotDir(t, dir); !reflect.DeepEqual(before, after) {
		t.Fatalf("filesystem changed:\nbefore=%v\nafter=%v", before, after)
	}
}

func TestRunWorkflowTransitionValidDoesNotMutateFilesystem(t *testing.T) {
	assertWorkflowTransitionDoesNotMutate(t, strPtr(`{"state":"planned"}`), workflowTestFeature, "tasked", 0)
}

func TestRunWorkflowTransitionInvalidDoesNotMutateFilesystem(t *testing.T) {
	assertWorkflowTransitionDoesNotMutate(t, strPtr(`{"state":"planned"}`), workflowTestFeature, "approved", 1)
}

func TestRunWorkflowTransitionTargetUnsupportedDoesNotMutateFilesystem(t *testing.T) {
	assertWorkflowTransitionDoesNotMutate(t, strPtr(`{"state":"planned"}`), workflowTestFeature, "reviewing", 1)
}

func TestRunWorkflowTransitionFeatureUnknownDoesNotMutateFilesystem(t *testing.T) {
	assertWorkflowTransitionDoesNotMutate(t, nil, "does-not-exist", "tasked", 1)
}

func TestRunWorkflowTransitionStateInvalidDoesNotMutateFilesystem(t *testing.T) {
	assertWorkflowTransitionDoesNotMutate(t, strPtr(`{}`), workflowTestFeature, "tasked", 1)
}

func TestRunWorkflowTransitionIsDeterministic(t *testing.T) {
	fixture := workflowFixture(strPtr(`{"state":"implemented"}`))
	for _, format := range []string{"text", "json"} {
		for _, target := range []string{"approved", "draft", "reviewing"} {
			var first, second, stderr bytes.Buffer
			RunWorkflowTransition([]string{"--format", format, workflowTestFeature, target}, fixture, &first, &stderr)
			RunWorkflowTransition([]string{"--format", format, workflowTestFeature, target}, fixture, &second, &stderr)
			if first.String() != second.String() {
				t.Fatalf("%s/%s output differs: %q vs %q", format, target, first.String(), second.String())
			}
		}
	}
}

func TestRunWorkflowTransitionWriteFailureIsNotReclassified(t *testing.T) {
	for _, format := range []string{"text", "json"} {
		var stderr bytes.Buffer
		code := RunWorkflowTransition([]string{"--format", format, workflowTestFeature, "tasked"}, workflowFixture(nil), errWriter{}, &stderr)
		if code != 2 {
			t.Fatalf("%s exit = %d, want 2", format, code)
		}
		if !strings.Contains(stderr.String(), "write failed") || strings.Contains(stderr.String(), "outcome=") {
			t.Fatalf("%s stderr = %q, want a write diagnostic without an outcome", format, stderr.String())
		}
	}
}

// --- Dispatcher-level assertions (T026) ---

func TestRunDispatchesWorkflowTransition(t *testing.T) {
	code, stdout, stderr, exec, launcher := runDispatcher(t, []string{"workflow", "transition", "--format", "json", workflowTestFeature, "approved"}, workflowFixture(strPtr(`{"state":"implemented"}`)))
	if code != 0 || stderr != "" {
		t.Fatalf("exit=%d stderr=%q, want 0 and empty", code, stderr)
	}
	assertExactWorkflowJSONFields(t, decodeExactlyOneJSONObject(t, []byte(stdout)),
		map[string]any{"feature_id": workflowTestFeature, "current_state": "implemented", "target": "approved", "outcome": "transition_valid"})
	if exec.lookups != 0 || exec.runs != 0 || launcher.launches != 0 {
		t.Fatalf("workflow transition reached an executor or launcher: lookups=%d runs=%d launches=%d", exec.lookups, exec.runs, launcher.launches)
	}

	code, stdout, stderr, _, _ = runDispatcher(t, []string{"workflow", "transition", "does-not-exist", "reviewing"}, fstest.MapFS{})
	if code != 1 || stderr != "" || stdout != "ai-dev: outcome=target_unsupported feature=\"does-not-exist\" target=\"reviewing\"\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestRunWorkflowTransitionFailureOutcomesNeverResembleSuccess(t *testing.T) {
	cases := map[string]struct {
		root      fstest.MapFS
		featureID string
		target    string
		want      string
	}{
		"feature_unknown":           {fstest.MapFS{}, "does-not-exist", "specified", "feature_unknown"},
		"feature_unknown traversal": {workflowFixture(nil), "../x", "specified", "feature_unknown"},
		"state_invalid wrong case":  {workflowFixture(strPtr(`{"state":"Planned"}`)), workflowTestFeature, "tasked", "state_invalid"},
		"state_invalid absent":      {workflowFixture(strPtr(`{}`)), workflowTestFeature, "specified", "state_invalid"},
		"state_invalid unparseable": {workflowFixture(strPtr(`[`)), workflowTestFeature, "specified", "state_invalid"},
	}
	for name, tc := range cases {
		for _, format := range []string{"text", "json"} {
			code, stdout, stderr, _, _ := runDispatcher(t, []string{"workflow", "transition", "--format", format, tc.featureID, tc.target}, tc.root)
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

// --- Remediation I-2: text line injection through the feature identifier ---

const workflowInjectionFeatureID = "x\nai-dev: outcome=transition_valid feature=f current=draft target=specified"

const workflowInjectionQuoted = `"x\nai-dev: outcome=transition_valid feature=f current=draft target=specified"`

func TestRunWorkflowTextOutputCannotBeInjectedThroughFeatureID(t *testing.T) {
	existing := fstest.MapFS{
		"specs/" + workflowInjectionFeatureID:                          &fstest.MapFile{Mode: fs.ModeDir | 0o755},
		"specs/" + workflowInjectionFeatureID + "/workflow-state.json": &fstest.MapFile{Data: []byte(`{"state":"planned"}`)},
	}
	cases := []struct {
		name     string
		args     []string
		root     fstest.MapFS
		wantCode int
		outcome  string
		want     string
	}{
		{"transition target_unsupported", []string{"workflow", "transition", workflowInjectionFeatureID, "reviewing"}, fstest.MapFS{}, 1, "target_unsupported",
			"ai-dev: outcome=target_unsupported feature=" + workflowInjectionQuoted + " target=\"reviewing\"\n"},
		{"transition feature_unknown", []string{"workflow", "transition", workflowInjectionFeatureID, "specified"}, fstest.MapFS{}, 1, "feature_unknown",
			"ai-dev: outcome=feature_unknown feature=" + workflowInjectionQuoted + "\n"},
		{"transition_invalid on existing feature", []string{"workflow", "transition", workflowInjectionFeatureID, "approved"}, existing, 1, "transition_invalid",
			"ai-dev: outcome=transition_invalid feature=" + workflowInjectionQuoted + " current=planned target=approved\n"},
		{"transition_valid on existing feature", []string{"workflow", "transition", workflowInjectionFeatureID, "tasked"}, existing, 0, "transition_valid",
			"ai-dev: outcome=transition_valid feature=" + workflowInjectionQuoted + " current=planned target=tasked\n"},
		{"state feature_unknown", []string{"workflow", "state", workflowInjectionFeatureID}, fstest.MapFS{}, 1, "feature_unknown",
			"ai-dev: outcome=feature_unknown feature=" + workflowInjectionQuoted + "\n"},
		{"state_reported on existing feature", []string{"workflow", "state", workflowInjectionFeatureID}, existing, 0, "state_reported",
			"ai-dev: outcome=state_reported feature=" + workflowInjectionQuoted + " state=planned terminal=false\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr, _, _ := runDispatcher(t, tc.args, tc.root)
			if code != tc.wantCode || stderr != "" {
				t.Fatalf("exit=%d stderr=%q, want %d and empty", code, stderr, tc.wantCode)
			}
			if stdout != tc.want {
				t.Fatalf("stdout = %q, want %q", stdout, tc.want)
			}
			// One line whose leading outcome token is the real one; the forged
			// "outcome=transition_valid" survives only escaped inside the quoted ID.
			assertExactlyOneTextLine(t, stdout)
			if !strings.HasPrefix(stdout, "ai-dev: outcome="+tc.outcome+" feature=\"x\\n") {
				t.Fatalf("stdout %q does not lead with outcome %q and the escaped feature ID", stdout, tc.outcome)
			}
		})
	}
}

// --- Remediation I-1: exact, case-sensitive, non-duplicated "state" key ---

func TestRunWorkflowRejectsCaseVariantAndDuplicateStateKeys(t *testing.T) {
	for _, record := range []string{
		`{"State":"approved"}`,
		`{"STATE":"planned"}`,
		`{"ſtate":"approved"}`,
		`{"state":"draft","State":"approved"}`,
		`{"state":"draft","state":"approved"}`,
	} {
		root := workflowFixture(strPtr(record))
		code, stdout, stderr, _, _ := runDispatcher(t, []string{"workflow", "state", workflowTestFeature}, root)
		if code != 1 || stderr != "" || stdout != "ai-dev: outcome=state_invalid feature=\"004-workflow-state\"\n" {
			t.Fatalf("%s: state text exit=%d stdout=%q stderr=%q", record, code, stdout, stderr)
		}
		code, stdout, stderr, _, _ = runDispatcher(t, []string{"workflow", "state", "--format", "json", workflowTestFeature}, root)
		if code != 1 || stderr != "" {
			t.Fatalf("%s: state json exit=%d stderr=%q", record, code, stderr)
		}
		assertExactWorkflowJSONFields(t, decodeExactlyOneJSONObject(t, []byte(stdout)),
			map[string]any{"feature_id": workflowTestFeature, "outcome": "state_invalid"})
		for _, target := range []string{"specified", "abandoned"} {
			code, stdout, stderr, _, _ = runDispatcher(t, []string{"workflow", "transition", workflowTestFeature, target}, root)
			if code != 1 || stderr != "" || stdout != "ai-dev: outcome=state_invalid feature=\"004-workflow-state\"\n" {
				t.Fatalf("%s/%s: transition exit=%d stdout=%q stderr=%q", record, target, code, stdout, stderr)
			}
		}
	}
}
