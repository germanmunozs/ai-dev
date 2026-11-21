package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/germanmunozs/ai-dev/internal/sdd"
	"github.com/germanmunozs/ai-dev/internal/workflow"
)

func workflowNextFixture(record *string, files ...string) fstest.MapFS {
	fixture := workflowFixture(record)
	for _, name := range files {
		fixture["specs/"+workflowTestFeature+"/"+name] = &fstest.MapFile{Data: []byte("fixture"), Mode: 0o644}
	}
	return fixture
}

type workflowNextCountingErrorWriter struct{ calls int }

func (w *workflowNextCountingErrorWriter) Write([]byte) (int, error) {
	w.calls++
	return 0, errors.New("workflow next injected write failure")
}

type workflowNextFailReader struct {
	t      *testing.T
	called bool
}

func (r *workflowNextFailReader) Read([]byte) (int, error) {
	r.called = true
	r.t.Fatal("workflow next read stdin")
	return 0, io.EOF
}

func TestRunWorkflowNextUsageFailures(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"no ID", nil},
		{"explicit empty ID", []string{""}},
		{"whitespace ID", []string{" \t"}},
		{"extra positional", []string{workflowTestFeature, "extra"}},
		{"unknown flag", []string{"--unknown", workflowTestFeature}},
		{"invalid format", []string{"--format", "yaml", workflowTestFeature}},
		{"format missing value", []string{"--format"}},
		{"flag after positional", []string{workflowTestFeature, "--format", "json"}},
	}
	const wantUsage = "Usage: ai-dev workflow next [--format text|json] <feature-id>\n"
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := &recordingRootFS{inner: workflowFixture(strPtr(`{"state":"planned"}`))}
			called := false
			evaluate := func(fs.FS, string) sdd.Report {
				called = true
				t.Fatal("usage failure called evaluator")
				return sdd.Report{}
			}
			var stdout, stderr bytes.Buffer
			code := runWorkflowNext(tc.args, root, &stdout, &stderr, evaluate)
			if code != 2 || stdout.Len() != 0 || stderr.String() != wantUsage {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if called || len(root.calls) != 0 {
				t.Fatalf("usage failure called evaluator=%v filesystem calls=%v", called, root.calls)
			}
		})
	}
}

func TestRunWorkflowNextStructurallyInvalidIDIsRecognized(t *testing.T) {
	root := &recordingRootFS{inner: workflowFixture(nil)}
	called := false
	evaluate := func(_ fs.FS, id string) sdd.Report {
		called = true
		if id != "../x" {
			t.Fatalf("evaluator ID = %q", id)
		}
		return sdd.Report{FeatureID: id, Outcome: sdd.FeatureUnknown}
	}
	var stdout, stderr bytes.Buffer
	_ = runWorkflowNext([]string{"../x"}, root, &stdout, &stderr, evaluate)
	if !called || strings.Contains(stderr.String(), "Usage:") {
		t.Fatalf("structurally invalid ID treated as usage: called=%v stderr=%q", called, stderr.String())
	}
}

