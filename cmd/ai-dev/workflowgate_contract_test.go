//go:build (darwin && !ios) || (linux && !android)

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func makeWorkflowGateRepo(t *testing.T, features map[string][]string) string {
	t.Helper()
	repo := t.TempDir()
	for id, files := range features {
		dir := filepath.Join(repo, "specs", id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, name := range files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("artifact"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return repo
}

func TestWorkflowGateContractWalk(t *testing.T) {
	binary := buildWorkflowNextBinary(t)
	repo := makeWorkflowGateRepo(t, map[string][]string{"f": {"spec.md", "plan.md", "tasks.md"}})
	steps := []struct {
		from, target string
		nextOutcome  string
		nextActivity string
		nextRole     string
	}{
		{"draft", "specified", "step_recommended", "plan", "planning"},
		{"specified", "planned", "step_recommended", "tasks", "planning"},
		{"planned", "tasked", "step_recommended", "implement", "implementation"},
		{"tasked", "implemented", "step_recommended", "review", "review"},
		{"implemented", "approved", "workflow_terminal", "", ""},
	}
	for i, step := range steps {
		target := step.target
		cmd := exec.Command(binary, "workflow", "approve", "--format", "json", "--approver", "alice", "f", target)
		cmd.Dir = repo
		stdout, stderr, code := runCommand(cmd)
		if code != 0 || stderr != "" {
			t.Fatalf("step %s code=%d stdout=%q stderr=%q", target, code, stdout, stderr)
		}
		var report map[string]any
		if err := json.Unmarshal([]byte(stdout), &report); err != nil {
			t.Fatal(err)
		}
		wantReport := map[string]any{
			"feature_id": "f", "outcome": "transition_applied", "decision": "approval", "approver": "alice",
			"from_state": step.from, "supplied_target": target, "target": target,
		}
		if !reflect.DeepEqual(report, wantReport) {
			t.Fatalf("report=%v want=%v", report, wantReport)
		}
		stateCmd := exec.Command(binary, "workflow", "state", "--format", "json", "f")
		stateCmd.Dir = repo
		stateOut, stateErr, stateCode := runCommand(stateCmd)
		if stateCode != 0 || stateErr != "" {
			t.Fatalf("state code=%d err=%q", stateCode, stateErr)
		}
		var state map[string]any
		_ = json.Unmarshal([]byte(stateOut), &state)
		if state["state"] != target {
			t.Fatalf("state=%v", state)
		}

		nextCmd := exec.Command(binary, "workflow", "next", "--format", "json", "f")
		nextCmd.Dir = repo
		nextOut, nextErr, nextCode := runCommand(nextCmd)
		if nextCode != 0 || nextErr != "" {
			t.Fatalf("next code=%d out=%q err=%q", nextCode, nextOut, nextErr)
		}
		var next map[string]any
		if err := json.Unmarshal([]byte(nextOut), &next); err != nil {
			t.Fatal(err)
		}
		wantNext := map[string]any{"feature_id": "f", "outcome": step.nextOutcome, "current_state": target}
		if step.nextOutcome == "step_recommended" {
			wantNext["activity"] = step.nextActivity
			wantNext["role"] = step.nextRole
		}
		if !reflect.DeepEqual(next, wantNext) {
			t.Fatalf("next=%v want=%v", next, wantNext)
		}

		recordData, err := os.ReadFile(filepath.Join(repo, "specs", "f", "workflow-state.json"))
		if err != nil {
			t.Fatal(err)
		}
		var record struct {
			State      string           `json:"state"`
			HumanGates []map[string]any `json:"human_gates"`
		}
		if err := json.Unmarshal(recordData, &record); err != nil {
			t.Fatal(err)
		}
		if record.State != target || len(record.HumanGates) != i+1 {
			t.Fatalf("record state=%q history=%d want state=%q history=%d", record.State, len(record.HumanGates), target, i+1)
		}
		for historyIndex, entry := range record.HumanGates {
			if entry["feature_id"] != "f" || entry["decision"] != "approval" || entry["approver"] != "alice" || entry["to_state"] != steps[historyIndex].target {
				t.Fatalf("history[%d]=%v", historyIndex, entry)
			}
		}
	}
}
