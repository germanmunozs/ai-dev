package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/germanmunozs/ai-dev/internal/gate"
	"github.com/germanmunozs/ai-dev/internal/gatestore"
	"github.com/germanmunozs/ai-dev/internal/sdd"
	"github.com/germanmunozs/ai-dev/internal/workflow"
)

func TestWorkflowGateAbandonEveryNonTerminalState(t *testing.T) {
	states := []workflow.State{workflow.Draft, workflow.Specified, workflow.Planned, workflow.Tasked, workflow.Implemented}
	for _, state := range states {
		t.Run(string(state), func(t *testing.T) {
			repo := controlledTempDir(t)
			dir := filepath.Join(repo, "specs", "f")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if state != workflow.Draft {
				if err := os.WriteFile(filepath.Join(dir, workflow.RecordFileName), []byte(`{"state":"`+string(state)+`"}`), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var out, errOut bytes.Buffer
			code := RunWorkflowGate(gate.Abandonment, []string{"--approver", "bob", "--reason", "superseded\nexact", "f", "abandoned"}, os.DirFS(repo), gatestore.New(repo), &out, &errOut)
			if code != 0 || errOut.Len() != 0 || !strings.Contains(out.String(), "decision=abandonment") || !strings.Contains(out.String(), `reason="superseded\nexact"`) {
				t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
			}
			stateReport := workflow.EvaluateState(os.DirFS(repo), "f")
			if stateReport.State != workflow.Abandoned || !stateReport.Terminal {
				t.Fatalf("state=%+v", stateReport)
			}
			if next := sdd.Evaluate(os.DirFS(repo), "f"); next.Outcome != sdd.WorkflowTerminal {
				t.Fatalf("next=%+v", next)
			}
			data, _ := os.ReadFile(filepath.Join(dir, workflow.RecordFileName))
			var object map[string]json.RawMessage
			_ = json.Unmarshal(data, &object)
			var history []map[string]any
			_ = json.Unmarshal(object["human_gates"], &history)
			if len(history) != 1 || history[0]["decision"] != "abandonment" || history[0]["reason"] != "superseded\nexact" {
				t.Fatalf("history=%v", history)
			}
		})
	}
}

func TestWorkflowGateAbandonUsageAndTerminal(t *testing.T) {
	for _, reasonArgs := range [][]string{nil, {"--reason", ""}, {"--reason", " "}} {
		store := &fakeGateStore{status: gate.Committed}
		args := []string{"--approver", "b"}
		args = append(args, reasonArgs...)
		args = append(args, "f", "abandoned")
		var out, errOut bytes.Buffer
		code := runWorkflowGate(gate.Abandonment, args, appGateFixture(workflow.Draft), store, time.Now, &out, &errOut)
		if code != 2 || out.Len() != 0 || store.calls != 0 {
			t.Fatalf("args=%v code=%d out=%q calls=%d", args, code, out.String(), store.calls)
		}
	}
	for _, state := range []workflow.State{workflow.Approved, workflow.Abandoned} {
		var out, errOut bytes.Buffer
		store := &fakeGateStore{status: gate.Committed}
		code := runWorkflowGate(gate.Abandonment, []string{"--approver", "b", "--reason", "stop", "f", "abandoned"}, appGateFixture(state), store, time.Now, &out, &errOut)
		if code != 1 || !strings.Contains(out.String(), "transition_invalid") || store.calls != 0 {
			t.Fatalf("state=%s code=%d out=%q calls=%d", state, code, out.String(), store.calls)
		}
	}
}