func TestRunWorkflowNextCannotRenderTableMiss(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runWorkflowNext([]string{"f"}, workflowFixture(nil), &stdout, &stderr, func(fs.FS, string) sdd.Report {
		return sdd.Report{FeatureID: "f"}
	})
	const want = "ai-dev: internal error: workflow next report cannot be rendered for feature \"f\"\n"
	if code != 2 || stdout.Len() != 0 || stderr.String() != want {
		t.Fatalf("exit=%d stdout=%q stderr=%q, want exit 2 empty stdout stderr %q", code, stdout.String(), stderr.String(), want)
	}
	if strings.Contains(stdout.String()+stderr.String(), "outcome") {
		t.Fatalf("table miss emitted an outcome token: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunWorkflowGroupUsageRemainsUnchangedWithNext(t *testing.T) {
	const want = "Usage: ai-dev workflow <state|transition> [options]\n"
	for _, args := range [][]string{{"workflow"}, {"workflow", "bogus"}} {
		code, stdout, stderr, _, _ := runDispatcher(t, args, workflowFixture(nil))
		if code != 2 || stdout != "" || stderr != want {
			t.Fatalf("args=%v exit=%d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
	}
}

func TestBrokenPipeAsErrorReleaseIsCallable(t *testing.T) {
	release := brokenPipeAsError()
	if release == nil {
		t.Fatal("brokenPipeAsError returned nil release")
	}
	release()
}

func TestRunWorkflowNextStepRecommendedAllStates(t *testing.T) {
	cases := []struct {
		state    workflow.State
		record   *string
		files    []string
		activity sdd.Activity
		role     string
	}{
		{workflow.Draft, nil, nil, sdd.ActivitySpecify, "specification"},
		{workflow.Specified, strPtr(`{"state":"specified"}`), []string{sdd.SpecArtifact}, sdd.ActivityPlan, "planning"},
		{workflow.Planned, strPtr(`{"state":"planned"}`), []string{sdd.SpecArtifact, sdd.PlanArtifact}, sdd.ActivityTasks, "planning"},
		{workflow.Tasked, strPtr(`{"state":"tasked"}`), []string{sdd.SpecArtifact, sdd.PlanArtifact, sdd.TasksArtifact}, sdd.ActivityImplement, "implementation"},
		{workflow.Implemented, strPtr(`{"state":"implemented"}`), []string{sdd.SpecArtifact, sdd.PlanArtifact, sdd.TasksArtifact}, sdd.ActivityReview, "review"},
	}
	for _, tc := range cases {
		t.Run(string(tc.state), func(t *testing.T) {
			root := workflowNextFixture(tc.record, tc.files...)
			code, stdout, stderr, exec, launcher := runDispatcher(t, []string{"workflow", "next", workflowTestFeature}, root)
			wantText := "ai-dev: outcome=step_recommended feature=\"004-workflow-state\" state=" + string(tc.state) + " activity=" + string(tc.activity) + " role=" + tc.role + " note=\"recommendation only; does not authorize this activity or any transition\"\n"
			if code != 0 || stdout != wantText || stderr != "" {
				t.Fatalf("text exit=%d stdout=%q stderr=%q, want %q", code, stdout, stderr, wantText)
			}
			if exec.lookups != 0 || exec.runs != 0 || launcher.launches != 0 {
				t.Fatalf("workflow next reached executor/launcher: lookups=%d runs=%d launches=%d", exec.lookups, exec.runs, launcher.launches)
			}

			code, stdout, stderr, exec, launcher = runDispatcher(t, []string{"workflow", "next", "--format", "json", workflowTestFeature}, root)
			wantJSON := map[string]any{"feature_id": workflowTestFeature, "outcome": "step_recommended", "current_state": string(tc.state), "activity": string(tc.activity), "role": tc.role}
			if code != 0 || stderr != "" {
				t.Fatalf("json exit=%d stderr=%q", code, stderr)
			}
			if got := decodeExactlyOneJSONObject(t, []byte(stdout)); !reflect.DeepEqual(got, wantJSON) {
				t.Fatalf("JSON=%#v, want %#v", got, wantJSON)
			}
			if exec.lookups != 0 || exec.runs != 0 || launcher.launches != 0 {
				t.Fatalf("workflow next reached executor/launcher: lookups=%d runs=%d launches=%d", exec.lookups, exec.runs, launcher.launches)
			}
		})
	}
}

func TestRunWorkflowNextAheadArtifactsAndDeterminism(t *testing.T) {
	root := workflowNextFixture(strPtr(`{"state":"specified"}`), sdd.SpecArtifact, sdd.PlanArtifact, sdd.TasksArtifact)
	args := []string{"workflow", "next", workflowTestFeature}
	firstCode, firstOut, firstErr, _, _ := runDispatcher(t, args, root)
	secondCode, secondOut, secondErr, _, _ := runDispatcher(t, args, root)
	if firstCode != 0 || firstOut != secondOut || firstErr != secondErr || !strings.Contains(firstOut, "activity=plan role=planning") {
		t.Fatalf("first=(%d,%q,%q) second=(%d,%q,%q)", firstCode, firstOut, firstErr, secondCode, secondOut, secondErr)
	}
}

func TestRunWorkflowNextDoesNotMutateReadStdinOrExposeSecrets(t *testing.T) {
	dir := controlledTempDir(t)
	featureDir := filepath.Join(dir, "specs", workflowTestFeature)
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	before := snapshotDir(t, dir)
	t.Setenv("AI_DEV_TEST_SECRET", "s3cr3t-value")
	stdin := &workflowNextFailReader{t: t}
	exec := &fakeExecutor{}
	launcher := &appRunLauncher{}
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"workflow", "next", workflowTestFeature}, exec, launcher, os.DirFS(dir), stdin, &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "outcome=step_recommended") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if stdin.called || exec.lookups != 0 || exec.runs != 0 || launcher.launches != 0 {
		t.Fatalf("stdin=%v lookups=%d runs=%d launches=%d", stdin.called, exec.lookups, exec.runs, launcher.launches)
	}
	if strings.Contains(stdout.String()+stderr.String(), "s3cr3t-value") {
		t.Fatal("secret environment value appeared in output")
	}
	if _, err := os.Lstat(filepath.Join(featureDir, workflow.RecordFileName)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("workflow-state record was created: %v", err)
	}
	if after := snapshotDir(t, dir); !reflect.DeepEqual(before, after) {
		t.Fatalf("filesystem changed:\nbefore=%v\nafter=%v", before, after)
	}
}

func TestRunWorkflowNextWriteFailureIsOneAttempt(t *testing.T) {
	for _, formatArgs := range [][]string{{workflowTestFeature}, {"--format", "json", workflowTestFeature}} {
		writer := &workflowNextCountingErrorWriter{}
		var stderr bytes.Buffer
		code := RunWorkflowNext(formatArgs, workflowNextFixture(nil), writer, &stderr)
		if code != 2 || writer.calls != 1 || !strings.Contains(stderr.String(), "write workflow next report:") || strings.Contains(stderr.String(), "outcome=") {
			t.Fatalf("args=%v exit=%d calls=%d stderr=%q", formatArgs, code, writer.calls, stderr.String())
		}
	}
}

func TestRunWorkflowNextTerminalOutcomes(t *testing.T) {
	cases := []struct {
		name  string
		state workflow.State
		files []string
	}{
		{"approved", workflow.Approved, []string{sdd.SpecArtifact, sdd.PlanArtifact, sdd.TasksArtifact}},
		{"abandoned without artifacts", workflow.Abandoned, nil},
		{"abandoned with arbitrary artifacts", workflow.Abandoned, []string{sdd.SpecArtifact, "extra.md"}},
	}
	var terminalCodes []int
	var terminalJSON []map[string]any
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			record := `{"state":"` + string(tc.state) + `"}`
			root := workflowNextFixture(&record, tc.files...)
			code, stdout, stderr, _, _ := runDispatcher(t, []string{"workflow", "next", workflowTestFeature}, root)
			wantText := "ai-dev: outcome=workflow_terminal feature=\"004-workflow-state\" state=" + string(tc.state) + "\n"
			if code != 0 || stdout != wantText || stderr != "" {
				t.Fatalf("text exit=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			lowerWithoutObservedState := strings.ReplaceAll(strings.ToLower(stdout), "state="+string(tc.state), "")
			if strings.Contains(lowerWithoutObservedState, "approval") || strings.Contains(lowerWithoutObservedState, "authoriz") {
				t.Fatalf("terminal output grants authority: %q", stdout)
			}

			code, stdout, stderr, _, _ = runDispatcher(t, []string{"workflow", "next", "--format", "json", workflowTestFeature}, root)
			wantJSON := map[string]any{"feature_id": workflowTestFeature, "outcome": "workflow_terminal", "current_state": string(tc.state)}
			gotJSON := decodeExactlyOneJSONObject(t, []byte(stdout))
			if code != 0 || stderr != "" || !reflect.DeepEqual(gotJSON, wantJSON) {
				t.Fatalf("json exit=%d report=%#v stderr=%q", code, gotJSON, stderr)
			}
			terminalCodes = append(terminalCodes, code)
			terminalJSON = append(terminalJSON, gotJSON)
		})
	}
	if len(terminalCodes) != 3 || terminalCodes[0] != terminalCodes[1] || terminalCodes[1] != terminalCodes[2] {
		t.Fatalf("terminal exit codes differ: %v", terminalCodes)
	}
	approved := terminalJSON[0]
	abandoned := terminalJSON[1]
	delete(approved, "current_state")
	delete(abandoned, "current_state")
	if !reflect.DeepEqual(approved, abandoned) {
		t.Fatalf("terminal reports differ beyond current_state: approved=%#v abandoned=%#v", approved, abandoned)
	}
}

func TestRunWorkflowNextExitZeroRequiresReadingOutcome(t *testing.T) {
	stepCode, stepOut, stepErr, _, _ := runDispatcher(t, []string{"workflow", "next", "--format", "json", workflowTestFeature}, workflowNextFixture(nil))
	abandonedRecord := `{"state":"abandoned"}`
	terminalCode, terminalOut, terminalErr, _, _ := runDispatcher(t, []string{"workflow", "next", "--format", "json", workflowTestFeature}, workflowNextFixture(&abandonedRecord))
	step := decodeExactlyOneJSONObject(t, []byte(stepOut))
	terminal := decodeExactlyOneJSONObject(t, []byte(terminalOut))
	if stepCode != 0 || terminalCode != 0 || stepErr != "" || terminalErr != "" || step["outcome"] == terminal["outcome"] {
		t.Fatalf("step=(%d,%#v,%q) terminal=(%d,%#v,%q)", stepCode, step, stepErr, terminalCode, terminal, terminalErr)
	}
}

func TestRunWorkflowNextTerminalDoesNotMutate(t *testing.T) {
	for _, state := range []workflow.State{workflow.Approved, workflow.Abandoned} {
		t.Run(string(state), func(t *testing.T) {
			dir := controlledTempDir(t)
			featureDir := filepath.Join(dir, "specs", workflowTestFeature)
			if err := os.MkdirAll(featureDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(featureDir, workflow.RecordFileName), []byte(`{"state":"`+string(state)+`"}`), 0o644); err != nil {
				t.Fatal(err)
			}
			if state == workflow.Approved {
				for _, name := range []string{sdd.SpecArtifact, sdd.PlanArtifact, sdd.TasksArtifact} {
					if err := os.WriteFile(filepath.Join(featureDir, name), nil, 0o644); err != nil {
						t.Fatal(err)
					}
				}
			}
			before := snapshotDir(t, dir)
			for _, format := range []string{"text", "json"} {
				var stdout, stderr bytes.Buffer
				if code := RunWorkflowNext([]string{"--format", format, workflowTestFeature}, os.DirFS(dir), &stdout, &stderr); code != 0 || stderr.Len() != 0 {
					t.Fatalf("%s exit=%d stderr=%q", format, code, stderr.String())
				}
			}
			if after := snapshotDir(t, dir); !reflect.DeepEqual(before, after) {
				t.Fatalf("filesystem changed:\nbefore=%v\nafter=%v", before, after)
			}
		})
	}
}

func TestRunWorkflowNextArtifactsInconsistent(t *testing.T) {
	cases := []struct {
		name        string
		state       workflow.State
		files       []string
		symlinkTask bool
		wantMissing []string
	}{
		{"planned missing plan", workflow.Planned, []string{sdd.SpecArtifact}, false, []string{sdd.PlanArtifact}},
		{"implemented missing all", workflow.Implemented, nil, false, []string{sdd.SpecArtifact, sdd.PlanArtifact, sdd.TasksArtifact}},
		{"approved missing tasks", workflow.Approved, []string{sdd.SpecArtifact, sdd.PlanArtifact}, false, []string{sdd.TasksArtifact}},
		{"case variant plan", workflow.Planned, []string{sdd.SpecArtifact, "Plan.md"}, false, []string{sdd.PlanArtifact}},
		{"tasked symlink tasks", workflow.Tasked, []string{sdd.SpecArtifact, sdd.PlanArtifact}, true, []string{sdd.TasksArtifact}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			record := `{"state":"` + string(tc.state) + `"}`
			root := workflowNextFixture(&record, tc.files...)
			if tc.symlinkTask {
				root["specs/"+workflowTestFeature+"/"+sdd.TasksArtifact] = &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte(sdd.PlanArtifact)}
			}
			var stdout, stderr bytes.Buffer
			code := RunWorkflowNext([]string{workflowTestFeature}, root, &stdout, &stderr)
			wantText := "ai-dev: outcome=artifacts_inconsistent feature=\"004-workflow-state\" state=" + string(tc.state) + " missing=" + strings.Join(tc.wantMissing, ",") + "\n"
			if code != 1 || stdout.String() != wantText || stderr.Len() != 0 {
				t.Fatalf("text exit=%d stdout=%q stderr=%q, want %q", code, stdout.String(), stderr.String(), wantText)
			}
			if strings.Contains(stdout.String(), "outcome=step_recommended") {
				t.Fatalf("failure resembles recommendation: %q", stdout.String())
			}

			stdout.Reset()
			code = RunWorkflowNext([]string{"--format", "json", workflowTestFeature}, root, &stdout, &stderr)
			got := decodeExactlyOneJSONObject(t, stdout.Bytes())
			wantArray := make([]any, len(tc.wantMissing))
			for i, name := range tc.wantMissing {
				wantArray[i] = name
			}
			wantJSON := map[string]any{"feature_id": workflowTestFeature, "outcome": "artifacts_inconsistent", "current_state": string(tc.state), "missing_artifacts": wantArray}
			if code != 1 || stderr.Len() != 0 || !reflect.DeepEqual(got, wantJSON) || strings.Contains(stdout.String(), `"outcome":"step_recommended"`) {
				t.Fatalf("json exit=%d report=%#v stderr=%q", code, got, stderr.String())
			}
		})
	}
}

