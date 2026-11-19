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

	"ai-dev/internal/guide"
	"ai-dev/internal/workflow"
)

func TestRunWorkflowGuideUsageFailures(t *testing.T) {
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
		{"autonomous flag", []string{"--autonomous", workflowTestFeature}},
		{"execute flag", []string{"--execute", workflowTestFeature}},
	}
	const wantUsage = "Usage: ai-dev workflow guide [--format text|json] <feature-id>\n"
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := &recordingRootFS{inner: workflowFixture(strPtr(`{"state":"planned"}`))}
			called := false
			evaluate := func(fs.FS, string) guide.Report {
				called = true
				return guide.Report{}
			}
			var stdout, stderr bytes.Buffer
			code := runWorkflowGuide(tc.args, root, &stdout, &stderr, evaluate)
			if code != 2 || stdout.Len() != 0 || stderr.String() != wantUsage {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if called || len(root.calls) != 0 {
				t.Fatalf("usage failure called evaluator=%v filesystem calls=%v", called, root.calls)
			}
		})
	}
}

func TestRunWorkflowGuideExitCodesMatchWorkflowNext(t *testing.T) {
	cases := []struct {
		name   string
		record *string
		files  []string
		id     string
		want   int
	}{
		{"draft", nil, nil, workflowTestFeature, 0},
		{"specified", strPtr(`{"state":"specified"}`), []string{"spec.md"}, workflowTestFeature, 0},
		{"tasked", strPtr(`{"state":"tasked"}`), []string{"spec.md", "plan.md", "tasks.md"}, workflowTestFeature, 0},
		{"approved", strPtr(`{"state":"approved"}`), []string{"spec.md", "plan.md", "tasks.md"}, workflowTestFeature, 0},
		{"abandoned", strPtr(`{"state":"abandoned"}`), nil, workflowTestFeature, 0},
		{"artifacts inconsistent", strPtr(`{"state":"planned"}`), []string{"spec.md"}, workflowTestFeature, 1},
		{"state invalid", strPtr(`{"state":"Planned"}`), nil, workflowTestFeature, 1},
		{"feature unknown", nil, nil, "missing", 1},
		{"structurally invalid", nil, nil, "../x", 1},
	}
	for _, tc := range cases {
		for _, format := range []string{"text", "json"} {
			t.Run(tc.name+" "+format, func(t *testing.T) {
				root := workflowNextFixture(tc.record, tc.files...)
				executor := &fakeExecutor{paths: map[string]string{"claude": "/fake/claude"}}
				var stdout, stderr, nextOut, nextErr bytes.Buffer
				code := RunWorkflowGuide([]string{"--format", format, tc.id}, root, executor, &stdout, &stderr)
				nextCode := RunWorkflowNext([]string{"--format", format, tc.id}, root, &nextOut, &nextErr)
				if code != tc.want || code != nextCode || stderr.Len() != 0 || stdout.Len() == 0 {
					t.Fatalf("guide exit=%d next exit=%d want %d stdout=%q stderr=%q", code, nextCode, tc.want, stdout.String(), stderr.String())
				}
				if executor.runs != 0 {
					t.Fatalf("guidance ran %d processes", executor.runs)
				}
			})
		}
	}
}

func TestRunWorkflowGuideCannotRenderInvalidReport(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runWorkflowGuide([]string{"f"}, workflowFixture(nil), &stdout, &stderr, func(fs.FS, string) guide.Report {
		return guide.Report{}
	})
	const want = "ai-dev: internal error: workflow guide report cannot be rendered for feature \"f\"\n"
	if code != 2 || stdout.Len() != 0 || stderr.String() != want {
		t.Fatalf("exit=%d stdout=%q stderr=%q, want exit 2 empty stdout stderr %q", code, stdout.String(), stderr.String(), want)
	}
}

