package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"ai-dev/internal/gate"
	"ai-dev/internal/gatestore"
	"ai-dev/internal/system"
	"ai-dev/internal/workflow"
)

type authorityLauncher struct{ repo string }

func (l authorityLauncher) Launch(_ context.Context, _ system.Spec) (system.Handle, error) {
	_ = os.WriteFile(filepath.Join(l.repo, "specs", "f", "plan.md"), []byte("plan"), 0o644)
	_ = os.WriteFile(filepath.Join(l.repo, "specs", "f", "tasks.md"), []byte("tasks"), 0o644)
	return appRunHandle{status: system.ExitStatus{Exited: true, ExitCode: 0}}, nil
}

func TestAuthorityExistingCapabilitiesDoNotApprove(t *testing.T) {
	repo := controlledTempDir(t)
	dir := filepath.Join(repo, "specs", "f")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	recordPath := filepath.Join(dir, workflow.RecordFileName)
	original := []byte(`{"state":"specified"}`)
	if err := os.WriteFile(recordPath, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "spec.md"), []byte("spec"), 0o644); err != nil {
		t.Fatal(err)
	}
	executor := &fakeExecutor{paths: map[string]string{"git": "/git", "specify": "/specify", "claude": "/claude", "codex": "/codex"}}
	launcher := authorityLauncher{repo: repo}
	commands := [][]string{{"doctor"}, {"agents"}, {"agent", "select", "implementation"}, {"agent", "run", "claude-code", "x"}, {"workflow", "state", "f"}, {"workflow", "transition", "f", "planned"}, {"workflow", "next", "f"}}
	for _, args := range commands {
		var out, errOut bytes.Buffer
		_ = RunWithGateStore(context.Background(), args, executor, launcher, os.DirFS(repo), gatestore.New(repo), bytes.NewReader(nil), &out, &errOut)
	}
	after, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, original) {
		t.Fatalf("record changed: %s", after)
	}
}

func TestAuthorityReplayIsTransitionInvalid(t *testing.T) {
	repo := controlledTempDir(t)
	dir := filepath.Join(repo, "specs", "f")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "spec.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{"--approver", "a", "f", "specified"}
	var out, errOut bytes.Buffer
	if code := RunWorkflowGate(gate.Approval, args, os.DirFS(repo), gatestore.New(repo), &out, &errOut); code != 0 {
		t.Fatalf("first code=%d err=%q", code, errOut.String())
	}
	recordPath := filepath.Join(dir, workflow.RecordFileName)
	before, _ := os.ReadFile(recordPath)
	out.Reset()
	errOut.Reset()
	if code := RunWorkflowGate(gate.Approval, args, os.DirFS(repo), gatestore.New(repo), &out, &errOut); code != 1 || !bytes.Contains(out.Bytes(), []byte("transition_invalid")) {
		t.Fatalf("replay code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
	after, _ := os.ReadFile(recordPath)
	if !bytes.Equal(before, after) {
		t.Fatal("replay changed record")
	}
}
