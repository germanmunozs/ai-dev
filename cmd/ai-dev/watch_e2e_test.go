package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The end-to-end tests of ai-dev watch run the built binary as a subprocess
// with fake agents and tools on a private PATH; no real coding agent runs.

func buildAIDev(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake executables are POSIX shell scripts")
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "ai-dev")
	build := exec.Command("go", "build", "-o", bin, "./cmd/ai-dev")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	return bin
}

// watchFixture is a ready project with three features: specified,
// implemented with every artifact, and an invalid record.
func watchFixture(t *testing.T) string {
	t.Helper()
	root := makeReadyFixture(t)
	files := map[string]string{
		"specs/001-specified/spec.md":               "# spec\n",
		"specs/001-specified/workflow-state.json":   `{"state":"specified"}`,
		"specs/002-implemented/spec.md":             "# spec\n",
		"specs/002-implemented/plan.md":             "# plan\n",
		"specs/002-implemented/tasks.md":            "# tasks\n",
		"specs/002-implemented/workflow-state.json": `{"state":"implemented"}`,
		"specs/003-invalid/spec.md":                 "# spec\n",
		"specs/003-invalid/workflow-state.json":     `{"state":"Implemented"}`,
	}
	for path, contents := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// watchEnv is the environment for every subprocess: fake tools first on
// PATH, and the fake git's marker outside the project.
func watchEnv(t *testing.T) []string {
	t.Helper()
	marker := filepath.Join(t.TempDir(), "marker")
	return append(os.Environ(), "PATH="+makeFakePATH(t, marker)+":/usr/bin:/bin", "AI_DEV_TEST_MARKER="+marker)
}

func runIn(t *testing.T, bin, dir string, env []string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir, cmd.Env = dir, env
	return runCommand(cmd)
}

type watchLine struct {
	Seq  int64           `json:"seq"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func TestWatchOnce(t *testing.T) {
	bin := buildAIDev(t)
	dir := watchFixture(t)
	env := watchEnv(t)

	stdout, stderr, code := runIn(t, bin, dir, env, "watch", "--format", "json", "--once")
	if code != 0 || strings.Count(stdout, "\n") != 1 {
		t.Fatalf("watch --once: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	var line watchLine
	if err := json.Unmarshal([]byte(stdout), &line); err != nil || line.Type != "snapshot" || line.Seq != 1 {
		t.Fatalf("line %q: %v", stdout, err)
	}
	var data struct {
		Features []struct {
			Feature string          `json:"feature"`
			State   json.RawMessage `json:"state"`
			Next    json.RawMessage `json:"next"`
		} `json:"features"`
		Setup       json.RawMessage `json:"setup"`
		Environment json.RawMessage `json:"environment"`
	}
	if err := json.Unmarshal(line.Data, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Features) != 3 {
		t.Fatalf("features %d, want 3", len(data.Features))
	}
	same := func(name string, got json.RawMessage, args ...string) {
		t.Helper()
		want, _, _ := runIn(t, bin, dir, env, args...)
		if string(got) != strings.TrimSuffix(want, "\n") {
			t.Errorf("%s:\n%s\nwant\n%s", name, got, want)
		}
	}
	for _, f := range data.Features {
		same(f.Feature+" state", f.State, "workflow", "state", "--format", "json", f.Feature)
		same(f.Feature+" next", f.Next, "workflow", "next", "--format", "json", f.Feature)
	}
	same("setup", data.Setup, "setup", "show", "--format", "json")
	same("environment", data.Environment, "doctor", "--format", "json")

	stdout, _, code = runIn(t, bin, dir, env, "watch", "--format", "text", "--once")
	if code != 0 || !strings.HasPrefix(stdout, "ai-dev: event=snapshot seq=1 ") || strings.Count(stdout, "\n") != 1 {
		t.Fatalf("text --once: exit %d, %q", code, stdout)
	}
}

// watchProcess is a running ai-dev watch whose lines the test reads.
type watchProcess struct {
	cmd   *exec.Cmd
	lines chan watchLine
	raw   chan string
}

func startWatch(t *testing.T, bin, dir string, env []string, args ...string) *watchProcess {
	t.Helper()
	cmd := exec.Command(bin, append([]string{"watch", "--format", "json"}, args...)...)
	cmd.Dir, cmd.Env = dir, env
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := &watchProcess{cmd: cmd, lines: make(chan watchLine, 4096), raw: make(chan string, 4096)}
	go func() {
		reader := bufio.NewReaderSize(stdout, 1<<20)
		for {
			text, err := reader.ReadString('\n')
			if text != "" {
				var line watchLine
				if json.Unmarshal([]byte(text), &line) != nil {
					line.Type = "invalid:" + text
				}
				p.raw <- text
				p.lines <- line
			}
			if err != nil {
				close(p.lines)
				return
			}
		}
	}()
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_, _ = io.Copy(io.Discard, stdout)
			_ = cmd.Wait()
		}
	})
	return p
}

// next returns the next line of one of the given types, skipping
// heartbeats, within timeout.
func (p *watchProcess) next(t *testing.T, timeout time.Duration, types ...string) watchLine {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case line, ok := <-p.lines:
			if !ok {
				t.Fatalf("watch ended while waiting for %v", types)
			}
			for _, want := range types {
				if line.Type == want {
					return line
				}
			}
			if line.Type != "heartbeat" {
				t.Logf("skipping %s %s", line.Type, line.Data)
			}
		case <-deadline:
			t.Fatalf("no %v within %v", types, timeout)
		}
	}
}

// quiet asserts that no line of the given types arrives within d.
func (p *watchProcess) quiet(t *testing.T, d time.Duration, types ...string) {
	t.Helper()
	deadline := time.After(d)
	for {
		select {
		case line, ok := <-p.lines:
			if !ok {
				return
			}
			for _, unwanted := range types {
				if line.Type == unwanted {
					t.Fatalf("unexpected %s %s", line.Type, line.Data)
				}
			}
		case <-deadline:
			return
		}
	}
}

// stop sends sig and returns the exit code and the last line.
func (p *watchProcess) stop(t *testing.T, sig syscall.Signal) (int, watchLine) {
	t.Helper()
	if err := p.cmd.Process.Signal(sig); err != nil {
		t.Fatal(err)
	}
	var last watchLine
	for line := range p.lines {
		last = line
	}
	err := p.cmd.Wait()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	}
	return code, last
}

// Human Gates applied and refused, and a manual edit, through the built
// binary (SC-002, US1 scenarios 1-4).
func TestWatchHumanGates(t *testing.T) {
	bin := buildAIDev(t)
	dir := watchFixture(t)
	env := watchEnv(t)
	p := startWatch(t, bin, dir, env)
	p.next(t, 10*time.Second, "snapshot")

	type change struct {
		Feature   string  `json:"feature"`
		From      *string `json:"from"`
		To        *string `json:"to"`
		Outcome   string  `json:"outcome"`
		HumanGate *struct {
			Decision string `json:"decision"`
			Approver string `json:"approver"`
			ToState  string `json:"to_state"`
		} `json:"human_gate"`
	}
	decode := func(line watchLine) change {
		var c change
		if err := json.Unmarshal(line.Data, &c); err != nil {
			t.Fatal(err)
		}
		return c
	}

	if _, stderr, code := runIn(t, bin, dir, env, "workflow", "approve", "--approver", "ana@example.com", "002-implemented", "approved"); code != 0 {
		t.Fatalf("approve: exit %d %s", code, stderr)
	}
	c := decode(p.next(t, 5*time.Second, "workflow_state_changed"))
	if c.Feature != "002-implemented" || c.From == nil || *c.From != "implemented" || c.To == nil || *c.To != "approved" ||
		c.Outcome != "state_reported" || c.HumanGate == nil || c.HumanGate.Decision != "approval" || c.HumanGate.Approver != "ana@example.com" {
		t.Fatalf("approval change %+v", c)
	}

	if _, _, code := runIn(t, bin, dir, env, "workflow", "approve", "--approver", "ana@example.com", "002-implemented", "approved"); code == 0 {
		t.Fatal("a repeated approval of a terminal feature succeeded")
	}
	if _, _, code := runIn(t, bin, dir, env, "workflow", "approve", "--approver", "ana@example.com", "001-specified", "planned"); code == 0 {
		t.Fatal("an approval with a missing plan succeeded")
	}
	p.quiet(t, time.Second, "workflow_state_changed")

	record := filepath.Join(dir, "specs", "001-specified", "workflow-state.json")
	if err := os.WriteFile(record, []byte(`{"state":"planned"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	c = decode(p.next(t, 5*time.Second, "workflow_state_changed"))
	if c.Feature != "001-specified" || c.To == nil || *c.To != "planned" || c.HumanGate != nil {
		t.Fatalf("manual edit change %+v", c)
	}

	code, last := p.stop(t, syscall.SIGTERM)
	if code != 0 || last.Type != "watch_stopped" || !strings.Contains(string(last.Data), `"reason":"signal"`) {
		t.Fatalf("stop: exit %d, last %s %s", code, last.Type, last.Data)
	}
}
