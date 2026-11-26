package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
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

// SIGINT and SIGTERM end with watch_stopped signal and exit 0 (US5 1).
func TestWatchSignals(t *testing.T) {
	bin := buildAIDev(t)
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			dir := watchFixture(t)
			p := startWatch(t, bin, dir, watchEnv(t))
			p.next(t, 10*time.Second, "snapshot")
			code, last := p.stop(t, sig)
			if code != 0 || last.Type != "watch_stopped" || !strings.Contains(string(last.Data), `"reason":"signal"`) {
				t.Fatalf("exit %d, last %s %s", code, last.Type, last.Data)
			}
		})
	}
}

// Renaming or deleting the project ends with project_gone and exit 1
// (US5 4, analysis U3).
func TestWatchProjectGone(t *testing.T) {
	bin := buildAIDev(t)
	for name, remove := range map[string]func(string) error{
		"renamed": func(dir string) error { return os.Rename(dir, dir+"-moved") },
		"deleted": os.RemoveAll,
	} {
		t.Run(name, func(t *testing.T) {
			dir := watchFixture(t)
			p := startWatch(t, bin, dir, watchEnv(t))
			p.next(t, 10*time.Second, "snapshot")
			if err := remove(dir); err != nil {
				t.Fatal(err)
			}
			last := p.next(t, 10*time.Second, "watch_stopped")
			if !strings.Contains(string(last.Data), `"reason":"project_gone"`) {
				t.Fatalf("watch_stopped %s, want project_gone", last.Data)
			}
			for range p.lines {
			}
			err := p.cmd.Wait()
			if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
				t.Fatalf("exit %v, want 1", err)
			}
		})
	}
}

