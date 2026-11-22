//go:build (darwin && !ios) || (linux && !android)

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkflowGateC13BinaryMatrix(t *testing.T) {
	binary := buildWorkflowNextBinary(t)
	cases := []struct {
		verb, target string
		code         int
		token        string
		applied      bool
	}{
		{"approve", "abandoned", 2, "workflow abandon", false}, {"approve", "Abandoned", 1, "target_unsupported", false}, {"approve", "planned", 0, "transition_applied", true}, {"approve", "reviewing", 1, "target_unsupported", false},
		{"abandon", "abandoned", 0, "transition_applied", true}, {"abandon", "Abandoned", 2, "abandonment requires", false}, {"abandon", "planned", 2, "abandonment requires", false}, {"abandon", "reviewing", 2, "abandonment requires", false},
	}
	for _, tc := range cases {
		t.Run(tc.verb+"_"+tc.target, func(t *testing.T) {
			repo := makeWorkflowGateRepo(t, map[string][]string{"f": {"spec.md", "plan.md"}})
			record := filepath.Join(repo, "specs", "f", "workflow-state.json")
			if err := os.WriteFile(record, []byte(`{"state":"specified"}`), 0o644); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(record)
			args := []string{"workflow", tc.verb, "--approver", "a"}
			if tc.verb == "abandon" {
				args = append(args, "--reason", "stop")
			}
			args = append(args, "f", tc.target)
			cmd := exec.Command(binary, args...)
			cmd.Dir = repo
			stdout, stderr, code := runCommand(cmd)
			if code != tc.code || !strings.Contains(stdout+stderr, tc.token) {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			if code == 2 && stdout != "" {
				t.Fatalf("usage stdout=%q", stdout)
			}
			if !tc.applied {
				after, _ := os.ReadFile(record)
				if string(after) != string(before) {
					t.Fatal("non-applied cell changed record")
				}
			}
		})
	}
}
