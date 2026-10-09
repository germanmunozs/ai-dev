//go:build (darwin && !ios) || (linux && !android)

package main

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
)

func TestWorkflowGateBrokenPipe(t *testing.T) {
	binary := buildWorkflowNextBinary(t)
	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			repo := makeWorkflowGateRepo(t, map[string][]string{"f": {"spec.md"}})
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := r.Close(); err != nil {
				t.Fatal(err)
			}
			args := []string{"workflow", "approve", "--approver", "a"}
			if format == "json" {
				args = append(args, "--format", "json")
			}
			args = append(args, "f", "specified")
			cmd := exec.Command(binary, args...)
			cmd.Dir = repo
			cmd.Stdout = w
			var stderr strings.Builder
			cmd.Stderr = &stderr
			if err := cmd.Start(); err != nil {
				_ = w.Close()
				t.Fatal(err)
			}
			_ = w.Close()
			waitErr := cmd.Wait()
			status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
			if !ok {
				t.Fatalf("status=%T", cmd.ProcessState.Sys())
			}
			if status.Signaled() || cmd.ProcessState.ExitCode() != 2 || !strings.Contains(stderr.String(), "write workflow gate report:") {
				t.Fatalf("wait=%v status=%v exit=%d stderr=%q", waitErr, status, cmd.ProcessState.ExitCode(), stderr.String())
			}
			state := exec.Command(binary, "workflow", "state", "f")
			state.Dir = repo
			stdout, stateErr, code := runCommand(state)
			if code != 0 || stateErr != "" || !strings.Contains(stdout, "state=specified") {
				t.Fatalf("state code=%d out=%q err=%q", code, stdout, stateErr)
			}
		})
	}
}