func TestRunWorkflowGuideOutputFailure(t *testing.T) {
	for _, format := range []string{"text", "json"} {
		writer := &workflowNextCountingErrorWriter{}
		var stderr bytes.Buffer
		executor := &fakeExecutor{}
		code := RunWorkflowGuide([]string{"--format", format, workflowTestFeature}, workflowFixture(nil), executor, writer, &stderr)
		if code != 2 || writer.calls != 1 || !strings.HasPrefix(stderr.String(), "write workflow guide report: ") {
			t.Fatalf("%s: exit=%d writes=%d stderr=%q", format, code, writer.calls, stderr.String())
		}
	}
}

// guideTreeDigest records every path, mode and regular-file content under dir.
func guideTreeDigest(t *testing.T, dir string) []string {
	t.Helper()
	var digest []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		content := ""
		if entry.Type().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			content = string(data)
		}
		digest = append(digest, path+"|"+info.Mode().String()+"|"+info.ModTime().String()+"|"+content)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func TestRunWorkflowGuideHasNoSideEffects(t *testing.T) {
	repo := controlledTempDir(t)
	dir := filepath.Join(repo, "specs", "f")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, workflow.RecordFileName), []byte(`{"state":"specified"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "spec.md"), []byte("spec"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := guideTreeDigest(t, repo)
	for _, args := range [][]string{{"workflow", "guide", "f"}, {"workflow", "guide", "--format", "json", "f"}, {"workflow", "guide", "missing"}} {
		executor := &fakeExecutor{paths: map[string]string{"claude": "/fake/claude", "codex": "/fake/codex"}}
		launcher := &appRunLauncher{}
		stdin := &workflowNextFailReader{t: t}
		var stdout, stderr bytes.Buffer
		_ = Run(context.Background(), args, executor, launcher, os.DirFS(repo), stdin, &stdout, &stderr)
		if executor.runs != 0 || launcher.launches != 0 || stdin.called {
			t.Fatalf("%v: runs=%d launches=%d stdin read=%v", args, executor.runs, launcher.launches, stdin.called)
		}
		if executor.lookups > 2 {
			t.Fatalf("%v: %d executable lookups, want at most the two supported agents", args, executor.lookups)
		}
	}
	if after := guideTreeDigest(t, repo); !reflect.DeepEqual(before, after) {
		t.Fatalf("tree changed:\nbefore %v\nafter  %v", before, after)
	}
}

func TestRunDispatchesWorkflowGuide(t *testing.T) {
	code, stdout, stderr, executor, launcher := runDispatcher(t, []string{"workflow", "guide", workflowTestFeature}, workflowFixture(nil))
	const wantFirst = "ai-dev: outcome=step_recommended feature=\"004-workflow-state\" state=draft activity=specify role=specification agent=claude-code availability=unavailable note="
	if code != 0 || stderr != "" || !strings.HasPrefix(stdout, wantFirst) {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if executor.runs != 0 || launcher.launches != 0 {
		t.Fatalf("workflow guide reached a process: runs=%d launches=%d", executor.runs, launcher.launches)
	}
}

// TestRunUsageTextsUnchangedWithGuide is the F007-FR-013 regression: adding
// workflow guide changes neither the workflow group usage nor the top-level
// usage.
func TestRunUsageTextsUnchangedWithGuide(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"workflow"}, "Usage: ai-dev workflow <state|transition> [options]\n"},
		{[]string{"workflow", "run", "f"}, "Usage: ai-dev workflow <state|transition> [options]\n"},
		{[]string{"workflow", "Guide", "f"}, "Usage: ai-dev workflow <state|transition> [options]\n"},
		{nil, "Usage: ai-dev <doctor|agents|agent|workflow|init> [options]\n"},
		{[]string{"orchestrate", "f"}, "Usage: ai-dev <doctor|agents|agent|workflow|init> [options]\n"},
	}
	for _, tc := range cases {
		code, stdout, stderr, executor, launcher := runDispatcher(t, tc.args, workflowFixture(nil))
		if code != 2 || stdout != "" || stderr != tc.want {
			t.Fatalf("args=%v exit=%d stdout=%q stderr=%q, want 2/empty/%q", tc.args, code, stdout, stderr, tc.want)
		}
		if executor.lookups != 0 || executor.runs != 0 || launcher.launches != 0 {
			t.Fatalf("args=%v reached an executor or launcher", tc.args)
		}
	}
}
