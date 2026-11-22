//go:build (darwin && !ios) || (linux && !android)

package app

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// Hostile feature names (R-05): an OSC 52 clipboard write and an OSC 8
// hyperlink. A directory name cannot hold "/", so the link has none.
const (
	hostileClipboard = "004-\x1b]52;c;aGk=\x07"
	hostileLink      = "005-\x1b]8;;http:x\x1b\\link\x1b]8;;\x1b\\"
)

type cockpitTerminal struct {
	t      *testing.T
	cmd    *exec.Cmd
	master *os.File
	slave  *os.File
	before syscall.Termios
	mu     sync.Mutex
	output bytes.Buffer
	waited bool
	code   int
}

func terminalAttributes(t *testing.T, file *os.File) syscall.Termios {
	t.Helper()
	var attributes syscall.Termios
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), uintptr(terminalAttributesRequest), uintptr(unsafe.Pointer(&attributes))); errno != 0 {
		t.Fatalf("read terminal attributes: %v", errno)
	}
	return attributes
}

// cockpitProject is a project with one feature and the two hostile names.
func cockpitProject(t *testing.T) string {
	t.Helper()
	project := t.TempDir()
	for _, name := range []string{"001-alpha", hostileClipboard, hostileLink} {
		dir := filepath.Join(project, "specs", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "spec.md"), []byte("# Spec\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return project
}

// startCockpit runs "ai-dev tui" as a session leader on a pseudo-terminal.
// The attributes are read through the master: macOS revokes the slave
// when the session leader exits.
func startCockpit(t *testing.T, bin, project, agents string) *cockpitTerminal {
	t.Helper()
	master, slave := openPTY(t)
	c := &cockpitTerminal{t: t, master: master, slave: slave, before: terminalAttributes(t, master)}
	c.cmd = exec.Command(bin, "tui")
	c.cmd.Dir = project
	c.cmd.Env = []string{"HOME=" + t.TempDir(), "TERM=xterm-256color", "PATH=" + agents + ":/usr/bin:/bin"}
	c.cmd.Stdin, c.cmd.Stdout, c.cmd.Stderr = slave, slave, slave
	c.cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := c.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, err := master.Read(buffer)
			c.mu.Lock()
			c.output.Write(buffer[:n])
			c.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		if !c.waited {
			_ = c.cmd.Process.Kill()
			_ = c.cmd.Wait()
		}
		slave.Close()
		master.Close()
	})
	return c
}

func (c *cockpitTerminal) snapshot() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.output.String()
}

