package app

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"ai-dev/internal/gate"
	"ai-dev/internal/gatestore"
	"ai-dev/internal/workflow"
)

type stateChangingStore struct {
	repo  string
	inner gate.Store
}

func (s stateChangingStore) Commit(id string, expected workflow.State, entry gate.Evidence) (gate.CommitStatus, []string) {
	_ = os.WriteFile(filepath.Join(s.repo, "specs", id, workflow.RecordFileName), []byte(`{"state":"specified"}`), 0o644)
	return s.inner.Commit(id, expected, entry)
}

func TestWorkflowGateFailureOutcomes(t *testing.T) {
	t.Run("malformed history", func(t *testing.T) {
		repo := controlledTempDir(t)
		dir := filepath.Join(repo, "specs", "f")
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, workflow.RecordFileName), []byte(`{"state":"draft","human_gates":{}}`), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "spec.md"), nil, 0o644)
		before, _ := os.ReadFile(filepath.Join(dir, workflow.RecordFileName))
		var out, errOut bytes.Buffer
		code := RunWorkflowGate(gate.Approval, []string{"--approver", "a", "f", "specified"}, os.DirFS(repo), gatestore.New(repo), &out, &errOut)
		after, _ := os.ReadFile(filepath.Join(dir, workflow.RecordFileName))
		if code != 1 || !strings.Contains(out.String(), "persistence_failed") || !strings.Contains(out.String(), `note="no transition was applied"`) || strings.Contains(out.String(), "transition_applied") || string(before) != string(after) {
			t.Fatalf("code=%d out=%q", code, out.String())
		}
	})
	t.Run("state conflict", func(t *testing.T) {
		repo := controlledTempDir(t)
		dir := filepath.Join(repo, "specs", "f")
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, "spec.md"), nil, 0o644)
		store := stateChangingStore{repo: repo, inner: gatestore.New(repo)}
		var out, errOut bytes.Buffer
		code := RunWorkflowGate(gate.Approval, []string{"--approver", "a", "f", "specified"}, os.DirFS(repo), store, &out, &errOut)
		if code != 1 || !strings.Contains(out.String(), "state_conflict") || strings.Contains(out.String(), "transition_applied") {
			t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
		}
	})
}

func TestWorkflowGateUnsupportedPlatformMapping(t *testing.T) {
	const warning = "ai-dev: warning: human gate persistence is supported only on macOS and Linux"
	for _, tc := range []struct {
		name     string
		decision gate.Decision
		args     []string
	}{
		{"approval", gate.Approval, []string{"--approver", "a", "f", "specified"}},
		{"abandonment", gate.Abandonment, []string{"--approver", "a", "--reason", "stop", "f", "abandoned"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := controlledTempDir(t)
			dir := filepath.Join(repo, "specs", "f")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "spec.md"), nil, 0o644); err != nil {
				t.Fatal(err)
			}
			before := snapshotDir(t, repo)
			store := &fakeGateStore{status: gate.Failed, warnings: []string{warning}}
			var combined bytes.Buffer
			code := RunWorkflowGate(tc.decision, tc.args, os.DirFS(repo), store, &combined, &combined)
			if code != 1 || store.calls != 1 || !strings.Contains(combined.String(), "outcome=persistence_failed") || strings.Contains(combined.String(), "transition_applied") || strings.Contains(combined.String(), "state_conflict") {
				t.Fatalf("code=%d output=%q calls=%d", code, combined.String(), store.calls)
			}
			if reportAt, warningAt := strings.Index(combined.String(), "outcome=persistence_failed"), strings.Index(combined.String(), warning); reportAt < 0 || warningAt < reportAt {
				t.Fatalf("warning order=%q", combined.String())
			}
			if after := snapshotDir(t, repo); !reflect.DeepEqual(before, after) {
				t.Fatalf("repository changed\nbefore=%v\nafter=%v", before, after)
			}
		})
	}
}

