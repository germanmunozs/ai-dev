package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

const watchAgentsConfig = `{
  "schema_version": "ai-dev.agents/v1",
  "agents": ["claude-code", "codex-cli"],
  "roles": {"specification": "claude-code", "planning": "claude-code", "implementation": "codex-cli", "review": "claude-code", "remediation": "codex-cli"}
}
`

// writeWatchFixture creates a project with a feature in every Workflow
// State, an invalid record, a symbolic-link feature directory and the given
// agent configuration ("" for none).
func writeWatchFixture(t *testing.T, agents string) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, content string) {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	artifacts := map[string][]string{
		"001-draft": nil, "002-specified": {"spec.md"}, "003-planned": {"spec.md", "plan.md"},
		"004-tasked": {"spec.md", "plan.md", "tasks.md"}, "005-implemented": {"spec.md", "plan.md", "tasks.md"},
		"006-approved": {"spec.md", "plan.md", "tasks.md"}, "007-abandoned": {"spec.md"}, "008-invalid": {"spec.md"},
		"009-inconsistent": nil,
	}
	states := map[string]string{
		"002-specified": `{"state":"specified"}`, "003-planned": `{"state":"planned"}`, "004-tasked": `{"state":"tasked"}`,
		"005-implemented": `{"state":"implemented"}`, "006-approved": `{"state":"approved"}`, "007-abandoned": `{"state":"abandoned"}`,
		"008-invalid": `{"state":"Approved"}`, "009-inconsistent": `{"state":"planned"}`,
	}
	for feature, files := range artifacts {
		if err := os.MkdirAll(filepath.Join(dir, "specs", feature), 0o755); err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			write(filepath.Join("specs", feature, file), "# "+file+"\n")
		}
		if record := states[feature]; record != "" {
			write(filepath.Join("specs", feature, "workflow-state.json"), record)
		}
	}
	if err := os.Symlink(filepath.Join(dir, "specs", "002-specified"), filepath.Join(dir, "specs", "010-link")); err != nil {
		t.Fatal(err)
	}
	if agents != "" {
		write(filepath.Join(".ai-dev", "agents.json"), agents)
	}
	return dir
}

func watchFixtureExecutor() *fakeExecutor {
	return &fakeExecutor{stdout: "true\n", paths: map[string]string{"git": "/usr/bin/git", "claude": "/tools/claude", "specify": "/tools/specify"}}
}

func commandOutput(t *testing.T, dir string, executor *fakeExecutor, args ...string) []byte {
	t.Helper()
	var stdout, stderr bytes.Buffer
	RunWithDependencies(context.Background(), args, Dependencies{Executor: executor, Root: os.DirFS(dir), Stdout: &stdout, Stderr: &stderr,
		Git: unavailableGitManagementDependencies(), Stdin: bytes.NewReader(nil)})
	if stdout.Len() == 0 {
		t.Fatalf("%v printed nothing; stderr %q", args, stderr.String())
	}
	return stdout.Bytes()
}

// Each result is the command's own output, byte for byte (F015-FR-006,
// SC-007).
func TestWatchSourceEqualsCommands(t *testing.T) {
	for name, agents := range map[string]string{"configured": watchAgentsConfig, "invalid": "{", "missing": ""} {
		t.Run(name, func(t *testing.T) {
			dir := writeWatchFixture(t, agents)
			executor := watchFixtureExecutor()
			source := newWatchSource(os.DirFS(dir), executor)

			features, err := source.Features()
			if err != nil {
				t.Fatal(err)
			}
			rows, err := cockpitSource{executor: executor, root: os.DirFS(dir)}.LoadFeatures(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			var want []string
			for _, row := range rows {
				want = append(want, row.ID)
			}
			if !slices.Equal(features, want) || slices.Contains(features, "010-link") {
				t.Fatalf("Features = %v, want %v", features, want)
			}

			for _, id := range append(features, "099-unknown") {
				state := source.State(id)
				if got, want := state.JSON, commandOutput(t, dir, executor, "workflow", "state", "--format", "json", id); !bytes.Equal(got, want) {
					t.Errorf("%s state:\n%s\nwant\n%s", id, got, want)
				}
				if got, want := source.Next(id), commandOutput(t, dir, executor, "workflow", "next", "--format", "json", id); !bytes.Equal(got, want) {
					t.Errorf("%s next:\n%s\nwant\n%s", id, got, want)
				}
			}
			if s := source.State("006-approved"); !s.Recognized || s.State != "approved" || s.Outcome != "state_reported" {
				t.Errorf("approved state result = %+v", s)
			}
			if s := source.State("008-invalid"); s.Recognized || s.Outcome != "state_invalid" {
				t.Errorf("invalid state result = %+v", s)
			}
			if got, want := source.Setup(), commandOutput(t, dir, executor, "setup", "show", "--format", "json"); !bytes.Equal(got, want) {
				t.Errorf("setup:\n%s\nwant\n%s", got, want)
			}
			env := source.Environment(context.Background())
			if want := commandOutput(t, dir, executor, "doctor", "--format", "json"); !bytes.Equal(env.JSON, want) {
				t.Errorf("environment:\n%s\nwant\n%s", env.JSON, want)
			}
			if len(env.Checks) != 8 {
				t.Errorf("environment checks = %+v, want the 8 doctor checks", env.Checks)
			}
			for _, check := range env.Checks {
				if check.ID == "" || check.Status != "pass" && check.Status != "fail" {
					t.Errorf("check %+v", check)
				}
			}
		})
	}
}

func TestWatchSourceWithoutSpecs(t *testing.T) {
	dir := t.TempDir()
	features, err := newWatchSource(os.DirFS(dir), watchFixtureExecutor()).Features()
	if err != nil || len(features) != 0 {
		t.Fatalf("Features = %v, %v; want none", features, err)
	}
}

// A specs that is not a real, readable directory holds no features, as if
// it were absent, so watch keeps running (review F13); the cockpit's own
// listing is unchanged.
func TestWatchSourceSpecsNotADirectory(t *testing.T) {
	cases := map[string]func(t *testing.T, dir string){
		"file": func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, "specs"), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"symbolic link": func(t *testing.T, dir string) {
			real := filepath.Join(t.TempDir(), "specs")
			if err := os.MkdirAll(filepath.Join(real, "001-a"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(real, filepath.Join(dir, "specs")); err != nil {
				t.Fatal(err)
			}
		},
		"unreadable": func(t *testing.T, dir string) {
			if os.Getuid() == 0 {
				t.Skip("permissions do not apply to root")
			}
			specs := filepath.Join(dir, "specs")
			if err := os.MkdirAll(filepath.Join(specs, "001-a"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(specs, 0); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(specs, 0o755) })
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			setup(t, dir)
			features, err := newWatchSource(os.DirFS(dir), watchFixtureExecutor()).Features()
			if err != nil || len(features) != 0 {
				t.Fatalf("Features = %v, %v; want none and no error", features, err)
			}
		})
	}
}