func (c *cockpitTerminal) waitFor(want string) {
	c.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(c.snapshot(), want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	c.t.Fatalf("never printed %q; output:\n%q", want, c.snapshot())
}

// mark returns the output length, so waitAfter ignores earlier output.
func (c *cockpitTerminal) mark() int { return len(c.snapshot()) }

func (c *cockpitTerminal) waitAfter(mark int, want string) {
	c.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(c.snapshot()[mark:], want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	c.t.Fatalf("never printed %q after mark; output:\n%q", want, c.snapshot()[mark:])
}

func (c *cockpitTerminal) send(text string) {
	c.t.Helper()
	if _, err := c.master.WriteString(text); err != nil {
		c.t.Fatal(err)
	}
	// Let the program read each burst on its own, as a person types.
	time.Sleep(150 * time.Millisecond)
}

func (c *cockpitTerminal) wait() int {
	c.t.Helper()
	done := make(chan error, 1)
	go func() { done <- c.cmd.Wait() }()
	select {
	case err := <-done:
		c.waited = true
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			c.code = exit.ExitCode()
		} else if err != nil {
			c.t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		c.t.Fatalf("the cockpit did not exit; output:\n%q", c.snapshot())
	}
	return c.code
}

// assertRestored checks SC-004: attributes as before, alternate screen
// left and cursor shown.
func (c *cockpitTerminal) assertRestored() {
	c.t.Helper()
	if after := terminalAttributes(c.t, c.master); after != c.before {
		c.t.Errorf("terminal attributes not restored:\nbefore %+v\nafter  %+v", c.before, after)
	}
	output := c.snapshot()
	for _, want := range []string{"\x1b[?1049l", "\x1b[?25h"} {
		if !strings.Contains(output, want) {
			c.t.Errorf("output lacks %q", want)
		}
	}
}

// TestCockpitRestoresTheTerminalInPTY proves F012-FR-002 and SC-004 for
// every exit path, and SC-005 for hostile feature names on the real
// terminal.
func TestCockpitRestoresTheTerminalInPTY(t *testing.T) {
	bin := buildSetupBinary(t)
	agents, log := fakeAgentPath(t)
	cases := []struct {
		name string
		quit func(c *cockpitTerminal)
		code int
	}{
		{"q", func(c *cockpitTerminal) { c.send("q") }, 0},
		{"Ctrl-C", func(c *cockpitTerminal) { c.send("\x03") }, 0},
		{"SIGINT", func(c *cockpitTerminal) { _ = c.cmd.Process.Signal(os.Interrupt) }, 130},
		{"SIGTERM", func(c *cockpitTerminal) { _ = c.cmd.Process.Signal(syscall.SIGTERM) }, 143},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := startCockpit(t, bin, cockpitProject(t), agents)
			c.waitFor("ai-dev tui · Dashboard")
			// The attribute check sees the mode change, so equality after
			// the exit is meaningful.
			if terminalAttributes(t, c.master) == c.before {
				t.Fatal("raw mode is not observable through the master")
			}
			c.send("\t")
			c.waitFor("001-alpha")
			c.waitFor(`004-\x1b]52;c;aGk=\x07`)
			tc.quit(c)
			if code := c.wait(); code != tc.code {
				t.Fatalf("exit %d, want %d; output:\n%q", code, tc.code, c.snapshot())
			}
			c.assertRestored()
			output := c.snapshot()
			for _, raw := range []string{"\x1b]52;", "\x1b]8;;http:x"} {
				if strings.Contains(output, raw) {
					t.Errorf("a hostile sequence reached the terminal raw: %q", raw)
				}
			}
			if !strings.Contains(output, `005-\x1b]8;;http:x\x1b\link`) {
				t.Errorf("the hyperlink name is not shown escaped")
			}
		})
	}
	if data, err := os.ReadFile(log); err == nil && len(data) != 0 {
		t.Fatalf("an agent was started: %q", data)
	}
}