func TestRunWorkflowNextFeatureUnknownSkipsArtifactInspection(t *testing.T) {
	t.Run("unknown", func(t *testing.T) {
		root := &recordingRootFS{inner: workflowFixture(nil)}
		var stdout, stderr bytes.Buffer
		code := RunWorkflowNext([]string{"does-not-exist"}, root, &stdout, &stderr)
		if code != 1 || stdout.String() != "ai-dev: outcome=feature_unknown feature=\"does-not-exist\"\n" || stderr.Len() != 0 {
			t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
		for _, call := range root.calls {
			if strings.HasPrefix(call, "Open:") {
				t.Fatalf("unknown feature reached artifact listing: %v", root.calls)
			}
		}
	})

	for _, id := range []string{"..", "../x", "a/../../x", "/etc/passwd", "a/b"} {
		t.Run(id, func(t *testing.T) {
			root := &recordingRootFS{inner: workflowFixture(nil)}
			var stdout, stderr bytes.Buffer
			code := RunWorkflowNext([]string{id}, root, &stdout, &stderr)
			if code != 1 || stderr.Len() != 0 || len(root.calls) != 0 {
				t.Fatalf("exit=%d stdout=%q stderr=%q calls=%v", code, stdout.String(), stderr.String(), root.calls)
			}
			want := "ai-dev: outcome=feature_unknown feature=" + fmt.Sprintf("%q", id) + "\n"
			if stdout.String() != want {
				t.Fatalf("stdout=%q, want %q", stdout.String(), want)
			}
		})
	}

	t.Run("symlinked directory", func(t *testing.T) {
		dir := "specs/" + workflowTestFeature
		root := &recordingRootFS{inner: fstest.MapFS{dir: &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("real")}, "specs/real": &fstest.MapFile{Mode: fs.ModeDir}}}
		var stdout, stderr bytes.Buffer
		code := RunWorkflowNext([]string{workflowTestFeature}, root, &stdout, &stderr)
		if code != 1 || stderr.Len() != 0 || !reflect.DeepEqual(root.calls, []string{"Lstat:" + dir}) {
			t.Fatalf("exit=%d stdout=%q stderr=%q calls=%v", code, stdout.String(), stderr.String(), root.calls)
		}
	})
}

