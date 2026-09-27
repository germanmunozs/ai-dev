package app

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"
	"testing/fstest"

	"ai-dev/internal/doctor"
)

type fakeExecutor struct {
	stdout  string
	paths   map[string]string
	runErr  error
	lookups int
	runs    int
}

func (f *fakeExecutor) LookPath(name string) (string, error) {
	f.lookups++
	if path := f.paths[name]; path != "" {
		return path, nil
	}
	return "", errors.New("not found")
}

func (f *fakeExecutor) Run(context.Context, string, ...string) (string, error) {
	f.runs++
	return f.stdout, f.runErr
}

func TestRunFormatAndUsageContract(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantOutput string
		usage      bool
	}{
		{"default text", []string{"doctor"}, 0, "Overall: Ready", false},
		{"explicit text", []string{"doctor", "--format", "text"}, 0, "Overall: Ready", false},
		{"json", []string{"doctor", "--format", "json"}, 0, `"overall":"ready"`, false},
		{"unknown flag", []string{"doctor", "--unknown"}, 2, "Usage:", true},
		{"invalid format", []string{"doctor", "--format", "yaml"}, 2, "Usage:", true},
		{"positional", []string{"doctor", "extra"}, 2, "Usage:", true},
		{"missing subcommand", nil, 2, "Usage:", true},
		{"unknown subcommand", []string{"status"}, 2, "Usage:", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := readyExecutor()
			var stdout, stderr bytes.Buffer
			code := Run(context.Background(), tt.args, exec, readyFS(), &stdout, &stderr)
			if code != tt.wantCode {
				t.Fatalf("code = %d, want %d; stderr=%q", code, tt.wantCode, stderr.String())
			}
			stream := stdout.String()
			if tt.usage {
				stream = stderr.String()
				if exec.lookups != 0 || exec.runs != 0 || stdout.Len() != 0 {
					t.Fatalf("usage error executed checks or wrote stdout: lookups=%d runs=%d stdout=%q", exec.lookups, exec.runs, stdout.String())
				}
			}
			if !bytes.Contains([]byte(stream), []byte(tt.wantOutput)) {
				t.Fatalf("output %q missing %q", stream, tt.wantOutput)
			}
		})
	}
}

func TestRunExitCodeIsFormatIndependent(t *testing.T) {
	for _, format := range []string{"text", "json"} {
		exec := readyExecutor()
		exec.stdout = "false"
		var stdout, stderr bytes.Buffer
		if code := Run(context.Background(), []string{"doctor", "--format", format}, exec, readyFS(), &stdout, &stderr); code != 1 {
			t.Errorf("format %s code = %d, want 1", format, code)
		}
	}
}

func readyFS() fstest.MapFS {
	return fstest.MapFS{
		".specify/integrations/speckit.manifest.json": &fstest.MapFile{Data: []byte(`{"files":{".specify/template.md":"hash"}}`)},
		".specify/template.md":                        &fstest.MapFile{Data: []byte("template")},
		".specify/memory/constitution.md":             &fstest.MapFile{Data: []byte("**Version**: 1.0.0 | **Ratified**: 2026-09-27")},
		".specify/integration.json":                   &fstest.MapFile{Data: []byte(`{"installed_integrations":["claude","codex"]}`)},
		".specify/integrations/claude.manifest.json":  &fstest.MapFile{Data: []byte(`{"files":{".claude/skill.md":"hash"}}`)},
		".claude/skill.md":                            &fstest.MapFile{Data: []byte("claude")},
		".specify/integrations/codex.manifest.json":   &fstest.MapFile{Data: []byte(`{"files":{".agents/skill.md":"hash"}}`)},
		".agents/skill.md":                            &fstest.MapFile{Data: []byte("codex")},
	}
}

func cloneFS(source fstest.MapFS) fstest.MapFS {
	clone := make(fstest.MapFS, len(source))
	for path, file := range source {
		copyFile := *file
		copyFile.Data = append([]byte(nil), file.Data...)
		clone[path] = &copyFile
	}
	return clone
}

func readyExecutor() *fakeExecutor {
	return &fakeExecutor{stdout: "true", paths: map[string]string{"git": "git", "specify": "specify", "claude": "claude", "codex": "codex"}}
}