// fakeClaudePath holds a claude that prints each argument on its own line,
// or, with sleep, waits to be interrupted.
func fakeClaudePath(t *testing.T, sleep bool) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nfor a in \"$@\"; do printf 'arg:%s\\n' \"$a\"; done\n"
	if sleep {
		script += "trap 'echo fake-interrupted; exit 130' INT\necho fake-started\nsleep 30 &\nwait $!\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// runAgentFromCockpit opens the Features area and starts "run agent" on
// 001-alpha with the instruction, up to the confirmation.
func runAgentFromCockpit(c *cockpitTerminal, instruction string) {
	c.waitFor("ai-dev tui · Dashboard")
	c.send("\t")
	c.waitFor("001-alpha")
	c.send("\r")
	c.waitFor("Run the agent of the next step (available)")
	c.send("\r")
	c.waitFor("instruction (required)")
	c.send(instruction)
	c.send("\r")
	c.waitFor("[ Confirm ]")
	c.send("\t")
	c.send("\r")
}

// TestCockpitRunsTheAgentInPTY proves SC-003 and SC-007 on the real
// terminal: the fake agent receives the instruction as one argument, its
// output reaches the terminal, the exit code is shown, Enter returns to
// the same feature, and the terminal is restored on quit.
func TestCockpitRunsTheAgentInPTY(t *testing.T) {
	bin := buildSetupBinary(t)
	c := startCockpit(t, bin, cockpitProject(t), fakeClaudePath(t, false))
	started := time.Now()
	runAgentFromCockpit(c, `-x "a b"`)
	c.waitFor("arg:--print\r\narg:--\r\narg:-x \"a b\"\r\n")
	c.waitFor("ai-dev tui: the command exited with code 0. Press Enter to return.")
	t.Logf("from opening to the agent's output: %v (scripted keys)", time.Since(started))
	mark := c.mark()
	c.send("\r")
	c.waitAfter(mark, "The command exited with code 0.")
	c.waitAfter(mark, "Run the agent of the next step (available)")
	c.waitAfter(mark, "001-alpha")
	c.send("\x03")
	if code := c.wait(); code != 0 {
		t.Fatalf("exit %d", code)
	}
	c.assertRestored()
}

// TestCockpitInterruptReachesTheCommandInPTY proves R-04: Ctrl-C while the
// command runs reaches the command, the cockpit ignores it, and after Enter
// it resumes with every area evaluated again, none failed.
func TestCockpitInterruptReachesTheCommandInPTY(t *testing.T) {
	bin := buildSetupBinary(t)
	c := startCockpit(t, bin, cockpitProject(t), fakeClaudePath(t, true))
	runAgentFromCockpit(c, "wait")
	c.waitFor("fake-started")
	c.send("\x03") // the line discipline sends SIGINT to the foreground group
	c.waitFor("fake-interrupted")
	c.waitFor("Press Enter to return.")
	mark := c.mark()
	c.send("\r")
	// The renderer redraws only changed cells, so wait for whole lines
	// that are new after the return.
	c.waitAfter(mark, "The command exited with code")
	c.send("\x1b") // back to the list
	c.send("\t")
	c.send("\t")
	c.waitAfter(mark, "Overall:")
	if strings.Contains(c.snapshot()[mark:], "failed") {
		t.Fatalf("an area failed after the interrupt:\n%q", c.snapshot()[mark:])
	}
	c.send("\x03")
	if code := c.wait(); code != 0 {
		t.Fatalf("exit %d; the cockpit did not survive the command's interrupt", code)
	}
	c.assertRestored()
}

var decidedAt = regexp.MustCompile(`"decided_at": "[^"]*"`)

func readRecord(t *testing.T, project string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(project, "specs", "001-alpha", "workflow-state.json"))
	if errors.Is(err, os.ErrNotExist) {
		return "absent"
	}
	if err != nil {
		t.Fatal(err)
	}
	return decidedAt.ReplaceAllString(string(data), `"decided_at": "T"`)
}

