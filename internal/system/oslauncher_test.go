package system

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestOSLauncherPropagatesDirectoryArgumentsAndStreams(t *testing.T) {
	requirePOSIXShell(t)

	workdir := t.TempDir()
	var stdout, stderr bytes.Buffer
	handle, err := (OSLauncher{}).Launch(context.Background(), Spec{
		Name:   fixturePath(t, "exit-agent.sh"),
		Args:   []string{"0", "hello world", "$HOME", "line one\nline two"},
		Dir:    workdir,
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}
	status, err := handle.Wait()
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if !status.Exited || status.ExitCode != 0 {
		t.Fatalf("status = %#v, want normal exit 0", status)
	}
	wantLines := []string{
		"cwd=" + workdir,
		"arg=hello world",
		"arg=$HOME",
		"arg=line one\nline two",
	}
	for _, want := range wantLines {
		if !strings.Contains(stdout.String(), want+"\n") {
			t.Errorf("stdout %q missing %q", stdout.String(), want)
		}
	}
	if stderr.String() != "fixture-stderr\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestOSLauncherReportsNormalNonZeroAndAbnormalTermination(t *testing.T) {
	requirePOSIXShell(t)

	t.Run("normal non-zero", func(t *testing.T) {
		handle, err := (OSLauncher{}).Launch(context.Background(), Spec{
			Name: fixturePath(t, "exit-agent.sh"),
			Args: []string{"7"},
		})
		if err != nil {
			t.Fatalf("Launch() error = %v", err)
		}
		status, err := handle.Wait()
		if err == nil {
			t.Fatal("Wait() error = nil, want exit error")
		}
		if !status.Exited || status.ExitCode != 7 {
			t.Fatalf("status = %#v, want normal exit 7", status)
		}
	})

	t.Run("terminated by signal", func(t *testing.T) {
		handle, err := (OSLauncher{}).Launch(context.Background(), Spec{
			Name: "sh",
			Args: []string{"-c", "kill -TERM $$"},
		})
		if err != nil {
			t.Fatalf("Launch() error = %v", err)
		}
		status, err := handle.Wait()
		if err == nil {
			t.Fatal("Wait() error = nil, want signal error")
		}
		if status.Exited {
			t.Fatalf("status = %#v, want abnormal termination", status)
		}
	})
}

func TestOSLauncherStreamsMarkersBeforeProcessExit(t *testing.T) {
	requirePOSIXShell(t)

	temp := t.TempDir()
	releaseFile := filepath.Join(temp, "release")
	pidFile := filepath.Join(temp, "pid")
	stdout := newObservedWriter("stdout-before-exit\n")
	stderr := newObservedWriter("stderr-before-exit\n")
	handle, err := (OSLauncher{}).Launch(context.Background(), Spec{
		Name:   fixturePath(t, "streaming-agent.sh"),
		Args:   []string{releaseFile, pidFile, "0"},
		Stdout: stdout,
		Stderr: stderr,
	})
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}

	waitObserved(t, stdout.observed, "stdout marker")
	waitObserved(t, stderr.observed, "stderr marker")
	if err := os.WriteFile(releaseFile, []byte("release\n"), 0o600); err != nil {
		t.Fatalf("release fixture: %v", err)
	}
	status, err := handle.Wait()
	if err != nil || !status.Exited || status.ExitCode != 0 {
		t.Fatalf("Wait() = %#v, %v; want normal exit 0", status, err)
	}
}

func TestOSLauncherCancellationStopsChildBeforeWaitReturns(t *testing.T) {
	requirePOSIXShell(t)

	temp := t.TempDir()
	releaseFile := filepath.Join(temp, "never-release")
	pidFile := filepath.Join(temp, "pid")
	stdout := newObservedWriter("stdout-before-exit\n")
	ctx, cancel := context.WithCancel(context.Background())
	handle, err := (OSLauncher{}).Launch(ctx, Spec{
		Name:   fixturePath(t, "streaming-agent.sh"),
		Args:   []string{releaseFile, pidFile, "0"},
		Stdout: stdout,
	})
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}
	waitObserved(t, stdout.observed, "stdout marker")
	pid := waitForPID(t, pidFile)

	cancel()
	done := make(chan struct{})
	var status ExitStatus
	var waitErr error
	go func() {
		status, waitErr = handle.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(7 * time.Second):
		t.Fatal("Wait() did not return after cancellation")
	}
	if waitErr == nil {
		t.Fatal("Wait() error = nil after cancellation")
	}
	if !status.Canceled {
		t.Fatalf("status = %#v, want cancellation recorded", status)
	}
	if status.Exited && status.ExitCode == 0 {
		t.Fatalf("status = %#v after cancellation", status)
	}
	if err := syscall.Kill(pid, 0); err == nil || !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("child pid %d still exists after Wait; signal check error = %v", pid, err)
	}
}

type observedWriter struct {
	mu       sync.Mutex
	buffer   bytes.Buffer
	marker   string
	observed chan struct{}
	once     sync.Once
}

func newObservedWriter(marker string) *observedWriter {
	return &observedWriter{marker: marker, observed: make(chan struct{})}
}

func (w *observedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.buffer.Write(p)
	if strings.Contains(w.buffer.String(), w.marker) {
		w.once.Do(func() { close(w.observed) })
	}
	return n, err
}

func waitObserved(t *testing.T, observed <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-observed:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s was not observable before fixture release", name)
	}
}

func waitForPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		contents, err := os.ReadFile(path)
		if err == nil {
			pid, err := strconv.Atoi(strings.TrimSpace(string(contents)))
			if err != nil {
				t.Fatalf("parse fixture pid: %v", err)
			}
			return pid
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("fixture did not write pid file %s", path)
	return 0
}

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("fixture path: %v", err)
	}
	return path
}

func requirePOSIXShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell fixture")
	}
}