func TestRunDoctorReadyIsDeterministicAndOrdered(t *testing.T) {
	wantIDs := []string{"git-tool", "speckit-tool", "claude-code-tool", "codex-cli-tool", "git-repository", "speckit-initialized", "constitution", "claude-integration", "codex-integration"}
	wantNames := []string{"Git", "Spec Kit", "Claude Code", "Codex CLI", "Current Git Repository", "Spec Kit Initialization", "Project Constitution", "Claude Spec Kit Integration", "Codex Spec Kit Integration"}
	var previous doctor.Report
	for run := 0; run < 2; run++ {
		exec := &fakeExecutor{stdout: "true\n", paths: map[string]string{"git": "/bin/git", "specify": "/bin/specify", "claude": "/bin/claude", "codex": "/bin/codex"}}
		report := RunDoctor(context.Background(), exec, readyFS())
		if report.Overall != doctor.Pass || len(report.Checks) != 9 {
			t.Fatalf("unexpected report: %#v", report)
		}
		ids := make([]string, 0, 9)
		names := make([]string, 0, 9)
		for _, result := range report.Checks {
			ids = append(ids, result.ID)
			names = append(names, result.Name)
			if result.Status != doctor.Pass || (result.Area != "Environment" && result.Area != "Repository") {
				t.Fatalf("unexpected result: %#v", result)
			}
		}
		if !reflect.DeepEqual(ids, wantIDs) || !reflect.DeepEqual(names, wantNames) {
			t.Fatalf("ids/names = %v/%v", ids, names)
		}
		if run == 1 && !reflect.DeepEqual(report, previous) {
			t.Fatalf("repeat report changed: %#v != %#v", report, previous)
		}
		previous = report
	}
}

func TestRunReadyTextExitZero(t *testing.T) {
	exec := &fakeExecutor{stdout: "true", paths: map[string]string{"git": "git", "specify": "specify", "claude": "claude", "codex": "codex"}}
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"doctor"}, exec, readyFS(), &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 || stdout.Len() == 0 {
		t.Fatalf("Run = code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}

func TestRunDoctorEachSingleFailureIsIsolatedAndExitOne(t *testing.T) {
	baseline := RunDoctor(context.Background(), readyExecutor(), readyFS())
	tests := []struct {
		id    string
		setup func(*fakeExecutor, fstest.MapFS)
	}{
		{"git-tool", func(e *fakeExecutor, _ fstest.MapFS) { delete(e.paths, "git") }},
		{"speckit-tool", func(e *fakeExecutor, _ fstest.MapFS) { delete(e.paths, "specify") }},
		{"claude-code-tool", func(e *fakeExecutor, _ fstest.MapFS) { delete(e.paths, "claude") }},
		{"codex-cli-tool", func(e *fakeExecutor, _ fstest.MapFS) { delete(e.paths, "codex") }},
		{"git-repository", func(e *fakeExecutor, _ fstest.MapFS) { e.stdout = "false" }},
		{"speckit-initialized", func(_ *fakeExecutor, root fstest.MapFS) { delete(root, ".specify/integrations/speckit.manifest.json") }},
		{"constitution", func(_ *fakeExecutor, root fstest.MapFS) { delete(root, ".specify/memory/constitution.md") }},
		{"claude-integration", func(_ *fakeExecutor, root fstest.MapFS) { delete(root, ".specify/integrations/claude.manifest.json") }},
		{"codex-integration", func(_ *fakeExecutor, root fstest.MapFS) { delete(root, ".specify/integrations/codex.manifest.json") }},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			exec := readyExecutor()
			root := cloneFS(readyFS())
			tt.setup(exec, root)
			report := RunDoctor(context.Background(), exec, root)
			if report.Overall != doctor.Fail || len(report.Checks) != 9 {
				t.Fatalf("unexpected report: %#v", report)
			}
			for i, got := range report.Checks {
				if got.ID == tt.id {
					if got.Status != doctor.Fail || got.Explanation == "" || got.Remediation == "" {
						t.Fatalf("failed check is not actionable: %#v", got)
					}
				} else if got != baseline.Checks[i] {
					t.Fatalf("unaffected check %s changed: %#v != %#v", got.ID, got, baseline.Checks[i])
				}
			}
			var stdout, stderr bytes.Buffer
			if code := Run(context.Background(), []string{"doctor"}, exec, root, &stdout, &stderr); code != 1 {
				t.Fatalf("exit = %d, want 1", code)
			}
		})
	}
}

func TestRunDoctorOutsideRepositoryAndAllFailuresStillReportsNine(t *testing.T) {
	t.Run("outside repository", func(t *testing.T) {
		exec := readyExecutor()
		exec.stdout = "false"
		report := RunDoctor(context.Background(), exec, fstest.MapFS{})
		if len(report.Checks) != 9 || report.Overall != doctor.Fail {
			t.Fatalf("unexpected report: %#v", report)
		}
		for _, result := range report.Checks[4:] {
			if result.Status != doctor.Fail {
				t.Errorf("repository check passed outside repository: %#v", result)
			}
		}
	})
	t.Run("all fail", func(t *testing.T) {
		report := RunDoctor(context.Background(), &fakeExecutor{runErr: errors.New("not found")}, fstest.MapFS{})
		if len(report.Checks) != 9 || report.Overall != doctor.Fail {
			t.Fatalf("unexpected report: %#v", report)
		}
		for _, result := range report.Checks {
			if result.Status != doctor.Fail || result.Explanation == "" || result.Remediation == "" {
				t.Errorf("result is not actionable Fail: %#v", result)
			}
		}
	})
}
