package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func buildWorkflowNextBinary(t *testing.T) string {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "ai-dev")
	build := exec.Command("go", "build", "-o", bin, "./cmd/ai-dev")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build workflow next binary: %v\n%s", err, output)
	}
	return bin
}

func makeWorkflowNextRepo(t *testing.T, record *string, files ...string) string {
	t.Helper()
	root := t.TempDir()
	featureDir := filepath.Join(root, "specs", "f")
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if record != nil {
		if err := os.WriteFile(filepath.Join(featureDir, "workflow-state.json"), []byte(*record), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(featureDir, name), []byte("fixture"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestWorkflowNextBinaryContract(t *testing.T) {
	bin := buildWorkflowNextBinary(t)
	approvedRecord := `{"state":"approved"}`
	plannedRecord := `{"state":"planned"}`
	invalidRecord := `{"state":"Planned"}`
	cases := []struct {
		name     string
		record   *string
		files    []string
		wantCode int
		wantText string
		wantJSON string
	}{
		{"step recommended", nil, nil, 0,
			"ai-dev: outcome=step_recommended feature=\"f\" state=draft activity=specify role=specification note=\"recommendation only; does not authorize this activity or any transition\"\n",
			"{\"feature_id\":\"f\",\"outcome\":\"step_recommended\",\"current_state\":\"draft\",\"activity\":\"specify\",\"role\":\"specification\"}\n"},
		{"workflow terminal", &approvedRecord, []string{"spec.md", "plan.md", "tasks.md"}, 0,
			"ai-dev: outcome=workflow_terminal feature=\"f\" state=approved\n",
			"{\"feature_id\":\"f\",\"outcome\":\"workflow_terminal\",\"current_state\":\"approved\"}\n"},
		{"artifacts inconsistent", &plannedRecord, []string{"spec.md"}, 1,
			"ai-dev: outcome=artifacts_inconsistent feature=\"f\" state=planned missing=plan.md\n",
			"{\"feature_id\":\"f\",\"outcome\":\"artifacts_inconsistent\",\"current_state\":\"planned\",\"missing_artifacts\":[\"plan.md\"]}\n"},
		{"feature unknown", nil, nil, 1,
			"ai-dev: outcome=feature_unknown feature=\"missing\"\n",
			"{\"feature_id\":\"missing\",\"outcome\":\"feature_unknown\"}\n"},
		{"state invalid", &invalidRecord, nil, 1,
			"ai-dev: outcome=state_invalid feature=\"f\"\n",
			"{\"feature_id\":\"f\",\"outcome\":\"state_invalid\"}\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := makeWorkflowNextRepo(t, tc.record, tc.files...)
			id := "f"
			if tc.name == "feature unknown" {
				id = "missing"
			}
			cmd := exec.Command(bin, "workflow", "next", id)
			cmd.Dir = repo
			stdout, stderr, code := runCommand(cmd)
			if code != tc.wantCode || stdout != tc.wantText || stderr != "" {
				t.Fatalf("text code/stdout/stderr = %d/%q/%q, want %d/%q/empty", code, stdout, stderr, tc.wantCode, tc.wantText)
			}

			cmd = exec.Command(bin, "workflow", "next", "--format", "json", id)
			cmd.Dir = repo
			stdout, stderr, code = runCommand(cmd)
			if code != tc.wantCode || stdout != tc.wantJSON || stderr != "" {
				t.Fatalf("json code/stdout/stderr = %d/%q/%q, want %d/%q/empty", code, stdout, stderr, tc.wantCode, tc.wantJSON)
			}
		})
	}
}

func TestWorkflowNextBinaryUsageFailures(t *testing.T) {
	bin := buildWorkflowNextBinary(t)
	repo := makeWorkflowNextRepo(t, nil)
	cases := [][]string{
		{"workflow", "next"},
		{"workflow", "next", ""},
		{"workflow", "next", "   "},
		{"workflow", "next", "f", "extra"},
		{"workflow", "next", "--unknown", "f"},
		{"workflow", "next", "--format", "yaml", "f"},
		{"workflow", "next", "--format"},
		{"workflow", "next", "f", "--format", "json"},
	}
	const wantUsage = "Usage: ai-dev workflow next [--format text|json] <feature-id>\n"
	for _, args := range cases {
		cmd := exec.Command(bin, args...)
		cmd.Dir = repo
		stdout, stderr, code := runCommand(cmd)
		if code != 2 || stdout != "" || stderr != wantUsage {
			t.Fatalf("args=%v code/stdout/stderr = %d/%q/%q", args, code, stdout, stderr)
		}
	}
}

func TestWorkflowNextBinaryTerminalExitCodesMatch(t *testing.T) {
	bin := buildWorkflowNextBinary(t)
	for _, state := range []string{"approved", "abandoned"} {
		record := `{"state":"` + state + `"}`
		files := []string(nil)
		if state == "approved" {
			files = []string{"spec.md", "plan.md", "tasks.md"}
		}
		repo := makeWorkflowNextRepo(t, &record, files...)
		cmd := exec.Command(bin, "workflow", "next", "f")
		cmd.Dir = repo
		_, stderr, code := runCommand(cmd)
		if code != 0 || stderr != "" {
			t.Fatalf("state=%s exit=%d stderr=%q", state, code, stderr)
		}
	}
}
