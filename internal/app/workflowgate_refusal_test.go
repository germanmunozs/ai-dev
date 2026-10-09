package app

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"ai-dev/internal/gate"
	"ai-dev/internal/gatestore"
	"ai-dev/internal/workflow"
)

func TestWorkflowGateRefusalsAreNonMutating(t *testing.T) {
	cases := []struct {
		name, id, target, record string
		files                    []string
		want                     string
	}{
		{"invalid id", "../x", "specified", "", nil, "feature_unknown"},
		{"absent", "missing", "specified", "", nil, "feature_unknown"},
		{"invalid record", "f", "planned", `{"state":"bad"}`, []string{"spec.md", "plan.md"}, "state_invalid"},
		{"same state", "f", "specified", `{"state":"specified"}`, []string{"spec.md"}, "transition_invalid"},
		{"skip before artifact", "f", "tasked", `{"state":"specified"}`, nil, "transition_invalid"},
		{"missing artifact", "f", "tasked", `{"state":"planned"}`, []string{"spec.md"}, "artifacts_missing"},
		{"unsupported before feature", "missing", "Planned", "", nil, "target_unsupported"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := controlledTempDir(t)
			if tc.id == "f" {
				dir := filepath.Join(repo, "specs", "f")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if tc.record != "" {
					if err := os.WriteFile(filepath.Join(dir, workflow.RecordFileName), []byte(tc.record), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				for _, name := range tc.files {
					if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
						t.Fatal(err)
					}
				}
			}
			before := snapshotDir(t, repo)
			var out, errOut bytes.Buffer
			code := RunWorkflowGate(gate.Approval, []string{"--approver", "a", tc.id, tc.target}, os.DirFS(repo), gatestore.New(repo), &out, &errOut)
			if code != 1 || !bytes.Contains(out.Bytes(), []byte("outcome="+tc.want)) || bytes.Contains(out.Bytes(), []byte("outcome=transition_applied")) {
				t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
			}
			if after := snapshotDir(t, repo); !reflect.DeepEqual(before, after) {
				t.Fatalf("repository changed\nbefore=%v\nafter=%v", before, after)
			}
		})
	}
}

func TestWorkflowGateRefusalInvalidRecordMatrix(t *testing.T) {
	records := map[string]string{
		"malformed":       `{`,
		"non object":      `[]`,
		"absent state":    `{}`,
		"null state":      `{"state":null}`,
		"case variant":    `{"State":"specified"}`,
		"unicode variant": `{"ſtate":"specified"}`,
		"duplicate":       `{"state":"specified","state":"specified"}`,
		"non string":      `{"state":1}`,
		"empty":           `{"state":""}`,
		"whitespace":      `{"state":" "}`,
		"wrong case":      `{"state":"Specified"}`,
	}
	for name, record := range records {
		t.Run(name, func(t *testing.T) {
			repo := controlledTempDir(t)
			dir := filepath.Join(repo, "specs", "f")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, workflow.RecordFileName), []byte(record), 0o644); err != nil {
				t.Fatal(err)
			}
			before := snapshotDir(t, repo)
			var out, errOut bytes.Buffer
			code := RunWorkflowGate(gate.Approval, []string{"--approver", "a", "f", "planned"}, os.DirFS(repo), gatestore.New(repo), &out, &errOut)
			if code != 1 || !bytes.Contains(out.Bytes(), []byte("state_invalid")) || !reflect.DeepEqual(before, snapshotDir(t, repo)) {
				t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
			}
		})
	}
	for _, kind := range []string{"directory", "symlink", "unreadable"} {
		t.Run(kind, func(t *testing.T) {
			if kind == "unreadable" && (runtime.GOOS == "windows" || os.Geteuid() == 0) {
				t.Skip("permissions")
			}
			repo := controlledTempDir(t)
			dir := filepath.Join(repo, "specs", "f")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			recordPath := filepath.Join(dir, workflow.RecordFileName)
			switch kind {
			case "directory":
				if err := os.Mkdir(recordPath, 0o755); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.WriteFile(filepath.Join(dir, "target"), []byte(`{"state":"specified"}`), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("target", recordPath); err != nil {
					t.Skip(err)
				}
			case "unreadable":
				if err := os.WriteFile(recordPath, []byte(`{"state":"specified"}`), 0o000); err != nil {
					t.Fatal(err)
				}
				defer os.Chmod(recordPath, 0o644)
			}
			if kind == "unreadable" {
				var out, errOut bytes.Buffer
				code := RunWorkflowGate(gate.Approval, []string{"--approver", "a", "f", "planned"}, os.DirFS(repo), gatestore.New(repo), &out, &errOut)
				info, err := os.Lstat(recordPath)
				if err != nil {
					t.Fatal(err)
				}
				if code != 1 || !bytes.Contains(out.Bytes(), []byte("state_invalid")) || info.Mode().Perm() != 0 {
					t.Fatalf("code=%d mode=%o out=%q err=%q", code, info.Mode().Perm(), out.String(), errOut.String())
				}
				if err := os.Chmod(recordPath, 0o644); err != nil {
					t.Fatal(err)
				}
				content, err := os.ReadFile(recordPath)
				if err != nil {
					t.Fatal(err)
				}
				if string(content) != `{"state":"specified"}` {
					t.Fatalf("record changed: %q", content)
				}
				return
			}
			before := snapshotDir(t, repo)
			var out, errOut bytes.Buffer
			code := RunWorkflowGate(gate.Approval, []string{"--approver", "a", "f", "planned"}, os.DirFS(repo), gatestore.New(repo), &out, &errOut)
			if code != 1 || !bytes.Contains(out.Bytes(), []byte("state_invalid")) || !reflect.DeepEqual(before, snapshotDir(t, repo)) {
				t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
			}
		})
	}
}

func TestWorkflowGateRefusalSymlinkAndCaseArtifact(t *testing.T) {
	t.Run("symlink feature", func(t *testing.T) {
		repo := controlledTempDir(t)
		if err := os.MkdirAll(filepath.Join(repo, "specs", "real"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("real", filepath.Join(repo, "specs", "f")); err != nil {
			t.Skip(err)
		}
		before := snapshotDir(t, repo)
		var out, errOut bytes.Buffer
		code := RunWorkflowGate(gate.Approval, []string{"--approver", "a", "f", "specified"}, os.DirFS(repo), gatestore.New(repo), &out, &errOut)
		if code != 1 || !bytes.Contains(out.Bytes(), []byte("feature_unknown")) {
			t.Fatalf("code=%d out=%q", code, out.String())
		}
		if !reflect.DeepEqual(before, snapshotDir(t, repo)) {
			t.Fatal("repository changed")
		}
	})
	t.Run("Plan md variant", func(t *testing.T) {
		repo := controlledTempDir(t)
		dir := filepath.Join(repo, "specs", "f")
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, workflow.RecordFileName), []byte(`{"state":"specified"}`), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "spec.md"), nil, 0o644)
		_ = os.WriteFile(filepath.Join(dir, "Plan.md"), nil, 0o644)
		before := snapshotDir(t, repo)
		var out, errOut bytes.Buffer
		code := RunWorkflowGate(gate.Approval, []string{"--approver", "a", "f", "planned"}, os.DirFS(repo), gatestore.New(repo), &out, &errOut)
		if code != 1 || !bytes.Contains(out.Bytes(), []byte("missing=plan.md")) {
			t.Fatalf("code=%d out=%q", code, out.String())
		}
		if !reflect.DeepEqual(before, snapshotDir(t, repo)) {
			t.Fatal("repository changed")
		}
	})
}