// typed runs the same command by hand in the twin project.
func typed(t *testing.T, bin, project string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = project
	cmd.Env = []string{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin"}
	output, err := cmd.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return string(output), exit.ExitCode()
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(output), 0
}

// decideFromCockpit opens the action at offset in 001-alpha's detail, fills
// the values and confirms.
func decideFromCockpit(c *cockpitTerminal, offset int, values ...string) {
	c.send("\r")
	c.waitFor("Approve the feature")
	for range offset {
		c.send("j")
	}
	c.send("\r")
	c.waitFor("approver (required)")
	for _, value := range values {
		c.send(value)
		c.send("\r")
	}
	c.waitFor("[ Confirm ]")
	c.send("\t")
	c.send("\r")
}

// TestCockpitRecordsDecisionsLikeTheCommandInPTY proves SC-003 for US4: an
// approval and an abandonment from the cockpit leave the record, output and
// exit code of the same commands typed by hand; a refused approval leaves
// the record unchanged and shows only the command's own outcome.
func TestCockpitRecordsDecisionsLikeTheCommandInPTY(t *testing.T) {
	bin := buildSetupBinary(t)
	agents, _ := fakeAgentPath(t)

	t.Run("approve then abandon", func(t *testing.T) {
		project, twin := cockpitProject(t), cockpitProject(t)
		c := startCockpit(t, bin, project, agents)
		c.waitFor("ai-dev tui · Dashboard")
		c.send("\t")
		c.waitFor("001-alpha")

		decideFromCockpit(c, 1, "Germán", "specified", "")
		c.waitFor("Press Enter to return.")
		output, code := typed(t, bin, twin, "workflow", "approve", "--approver=Germán", "--", "001-alpha", "specified")
		if code != 0 || !strings.Contains(c.snapshot(), strings.TrimSuffix(output, "\n")) || !strings.Contains(c.snapshot(), "exited with code 0.") {
			t.Fatalf("approve: typed %q (exit %d); cockpit output:\n%q", output, code, c.snapshot())
		}
		if got, want := readRecord(t, project), readRecord(t, twin); got != want {
			t.Fatalf("approve record:\n%s\nwant\n%s", got, want)
		}
		mark := c.mark()
		c.send("\r")
		c.waitAfter(mark, "The command exited with code 0.")
		c.send("\x1b")

		decideFromCockpit(c, 2, "Germán", "out of scope")
		c.waitAfter(mark, "Press Enter to return.")
		output, code = typed(t, bin, twin, "workflow", "abandon", "--approver=Germán", "--reason=out of scope", "--", "001-alpha", "abandoned")
		if code != 0 || !strings.Contains(c.snapshot()[mark:], strings.TrimSuffix(output, "\n")) {
			t.Fatalf("abandon: typed %q (exit %d); cockpit output:\n%q", output, code, c.snapshot()[mark:])
		}
		if got, want := readRecord(t, project), readRecord(t, twin); got != want {
			t.Fatalf("abandon record:\n%s\nwant\n%s", got, want)
		}
		c.send("\r")
		c.send("\x03")
		c.wait()
		c.assertRestored()
	})

	t.Run("refused approve", func(t *testing.T) {
		project, twin := cockpitProject(t), cockpitProject(t)
		c := startCockpit(t, bin, project, agents)
		c.waitFor("ai-dev tui · Dashboard")
		c.send("\t")
		c.waitFor("001-alpha")
		decideFromCockpit(c, 1, "x", "bogus", "")
		c.waitFor("Press Enter to return.")
		output, code := typed(t, bin, twin, "workflow", "approve", "--approver=x", "--", "001-alpha", "bogus")
		if code != 1 || !strings.Contains(c.snapshot(), strings.TrimSuffix(output, "\n")) || !strings.Contains(c.snapshot(), "exited with code 1.") {
			t.Fatalf("typed %q (exit %d); cockpit output:\n%q", output, code, c.snapshot())
		}
		if readRecord(t, project) != "absent" {
			t.Fatalf("a refused approval wrote the record: %s", readRecord(t, project))
		}
		mark := c.mark()
		c.send("\r")
		c.waitAfter(mark, "The command exited with code 1.")
		if strings.Contains(c.snapshot()[mark:], "approved") || strings.Contains(c.snapshot()[mark:], "success") {
			t.Fatalf("the cockpit reported an outcome of its own:\n%q", c.snapshot()[mark:])
		}
		c.send("\x03")
		c.wait()
		c.assertRestored()
	})
}

// TestCockpitSurvivesARepeatedInterruptInPTY is the regression test of
// review F1: an agent that needs Ctrl-C twice to stop must not take the
// cockpit with it.
func TestCockpitSurvivesARepeatedInterruptInPTY(t *testing.T) {
	bin := buildSetupBinary(t)
	dir := t.TempDir()
	script := "#!/bin/sh\nn=0\ntrap 'n=$((n+1)); echo fake-int-$n; if [ $n -ge 2 ]; then exit 130; fi' INT\necho fake-started\nwhile :; do sleep 1 & wait $!; done\n"
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	c := startCockpit(t, bin, cockpitProject(t), dir)
	runAgentFromCockpit(c, "wait")
	c.waitFor("fake-started")
	c.send("\x03")
	c.send("\x03")
	c.waitFor("Press Enter to return.")
	mark := c.mark()
	c.send("\r")
	c.waitAfter(mark, "The command ")
	time.Sleep(500 * time.Millisecond)
	if c.cmd.ProcessState != nil {
		t.Fatal("the cockpit exited after the command's interrupts")
	}
	c.send("\x03")
	if code := c.wait(); code != 0 {
		t.Fatalf("exit %d, want 0: the interrupts were meant for the command", code)
	}
	c.assertRestored()
}
