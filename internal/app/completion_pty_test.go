//go:build (darwin && !ios) || (linux && !android)

package app

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// openPTY opens a pseudo-terminal pair, as the Feature 008 PTY tests do.
func openPTY(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	ioctl := func(request uintptr, argument unsafe.Pointer) {
		if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), request, uintptr(argument)); errno != 0 {
			master.Close()
			t.Skipf("pseudo-terminal ioctl %#x: %v", request, errno)
		}
	}
	var slavePath string
	switch runtime.GOOS {
	case "darwin":
		ioctl(0x20007454, nil) // TIOCPTYGRANT
		ioctl(0x20007452, nil) // TIOCPTYUNLK
		var name [128]byte
		ioctl(0x40807453, unsafe.Pointer(&name[0])) // TIOCPTYGNAME
		slavePath = string(bytes.TrimRight(name[:], "\x00"))
	case "linux":
		var unlock int32
		ioctl(0x40045431, unsafe.Pointer(&unlock)) // TIOCSPTLCK
		var number uint32
		ioctl(0x80045430, unsafe.Pointer(&number)) // TIOCGPTN
		slavePath = fmt.Sprintf("/dev/pts/%d", number)
	}
	slave, err := os.OpenFile(slavePath, os.O_RDWR, 0)
	if err != nil {
		master.Close()
		t.Skipf("open %s: %v", slavePath, err)
	}
	// A wide terminal keeps each command line on one screen line, so zsh
	// never redraws it in pieces that a substring match would miss.
	size := struct{ rows, cols, x, y uint16 }{50, 200, 0, 0}
	setSize := map[string]uintptr{"darwin": 0x80087467, "linux": 0x5414}[runtime.GOOS] // TIOCSWINSZ
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, slave.Fd(), setSize, uintptr(unsafe.Pointer(&size))); errno != 0 {
		t.Fatalf("set terminal size: %v", errno)
	}
	return master, slave
}

// zshSession is an interactive zsh in a pseudo-terminal.
type zshSession struct {
	t      *testing.T
	master *os.File
	mu     sync.Mutex
	output bytes.Buffer
}

func startZsh(t *testing.T, dir string) *zshSession {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("no zsh available")
	}
	return startShell(t, dir, zsh, "-f", "-i")
}

// startShell runs an interactive shell in a pseudo-terminal.
func startShell(t *testing.T, dir, shell string, args ...string) *zshSession {
	master, slave := openPTY(t)
	cmd := exec.Command(shell, args...)
	cmd.Dir = dir
	cmd.Env = []string{"HOME=" + t.TempDir(), "TERM=xterm", "PATH=" + os.Getenv("PATH"), "PS1=> "}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	slave.Close()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		master.Close()
	})
	s := &zshSession{t: t, master: master}
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
	return s
}

func (s *zshSession) send(text string) {
	if _, err := s.master.WriteString(text); err != nil {
		s.t.Fatal(err)
	}
}

func (s *zshSession) waitFor(want string) string {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		text := s.output.String()
		s.mu.Unlock()
		if strings.Contains(text, want) {
			return text
		}
		time.Sleep(50 * time.Millisecond)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.t.Fatalf("the shell never printed %q; output:\n%q", want, s.output.String())
	return ""
}

// complete types line (ending in a Tab), waits for want and fails if a
// completion function reported an error.
func (s *zshSession) complete(line, want string) {
	s.mu.Lock()
	s.output.Reset()
	s.mu.Unlock()
	s.send(line)
	text := s.waitFor(want)
	s.send("\x15") // kill the line
	for _, bad := range []string{"_files:", "_description:", "bad output format", "not an identifier"} {
		if strings.Contains(text, bad) {
			s.t.Fatalf("%q: zsh reported an error:\n%q", line, text)
		}
	}
}

// TestZshCompletionInRealShell completes in an interactive zsh with the
// real compinit and _files, which the stub-based test cannot cover: file
// names must be offered, and no completion function may report an error.
func TestZshCompletionInRealShell(t *testing.T) {
	script, dir := writeScript(t, "zsh"), fileFixture(t)
	s := startZsh(t, dir)
	s.send("PS1='> '; autoload -U compinit; compinit -u -D; eval \"$(<" + filepath.ToSlash(script) + ")\"; print READY\n")
	s.waitFor("READY")
	s.complete("ai-dev git commit validate --file no\t", "notes.txt")
	s.complete("ai-dev agent run --format=json c\t", "codex-cli")
	s.complete("ai-dev workflow st\t", "workflow state")
}

// Installed in fpath under the conventional name _ai-dev, the script
// completes on the very first Tab.
func TestZshCompletionAutoloadedFromFpath(t *testing.T) {
	script, dir := writeScript(t, "zsh"), fileFixture(t)
	functions := t.TempDir()
	data, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(functions, "_ai-dev"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	s := startZsh(t, dir)
	s.send("PS1='> '; fpath=(" + filepath.ToSlash(functions) + " $fpath); autoload -U compinit; compinit -u -D; print READY\n")
	s.waitFor("READY")
	s.complete("ai-dev workflow st\t", "workflow state")
}

// TestBashCompletionInRealShell checks what readline really inserts, which
// the COMP_WORDS simulation cannot show: bash replaces only the text after
// the last "=", in bash 3.2 (one word) and in bash 4+ (split words)
// (Feature 011 review M1, L1).
func TestBashCompletionInRealShell(t *testing.T) {
	script, dir := writeScript(t, "bash"), fileFixture(t)
	for _, bash := range bashBinaries(t) {
		t.Run(bash, func(t *testing.T) {
			s := startShell(t, dir, bash, "--norc", "--noprofile", "-i")
			s.send("source '" + filepath.ToSlash(script) + "'; echo READY\n")
			s.waitFor("READY")
			for typed, want := range map[string]string{
				"ai-dev setup apply --role implementation=cl": "ai-dev setup apply --role implementation=claude-code",
				"ai-dev setup apply --role=implementation=co": "ai-dev setup apply --role=implementation=codex-cli",
				"ai-dev agent run --format=j":                 "ai-dev agent run --format=json",
				"ai-dev setup apply --agent claude-code --ag": "ai-dev setup apply --agent claude-code --agent",
			} {
				s.mu.Lock()
				s.output.Reset()
				s.mu.Unlock()
				// Tab completes; Ctrl-A, "echo LINE:", Ctrl-E and Enter
				// print the line exactly as readline left it.
				s.send(typed + "\t")
				time.Sleep(300 * time.Millisecond)
				s.send("\x01echo LINE:\x05\n")
				s.waitFor("LINE:" + want + "\r\n")
			}
		})
	}
}
