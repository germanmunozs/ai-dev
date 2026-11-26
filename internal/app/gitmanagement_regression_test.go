package app

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"testing/fstest"
)

// T113/F008-FR-137, 144–145/AC-020: Feature 008 leaves Features 001–006
// and the compatibility wrappers unchanged. Run and RunWithGateStore keep
// routing the earlier commands with their exits; through them the Git
// Management bundle is unavailable, so init can neither inspect nor
// mutate, and no executor or Git process is reached. The top-level usage
// line is unchanged, and the pure validator stays available everywhere.
func TestGitManagementRegression(t *testing.T) {
	type call struct {
		args []string
		code int
		out  string
	}
	for _, wrapper := range []struct {
		name string
		run  func(args []string, executor *fakeExecutor, stdout, stderr *bytes.Buffer) int
	}{
		{"Run", func(args []string, executor *fakeExecutor, stdout, stderr *bytes.Buffer) int {
			return Run(context.Background(), args, executor, &appRunLauncher{}, fstest.MapFS{}, bytes.NewReader(nil), stdout, stderr)
		}},
		{"RunWithGateStore", func(args []string, executor *fakeExecutor, stdout, stderr *bytes.Buffer) int {
			return RunWithGateStore(context.Background(), args, executor, &appRunLauncher{}, fstest.MapFS{}, nil, bytes.NewReader(nil), stdout, stderr)
		}},
	} {
		for _, test := range []call{
			{nil, 2, "Usage: ai-dev <doctor|agents|agent|workflow|init|git|setup|tui|open|watch|version|help|completion> [options]\n"},
			{[]string{"unknown"}, 2, "Usage: ai-dev <doctor|agents|agent|workflow|init|git|setup|tui|open|watch|version|help|completion> [options]\n"},
			{[]string{"doctor", "--format", "yaml"}, 2, "Usage: ai-dev doctor [--format text|json]\n"},
			{[]string{"workflow", "state", "missing"}, 1, ""},
			{[]string{"init", "inspect", "--project-dir", t.TempDir()}, 1, "Git inspection is unavailable in this build"},
			{[]string{"init", "apply", "--plan", "p", "--confirm-plan", "sha256:x", "--authorization-id", "123e4567-e89b-42d3-a456-426614174113"}, 2, "apply is not enabled"},
			{[]string{"git", "commit", "validate", "--message", "feat: still pure"}, 0, ""},
		} {
			executor := &fakeExecutor{}
			var stdout, stderr bytes.Buffer
			code := wrapper.run(test.args, executor, &stdout, &stderr)
			if code != test.code || !strings.Contains(stdout.String()+stderr.String(), test.out) {
				t.Errorf("%s %v: exit %d, output %q %q", wrapper.name, test.args, code, stdout.String(), stderr.String())
			}
			if strings.HasPrefix(strings.Join(test.args, " "), "init") || strings.HasPrefix(strings.Join(test.args, " "), "git") {
				if executor.lookups+executor.runs != 0 {
					t.Errorf("%s %v reached the Feature 001–006 executor", wrapper.name, test.args)
				}
			}
		}
	}
	if DefaultGitManagementDependencies().Runner == nil {
		t.Fatal("the production composition does not enable apply (T107)")
	}
}