func TestWorkflowGateUnsupportedStoreNotCalledBeforePersistence(t *testing.T) {
	const warning = "ai-dev: warning: human gate persistence is supported only on macOS and Linux"
	cases := []struct {
		name       string
		decision   gate.Decision
		args       []string
		record     string
		artifacts  []string
		wantCode   int
		wantOutput string
		wantErr    string
	}{
		{"usage", gate.Approval, []string{"--approver", "a", "f"}, "", nil, 2, "", "Usage: ai-dev workflow approve [--format text|json] --approver <identity> [--reason <text>] <feature-id> <target-state>\n"},
		{"target unsupported", gate.Approval, []string{"--approver", "a", "f", "reviewing"}, "", nil, 1, "target_unsupported", ""},
		{"feature unknown", gate.Approval, []string{"--approver", "a", "missing", "specified"}, "", nil, 1, "feature_unknown", ""},
		{"state invalid", gate.Approval, []string{"--approver", "a", "f", "specified"}, `{"state":"invalid"}`, []string{"spec.md"}, 1, "state_invalid", ""},
		{"transition invalid", gate.Approval, []string{"--approver", "a", "f", "planned"}, `{"state":"draft"}`, []string{"spec.md", "plan.md"}, 1, "transition_invalid", ""},
		{"artifacts missing", gate.Approval, []string{"--approver", "a", "f", "specified"}, `{"state":"draft"}`, nil, 1, "artifacts_missing", ""},
		{"abandon C-13 Abandoned", gate.Abandonment, []string{"--approver", "a", "--reason", "stop", "f", "Abandoned"}, "", nil, 2, "", "abandonment requires target \"abandoned\"\nUsage: ai-dev workflow abandon [--format text|json] --approver <identity> --reason <text> <feature-id> <target-state>\n"},
		{"abandon C-13 planned", gate.Abandonment, []string{"--approver", "a", "--reason", "stop", "f", "planned"}, "", nil, 2, "", "abandonment requires target \"abandoned\"\nUsage: ai-dev workflow abandon [--format text|json] --approver <identity> --reason <text> <feature-id> <target-state>\n"},
		{"abandon C-13 reviewing", gate.Abandonment, []string{"--approver", "a", "--reason", "stop", "f", "reviewing"}, "", nil, 2, "", "abandonment requires target \"abandoned\"\nUsage: ai-dev workflow abandon [--format text|json] --approver <identity> --reason <text> <feature-id> <target-state>\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := controlledTempDir(t)
			if tc.name != "feature unknown" {
				dir := filepath.Join(repo, "specs", "f")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if tc.record != "" {
					if err := os.WriteFile(filepath.Join(dir, workflow.RecordFileName), []byte(tc.record), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				for _, artifact := range tc.artifacts {
					if err := os.WriteFile(filepath.Join(dir, artifact), nil, 0o644); err != nil {
						t.Fatal(err)
					}
				}
			}
			before := snapshotDir(t, repo)
			store := &fakeGateStore{status: gate.Failed, warnings: []string{warning}}
			var out, errOut bytes.Buffer
			code := RunWorkflowGate(tc.decision, tc.args, os.DirFS(repo), store, &out, &errOut)
			if code != tc.wantCode || store.calls != 0 || strings.Contains(errOut.String(), warning) || (tc.wantOutput != "" && !strings.Contains(out.String(), tc.wantOutput)) || errOut.String() != tc.wantErr || (tc.wantCode == 2 && out.Len() != 0) {
				t.Fatalf("code=%d out=%q err=%q calls=%d", code, out.String(), errOut.String(), store.calls)
			}
			if after := snapshotDir(t, repo); !reflect.DeepEqual(before, after) {
				t.Fatalf("repository changed\nbefore=%v\nafter=%v", before, after)
			}
		})
	}
}

func TestWorkflowGateReadOnlyDirectoryFails(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permissions")
	}
	repo := controlledTempDir(t)
	dir := filepath.Join(repo, "specs", "f")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "spec.md"), nil, 0o644)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o755)
	before := snapshotDir(t, repo)
	var out, errOut bytes.Buffer
	code := RunWorkflowGate(gate.Approval, []string{"--approver", "a", "f", "specified"}, os.DirFS(repo), gatestore.New(repo), &out, &errOut)
	if code != 1 || !strings.Contains(out.String(), "persistence_failed") || !strings.Contains(out.String(), `note="no transition was applied"`) || strings.Contains(out.String(), "transition_applied") {
		t.Fatalf("code=%d out=%q", code, out.String())
	}
	if after := snapshotDir(t, repo); !reflect.DeepEqual(before, after) {
		t.Fatalf("repository changed\nbefore=%v\nafter=%v", before, after)
	}
}