// A client that stops reading: the real 1,000-event queue overflows
// without any test-only switch, memory stays bounded, and reading again
// yields resync_required then a snapshot (SC-005, analysis U1).
func TestWatchBlockedClient(t *testing.T) {
	bin := buildAIDev(t)
	dir := t.TempDir()
	const features = 1100
	for i := range features {
		feature := filepath.Join(dir, "specs", fmt.Sprintf("%04d-f", i))
		if err := os.MkdirAll(feature, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(feature, "spec.md"), []byte("# a\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "watch", "--format", "json", "--debounce", "50ms", "--environment-interval", "0")
	cmd.Dir, cmd.Env, cmd.Stdout, cmd.Stderr = dir, watchEnv(t), write, os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	write.Close()
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait(); read.Close() })

	// The snapshot alone fills the pipe; one byte proves watch has started.
	if _, err := read.Read(make([]byte, 1)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	for i := range features {
		if err := os.WriteFile(filepath.Join(dir, "specs", fmt.Sprintf("%04d-f", i), "spec.md"), []byte("# changed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(3 * time.Second)
	out, err := exec.Command("ps", "-o", "rss=", "-p", fmt.Sprint(cmd.Process.Pid)).Output()
	if err != nil {
		t.Fatal(err)
	}
	var rss int
	if _, err := fmt.Sscan(strings.TrimSpace(string(out)), &rss); err != nil || rss > 64*1024 {
		t.Fatalf("RSS %q KB, want under 64 MB", out)
	}
	t.Logf("RSS while blocked: %d KB", rss)

	reader := bufio.NewReaderSize(read, 1<<20)
	if _, err := reader.ReadString('\n'); err != nil { // the rest of the first snapshot
		t.Fatal(err)
	}
	var types []string
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) && !(len(types) >= 2 && types[len(types)-2] == "resync_required" && types[len(types)-1] == "snapshot") {
		text, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read: %v after %v", err, types)
		}
		var line watchLine
		if err := json.Unmarshal([]byte(text), &line); err != nil {
			t.Fatalf("invalid line %q", text)
		}
		types = append(types, line.Type)
		if line.Type == "snapshot" {
			if n := strings.Count(string(line.Data), `"# changed`); n != 0 {
				t.Fatalf("snapshot carries content")
			}
			if n := strings.Count(string(line.Data), `"feature":"`); n != features {
				t.Fatalf("resync snapshot lists %d features, want %d", n, features)
			}
		}
	}
	if len(types) < 2 || types[len(types)-2] != "resync_required" || types[len(types)-1] != "snapshot" {
		t.Fatalf("events after the first snapshot: %d, ending %v; want resync_required then snapshot", len(types), types[max(0, len(types)-3):])
	}
	if len(types) > 1001+1 {
		t.Fatalf("%d events before the resync; the queue holds at most 1000", len(types))
	}
}

// A client that closes its end makes watch exit 2 (FR-018, analysis I3).
func TestWatchClosedOutput(t *testing.T) {
	bin := buildAIDev(t)
	dir := watchFixture(t)
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	read.Close()
	cmd := exec.Command(bin, "watch", "--format", "json")
	cmd.Dir, cmd.Env, cmd.Stdout = dir, watchEnv(t), write
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	write.Close()
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 2 {
			t.Fatalf("exit %v, want 2", err)
		}
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("watch did not exit with its output closed")
	}
}

// writeFiles creates files under dir.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for path, contents := range files {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func implementedFeature(id string) map[string]string {
	return map[string]string{
		"specs/" + id + "/spec.md": "# spec\n", "specs/" + id + "/plan.md": "# plan\n", "specs/" + id + "/tasks.md": "# tasks\n",
		"specs/" + id + "/workflow-state.json": `{"state":"implemented"}`,
	}
}

// 95% of state changes reach the client within 1 s with the default
// debounce (SC-001, analysis A1).
func TestWatchLatency(t *testing.T) {
	bin := buildAIDev(t)
	dir := makeReadyFixture(t)
	for i := range 20 {
		writeFiles(t, dir, implementedFeature(fmt.Sprintf("%03d-gate", i)))
	}
	writeFiles(t, dir, map[string]string{"specs/100-edit/spec.md": "# spec\n", "specs/100-edit/workflow-state.json": `{"state":"specified"}`})
	env := watchEnv(t)
	p := startWatch(t, bin, dir, env)
	p.next(t, 10*time.Second, "snapshot")

	var latencies []time.Duration
	edit := filepath.Join(dir, "specs", "100-edit", "workflow-state.json")
	states := []string{"planned", "specified"}
	for i := range 40 {
		var feature string
		if i%2 == 0 {
			feature = fmt.Sprintf("%03d-gate", i/2)
			if _, stderr, code := runIn(t, bin, dir, env, "workflow", "approve", "--approver", "a@example.com", feature, "approved"); code != 0 {
				t.Fatalf("approve %s: %d %s", feature, code, stderr)
			}
		} else {
			feature = "100-edit"
			if err := os.WriteFile(edit, []byte(`{"state":"`+states[i/2%2]+`"}`), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		changed := time.Now()
		for {
			line := p.next(t, 5*time.Second, "workflow_state_changed")
			if strings.Contains(string(line.Data), `"feature":"`+feature+`"`) {
				latencies = append(latencies, time.Since(changed))
				break
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	slices.Sort(latencies)
	p50, p95 := latencies[len(latencies)/2], latencies[len(latencies)*95/100-1]
	t.Logf("latency p50 %v, p95 %v, max %v", p50, p95, latencies[len(latencies)-1])
	if p95 >= time.Second {
		t.Fatalf("p95 latency %v, want under 1s", p95)
	}
}

type treeEntry struct {
	mode    os.FileMode
	size    int64
	modTime time.Time
	sum     [32]byte
}

func snapshotTree(t *testing.T, dir string) map[string]treeEntry {
	t.Helper()
	tree := map[string]treeEntry{}
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		e := treeEntry{mode: info.Mode(), size: info.Size(), modTime: info.ModTime()}
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			e.sum = sha256.Sum256(data)
		}
		rel, _ := filepath.Rel(dir, path)
		tree[rel] = e
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

// watch writes nothing: outside the test's own subtree the project is
// byte-identical after at least 50 changes (SC-003, F015-FR-019).
func TestWatchWritesNothing(t *testing.T) {
	bin := buildAIDev(t)
	dir := watchFixture(t)
	writeFiles(t, dir, map[string]string{".ai-dev/agents.json": "{"})
	env := watchEnv(t)
	before := snapshotTree(t, dir)
	p := startWatch(t, bin, dir, env)
	p.next(t, 10*time.Second, "snapshot")
	changes := 0
	for i := range 10 {
		id := fmt.Sprintf("zz-%02d", i)
		writeFiles(t, dir, implementedFeature(id))
		changes += 4
		if _, _, code := runIn(t, bin, dir, env, "workflow", "approve", "--approver", "a@example.com", id, "approved"); code != 0 {
			t.Fatalf("approve %s failed", id)
		}
		changes++
		if err := os.WriteFile(filepath.Join(dir, "specs", id, "spec.md"), []byte("# changed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		changes++
		time.Sleep(100 * time.Millisecond)
	}
	if changes < 50 {
		t.Fatalf("only %d changes", changes)
	}
	time.Sleep(time.Second)
	if code, last := p.stop(t, syscall.SIGTERM); code != 0 || last.Type != "watch_stopped" {
		t.Fatalf("stop: %d %s", code, last.Type)
	}
	after := snapshotTree(t, dir)
	for path, entry := range before {
		if path == "specs" {
			continue // its entries change with the test's own zz-* directories
		}
		if got, ok := after[path]; !ok || got != entry {
			t.Errorf("%s changed: %+v -> %+v", path, entry, got)
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok && path != "specs" && !strings.HasPrefix(path, filepath.Join("specs", "zz-")) {
			t.Errorf("%s appeared outside the test's subtree", path)
		}
	}
}

// hostileStrings are names, approvers and reasons that try to reach the
// terminal or a shell: escape and operating-system-command sequences, a
// hyperlink, bidirectional overrides, newlines and shell syntax.
func hostileStrings(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "quote-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct{ Input string }
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	out := []string{"\x1b[31mred\x1b[0m", "\x1b]8;;http://x\x07link\x1b]8;;\x07", "\x1b]52;c;cGF5bG9hZA==\x07", "\u202eevil\u2066x\u2069", "a\nb\rc", "$(touch pwned)`id`"}
	for _, v := range vectors {
		out = append(out, v.Input)
	}
	return out
}

// Hostile strings are JSON-encoded, and shown inert in text (SC-006,
// F015-FR-022).
func TestWatchHostileNames(t *testing.T) {
	bin := buildAIDev(t)
	for _, format := range []string{"json", "text"} {
		t.Run(format, func(t *testing.T) {
			dir := makeReadyFixture(t)
			env := watchEnv(t)
			var ids []string
			for i, s := range hostileStrings(t) {
				id := fmt.Sprintf("%02d-%s", i, s)
				if strings.ContainsAny(id, "/\x00") || strings.TrimSpace(s) == "" {
					continue
				}
				ids = append(ids, id)
				writeFiles(t, dir, implementedFeature(id))
			}
			cmd := exec.Command(bin, "watch", "--format", format, "--environment-interval", "0")
			cmd.Dir, cmd.Env = dir, env
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			collected := make(chan []byte, 1)
			go func() { data, _ := io.ReadAll(stdout); collected <- data }()
			time.Sleep(time.Second)
			strs := hostileStrings(t)
			for i, id := range ids {
				approver, reason := strs[i%len(strs)], strs[(i+1)%len(strs)]
				// workflow approve refuses a blank approver or reason.
				if strings.TrimSpace(approver) == "" {
					approver = "a" + approver
				}
				if strings.TrimSpace(reason) == "" {
					reason = "r" + reason
				}
				if _, stderr, code := runIn(t, bin, dir, env, "workflow", "approve", "--approver", approver, "--reason", reason, "--", id, "approved"); code != 0 {
					t.Fatalf("approve %q approver %q reason %q: %d %s", id, approver, reason, code, stderr)
				}
			}
			time.Sleep(2 * time.Second)
			if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
				t.Fatal(err)
			}
			out := <-collected
			_ = cmd.Wait()
			lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
			gates := strings.Count(string(out), "approval")
			if gates == 0 {
				t.Fatalf("no approval reported; output:\n%q", out)
			}
			if format == "json" {
				for i, line := range lines {
					var l struct {
						SchemaVersion string `json:"schema_version"`
						Seq           int64  `json:"seq"`
					}
					if err := json.Unmarshal([]byte(line), &l); err != nil || l.SchemaVersion != "ai-dev.watch/v1" || l.Seq != int64(i+1) {
						t.Fatalf("line %d %q: %v %+v", i+1, line, err, l)
					}
				}
				return
			}
			for i, b := range out {
				if b < 0x20 && b != '\n' || b == 0x7f {
					t.Fatalf("control byte %#x at %d in text output", b, i)
				}
			}
			if strings.Contains(string(out), "\u202e") || strings.Contains(string(out), "\u2066") {
				t.Fatal("bidirectional override reached the text output")
			}
		})
	}
}