func TestRunWorkflowNextStateInvalidSkipsArtifactInspection(t *testing.T) {
	for _, record := range []string{`{"state":"Planned"}`, `{"state":""}`, `{`} {
		t.Run(fmt.Sprintf("%q", record), func(t *testing.T) {
			root := &recordingRootFS{inner: workflowFixture(&record)}
			var stdout, stderr bytes.Buffer
			code := RunWorkflowNext([]string{workflowTestFeature}, root, &stdout, &stderr)
			if code != 1 || stdout.String() != "ai-dev: outcome=state_invalid feature=\"004-workflow-state\"\n" || stderr.Len() != 0 {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			for _, call := range root.calls {
				if call == "Open:specs/"+workflowTestFeature {
					t.Fatalf("state_invalid reached artifact listing: %v", root.calls)
				}
			}
		})
	}
}

func TestRunWorkflowNextFailureOutcomesDoNotMutate(t *testing.T) {
	cases := []struct {
		name      string
		featureID string
		record    *string
		files     []string
	}{
		{"artifacts inconsistent", workflowTestFeature, strPtr(`{"state":"planned"}`), []string{sdd.SpecArtifact}},
		{"feature unknown", "does-not-exist", nil, nil},
		{"state invalid", workflowTestFeature, strPtr(`{"state":"Planned"}`), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := controlledTempDir(t)
			featureDir := filepath.Join(dir, "specs", workflowTestFeature)
			if err := os.MkdirAll(featureDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if tc.record != nil {
				if err := os.WriteFile(filepath.Join(featureDir, workflow.RecordFileName), []byte(*tc.record), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range tc.files {
				if err := os.WriteFile(filepath.Join(featureDir, name), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			before := snapshotDir(t, dir)
			var stdout, stderr bytes.Buffer
			if code := RunWorkflowNext([]string{tc.featureID}, os.DirFS(dir), &stdout, &stderr); code != 1 || stderr.Len() != 0 {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if after := snapshotDir(t, dir); !reflect.DeepEqual(before, after) {
				t.Fatalf("filesystem changed:\nbefore=%v\nafter=%v", before, after)
			}
		})
	}
}
