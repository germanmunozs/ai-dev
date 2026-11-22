package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/gate"
	"github.com/germanmunozs/ai-dev/internal/gatestore"
	"github.com/germanmunozs/ai-dev/internal/workflow"
)

func TestWorkflowGateApplyWalk(t *testing.T) {
	repo := controlledTempDir(t)
	featureDir := filepath.Join(repo, "specs", "f")
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	steps := []struct {
		target   workflow.State
		artifact string
	}{{workflow.Specified, "spec.md"}, {workflow.Planned, "plan.md"}, {workflow.Tasked, "tasks.md"}, {workflow.Implemented, ""}, {workflow.Approved, ""}}
	for i, step := range steps {
		if step.artifact != "" {
			if err := os.WriteFile(filepath.Join(featureDir, step.artifact), []byte("artifact"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		args := []string{"--approver", "alice"}
		if i == 1 {
			args = append(args, "--reason", "plan reviewed")
		}
		args = append(args, "f", string(step.target))
		var out, errOut bytes.Buffer
		code := RunWorkflowGate(gate.Approval, args, os.DirFS(repo), gatestore.New(repo), &out, &errOut)
		if code != 0 || errOut.Len() != 0 || !bytes.Contains(out.Bytes(), []byte("outcome=transition_applied")) || !bytes.Contains(out.Bytes(), []byte("approver identity is not verified")) {
			t.Fatalf("step %s code=%d out=%q err=%q", step.target, code, out.String(), errOut.String())
		}
		report := workflow.EvaluateState(os.DirFS(repo), "f")
		if report.State != step.target {
			t.Fatalf("state=%s want %s", report.State, step.target)
		}
		data, err := os.ReadFile(filepath.Join(featureDir, workflow.RecordFileName))
		if err != nil {
			t.Fatal(err)
		}
		var object map[string]json.RawMessage
		_ = json.Unmarshal(data, &object)
		var history []json.RawMessage
		_ = json.Unmarshal(object["human_gates"], &history)
		if len(history) != i+1 {
			t.Fatalf("history=%d", len(history))
		}
	}
}
