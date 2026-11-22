//go:build (darwin && !ios) || (linux && !android)

package app

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// buildSetupBinary builds ai-dev for the pseudo-terminal tests.
func buildSetupBinary(t *testing.T) string {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "ai-dev")
	build := exec.Command("go", "build", "-o", bin, "./cmd/ai-dev")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build ai-dev: %v\n%s", err, output)
	}
	return bin
}

// fakeAgentPath holds claude and codex scripts that record any invocation.
func fakeAgentPath(t *testing.T) (dir, log string) {
	t.Helper()
	dir = t.TempDir()
	log = filepath.Join(t.TempDir(), "invocations.log")
	for _, name := range []string{"claude", "codex"} {
		script := "#!/bin/sh\necho " + name + " >> '" + log + "'\nexit 1\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir, log
}

type setupTerminal struct {
	t      *testing.T
	cmd    *exec.Cmd
	master *os.File
	mu     sync.Mutex
	output bytes.Buffer
}

// startSetupTerminal runs "ai-dev setup" as a session leader whose
// controlling terminal is the pseudo-terminal, as in a real shell.
func startSetupTerminal(t *testing.T, bin, project, agents string) *setupTerminal {
	master, slave := openPTY(t)
	cmd := exec.Command(bin, "setup")
	cmd.Dir = project
	cmd.Env = []string{"HOME=" + t.TempDir(), "TERM=xterm", "PATH=" + agents}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	slave.Close()
	s := &setupTerminal{t: t, cmd: cmd, master: master}
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, err := master.Read(buffer)
			s.mu.Lock()
			s.output.Write(buffer[:n])
			s.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		master.Close()
	})
	return s
}

func (s *setupTerminal) waitFor(want string) {
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		found := strings.Contains(s.output.String(), want)
		s.mu.Unlock()
		if found {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.t.Fatalf("never printed %q; output:\n%s", want, s.output.String())
}

func (s *setupTerminal) send(text string) {
	if _, err := s.master.WriteString(text); err != nil {
		s.t.Fatal(err)
	}
}

func (s *setupTerminal) exitCode() int {
	err := s.cmd.Wait()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	if err != nil {
		s.t.Fatal(err)
	}
	return 0
}

// TestSetupWizardInPTY drives the real binary in a terminal: "yes" writes
// the file; "no" and Ctrl-C leave the directory byte-identical (SC-003); no
// agent is ever started (SC-002).
func TestSetupWizardInPTY(t *testing.T) {
	bin := buildSetupBinary(t)
	agents, log := fakeAgentPath(t)

	t.Run("yes", func(t *testing.T) {
		project := t.TempDir()
		s := startSetupTerminal(t, bin, project, agents)
		s.waitFor("Which agents does this project work with?")
		s.send("1,2\n1\n1\n2\n1\n2\nyes\n")
		s.waitFor("outcome=configuration_written source=project")
		if code := s.exitCode(); code != 0 {
			t.Fatalf("exit %d", code)
		}
		data, err := os.ReadFile(filepath.Join(project, ".ai-dev", "agents.json"))
		if err != nil || !strings.Contains(string(data), `"implementation": "codex-cli"`) {
			t.Fatalf("config: %q %v", data, err)
		}
	})

	for name, input := range map[string]string{"no": "1\n1\n1\n1\n1\n1\nno\n", "ctrl-c": "\x03"} {
		t.Run(name, func(t *testing.T) {
			project := t.TempDir()
			before := snapshotDir(t, project)
			s := startSetupTerminal(t, bin, project, agents)
			s.waitFor("Which agents does this project work with?")
			s.send(input)
			s.waitFor("outcome=setup_cancelled")
			if code := s.exitCode(); code != 1 {
				t.Fatalf("exit %d", code)
			}
			if after := snapshotDir(t, project); !reflect.DeepEqual(after, before) {
				t.Fatalf("directory changed: %v", after)
			}
		})
	}

	if data, err := os.ReadFile(log); err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an agent was started: %q", data)
	}
}
