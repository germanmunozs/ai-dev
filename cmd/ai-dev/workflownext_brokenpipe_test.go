//go:build unix

package main

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
)

func TestWorkflowNextBrokenPipe(t *testing.T) {
	bin := buildWorkflowNextBinary(t)
	repo := makeWorkflowNextRepo(t, nil)

	// Positive control also reuses the existing subprocess helper owned by
	// main_test.go before exercising direct pipe wiring.
	control := exec.Command(bin, "workflow", "next", "f")
	control.Dir = repo
	if stdout, stderr, code := runCommand(control); code != 0 || stderr != "" || !strings.Contains(stdout, "outcome=step_recommended") {
		t.Fatalf("normal control code/stdout/stderr = %d/%q/%q", code, stdout, stderr)
	}

	for _, args := range [][]string{{"workflow", "next", "f"}, {"workflow", "next", "--format", "json", "f"}} {
		t.Run("broken stdout "+strings.Join(args, " "), func(t *testing.T) {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := r.Close(); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(bin, args...)
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
			waitStatus, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
			if !ok {
				t.Fatalf("process status type = %T", cmd.ProcessState.Sys())
			}
			if waitStatus.Signaled() || cmd.ProcessState.ExitCode() != 2 || !strings.Contains(stderr.String(), "write workflow next report:") {
				t.Fatalf("wait=%v signaled=%v exit=%d stderr=%q", waitErr, waitStatus.Signaled(), cmd.ProcessState.ExitCode(), stderr.String())
			}
		})
	}

	t.Run("broken stderr usage", func(t *testing.T) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(bin, "workflow", "next")
		cmd.Dir = repo
		cmd.Stderr = w
		var stdout strings.Builder
		cmd.Stdout = &stdout
		if err := cmd.Start(); err != nil {
			_ = w.Close()
			t.Fatal(err)
		}
		_ = w.Close()
		waitErr := cmd.Wait()
		waitStatus, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
		if !ok {
			t.Fatalf("process status type = %T", cmd.ProcessState.Sys())
		}
		if waitStatus.Signaled() || cmd.ProcessState.ExitCode() != 2 || stdout.Len() != 0 {
			t.Fatalf("wait=%v signaled=%v exit=%d stdout=%q", waitErr, waitStatus.Signaled(), cmd.ProcessState.ExitCode(), stdout.String())
		}
	})

	t.Run("workflow state isolation", func(t *testing.T) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(bin, "workflow", "state", "f")
		cmd.Dir = repo
		cmd.Stdout = w
		cmd.Stderr = &strings.Builder{}
		if err := cmd.Start(); err != nil {
			_ = w.Close()
			t.Fatal(err)
		}
		_ = w.Close()
		waitErr := cmd.Wait()
		waitStatus, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
		if !ok {
			t.Fatalf("process status type = %T", cmd.ProcessState.Sys())
		}
		if !waitStatus.Signaled() || waitStatus.Signal() != syscall.SIGPIPE {
			t.Fatalf("wait=%v status=%v, want SIGPIPE", waitErr, waitStatus)
		}
	})
}
