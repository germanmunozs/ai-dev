package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/germanmunozs/ai-dev/internal/agent"
	"github.com/germanmunozs/ai-dev/internal/cockpit"
	"github.com/germanmunozs/ai-dev/internal/guide"
	"github.com/germanmunozs/ai-dev/internal/report"
	"github.com/germanmunozs/ai-dev/internal/sdd"
	"github.com/germanmunozs/ai-dev/internal/selection"
	"github.com/germanmunozs/ai-dev/internal/system"
	"github.com/germanmunozs/ai-dev/internal/workflow"
)

// TestCockpitRefusals proves F012-FR-001 and contracts/cockpit.md: without
// a terminal, or with any argument, nothing is evaluated and the exit is 2.
func TestCockpitRefusals(t *testing.T) {
	cases := []struct {
		args   []string
		stderr string
	}{
		{[]string{"tui"}, "ai-dev: tui: refused: interactive_terminal_required; use the ai-dev commands directly\n"},
		{[]string{"tui", "x"}, "Usage: ai-dev tui\n"},
		{[]string{"tui", "--format", "json"}, "Usage: ai-dev tui\n"},
	}
	for _, c := range cases {
		executor := &setupExecutor{}
		var stdout, stderr bytes.Buffer
		code := RunWithDependencies(context.Background(), c.args, Dependencies{
			Executor: executor, Root: fstest.MapFS{}, Git: unavailableGitManagementDependencies(),
			Stdin: bytes.NewReader(nil), Stdout: &stdout, Stderr: &stderr,
		})
		if code != 2 || stdout.Len() != 0 || stderr.String() != c.stderr || len(executor.lookups) != 0 || executor.runs != 0 {
			t.Errorf("%q: code=%d stdout=%q stderr=%q lookups=%v", c.args, code, stdout.String(), stderr.String(), executor.lookups)
		}
	}
}

// featureFixture is one specs/<id> directory: an optional record and files.
type featureFixture struct {
	id     string
	record string
	files  []string
}

func writeFeatures(t *testing.T, project string, features []featureFixture) {
	t.Helper()
	for _, f := range features {
		dir := filepath.Join(project, "specs", f.id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if f.record != "" {
			if err := os.WriteFile(filepath.Join(dir, "workflow-state.json"), []byte(f.record), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		for _, name := range f.files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("fixture"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}

var allArtifacts = []string{"spec.md", "plan.md", "tasks.md"}

// cockpitFeatureFixtures covers every Workflow State, missing artifacts, an
// invalid record and a hostile name.
var cockpitFeatureFixtures = []featureFixture{
	{id: "001-draft"},
	{id: "002-specified", record: `{"state":"specified"}`, files: []string{"spec.md"}},
	{id: "003-planned", record: `{"state":"planned"}`, files: []string{"spec.md", "plan.md"}},
	{id: "004-tasked", record: `{"state":"tasked"}`, files: allArtifacts},
	{id: "005-implemented", record: `{"state":"implemented"}`, files: allArtifacts},
	{id: "006-approved", record: `{"state":"approved"}`, files: allArtifacts},
	{id: "007-abandoned", record: `{"state":"abandoned"}`},
	{id: "008-inconsistent", record: `{"state":"planned"}`, files: []string{"spec.md"}},
	{id: "009-invalid", record: `{"state":"Planned"}`},
	{id: "010-\x1b]52;c;aGk=\x07"},
}

// commandText is what "workflow state" and "workflow guide" print.
func commandText(t *testing.T, root fs.FS, executor system.Executor, id string) string {
	t.Helper()
	var out, errOut bytes.Buffer
	RunWorkflowState([]string{id}, root, &out, &errOut)
	RunWorkflowGuide([]string{id}, root, executor, &out, &errOut)
	if errOut.Len() != 0 {
		t.Fatalf("%q: stderr %q", id, errOut.String())
	}
	return out.String()
}

func texts(lines []cockpit.Text) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = line.String()
	}
	return out
}

// TestCockpitFeaturesEqualTheCommands proves F012-FR-005 and SC-002 for
// Features: each row's detail is the commands' text, line by line, and its
// fields are the capabilities' values.
func TestCockpitFeaturesEqualTheCommands(t *testing.T) {
	for _, config := range []struct{ name, agents string }{
		{"built-in assignment", ""},
		{"invalid configuration", `{"schema_version":"nope"}`},
	} {
		t.Run(config.name, func(t *testing.T) {
			project := t.TempDir()
			writeFeatures(t, project, cockpitFeatureFixtures)
			if err := os.Symlink(filepath.Join(project, "specs", "001-draft"), filepath.Join(project, "specs", "011-link")); err != nil {
				t.Fatal(err)
			}
			if config.agents != "" {
				if err := os.MkdirAll(filepath.Join(project, ".ai-dev"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(project, ".ai-dev", "agents.json"), []byte(config.agents), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			root := os.DirFS(project)
			executor := &fakeExecutor{paths: map[string]string{"claude": "/fake/claude"}}
			rows, err := cockpitSource{executor: executor, root: root}.LoadFeatures(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != len(cockpitFeatureFixtures) {
				t.Fatalf("%d rows, want %d (the symbolic link excluded)", len(rows), len(cockpitFeatureFixtures))
			}
			for i, row := range rows {
				want := cockpitFeatureFixtures[i].id
				if row.ID != want {
					t.Fatalf("row %d is %q, want %q (name order)", i, row.ID, want)
				}
				if got, want := texts(row.Detail), texts(cockpit.SanitizeLines(commandText(t, root, executor, row.ID))); !reflect.DeepEqual(got, want) {
					t.Errorf("%q detail:\n%q\nwant\n%q", row.ID, got, want)
				}
				state := workflow.EvaluateState(root, row.ID)
				guidance := guide.Evaluate(root, row.ID, selection.NewSelectorFor(agent.NewRegistry(executor), mustAssignment(root)))
				if row.Recognized != (state.Outcome == workflow.StateReported) || row.Terminal != state.Terminal {
					t.Errorf("%q: recognized=%t terminal=%t, state %+v", row.ID, row.Recognized, row.Terminal, state)
				}
				if row.StepRecommended != (guidance.Outcome == sdd.StepRecommended) || row.AgentID != string(guidance.Agent) ||
					row.Availability.String() != string(guidance.Availability) || row.Source.String() != string(guidance.Source) {
					t.Errorf("%q: row %+v, guidance %+v", row.ID, row, guidance)
				}
				if row.AgentAvailable != (guidance.Availability == guide.Available) {
					t.Errorf("%q: agent available %t, guidance %s", row.ID, row.AgentAvailable, guidance.Availability)
				}
			}
			hostile := rows[len(rows)-1]
			if hostile.Name.String() != `010-\x1b]52;c;aGk=\x07` || strings.Contains(strings.Join(texts(hostile.Detail), "\n"), "\x1b") {
				t.Errorf("hostile name not sanitized: %q %q", hostile.Name.String(), texts(hostile.Detail))
			}
			if config.agents != "" && rows[0].AgentAvailable {
				t.Errorf("an invalid configuration left the agent available: %+v", rows[0])
			}
		})
	}
}

func mustAssignment(root fs.FS) selection.Assignment {
	assignment, _, _, _ := loadAssignment(root)
	return assignment
}

// TestCockpitFeaturesLoadQuickly proves the second half of SC-001: 50
// features load in under 2 seconds.
func TestCockpitFeaturesLoadQuickly(t *testing.T) {
	project := t.TempDir()
	var features []featureFixture
	for i := range 50 {
		features = append(features, featureFixture{id: fmt.Sprintf("%03d-feature", i+1), record: `{"state":"tasked"}`, files: allArtifacts})
	}
	writeFeatures(t, project, features)
	executor := &fakeExecutor{paths: map[string]string{"claude": "/fake/claude", "codex": "/fake/codex"}}
	start := time.Now()
	rows, err := cockpitSource{executor: executor, root: os.DirFS(project)}.LoadFeatures(context.Background())
	elapsed := time.Since(start)
	if err != nil || len(rows) != 50 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
	t.Logf("50 features loaded in %v", elapsed)
	if elapsed >= 2*time.Second {
		t.Fatalf("50 features took %v, want under 2s", elapsed)
	}
}

// TestCockpitFeaturesFailInsteadOfGoingStale: a removed or unreadable
// project is a failure, never an empty or stale list (edge case).
func TestCockpitFeaturesFailInsteadOfGoingStale(t *testing.T) {
	removed := t.TempDir()
	writeFeatures(t, removed, []featureFixture{{id: "001-a"}})
	root := os.DirFS(removed)
	if err := os.RemoveAll(removed); err != nil {
		t.Fatal(err)
	}
	if rows, err := (cockpitSource{executor: &fakeExecutor{}, root: root}).LoadFeatures(context.Background()); err == nil {
		t.Fatalf("removed project: rows %v and no error", rows)
	}
	if os.Geteuid() == 0 {
		t.Skip("root reads unreadable directories")
	}
	unreadable := t.TempDir()
	writeFeatures(t, unreadable, []featureFixture{{id: "001-a"}})
	specs := filepath.Join(unreadable, "specs")
	if err := os.Chmod(specs, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(specs, 0o755) })
	if rows, err := (cockpitSource{executor: &fakeExecutor{}, root: os.DirFS(unreadable)}).LoadFeatures(context.Background()); err == nil {
		t.Fatalf("unreadable specs: rows %v and no error", rows)
	}
	empty := t.TempDir()
	if rows, err := (cockpitSource{executor: &fakeExecutor{}, root: os.DirFS(empty)}).LoadFeatures(context.Background()); err != nil || len(rows) != 0 {
		t.Fatalf("no specs/: rows %v err %v", rows, err)
	}
}

// cockpitLauncher records the launch and returns a fixed status.
type cockpitLauncher struct {
	spec      system.Spec
	ctxErr    error
	launches  int
	status    system.ExitStatus
	launchErr error
}

func (l *cockpitLauncher) Launch(ctx context.Context, spec system.Spec) (system.Handle, error) {
	l.launches++
	l.spec, l.ctxErr = spec, ctx.Err()
	if l.launchErr != nil {
		return nil, l.launchErr
	}
	return cockpitHandle{l.status}, nil
}

type cockpitHandle struct{ status system.ExitStatus }

func (h cockpitHandle) Wait() (system.ExitStatus, error) { return h.status, nil }

func cockpitRunnerFor(launcher system.Launcher, stdin *strings.Reader, stdout *bytes.Buffer) cockpitRunner {
	parent, cancel := context.WithCancel(context.Background())
	config := cockpitConfig(parent, Dependencies{Launcher: launcher, Stdin: stdin, Stdout: stdout, Stderr: stdout}, "/bin/ai-dev", &signalGate{})
	cancel() // as main does on an interrupt meant for the command
	return config.Runner.(cockpitRunner)
}

// TestCockpitRunnerLaunchesTheCommand proves R-03 and F012-FR-010: this
// binary with exactly the argument vector, on the terminal's streams, the
// exit code shown, one line read, and a context the cockpit never cancels.
func TestCockpitRunnerLaunchesTheCommand(t *testing.T) {
	args := []string{"agent", "run", "--", "claude-code", "-x \"a b\""}
	cases := []struct {
		name    string
		status  system.ExitStatus
		message string
		result  error
	}{
		{"success", system.ExitStatus{Exited: true}, "ai-dev tui: the command exited with code 0. Press Enter to return.", nil},
		{"refusal", system.ExitStatus{Exited: true, ExitCode: 3}, "ai-dev tui: the command exited with code 3. Press Enter to return.", cockpit.CommandExit{Code: 3}},
		{"signal", system.ExitStatus{ExitCode: -1}, "ai-dev tui: the command was terminated by a signal. Press Enter to return.", cockpit.CommandExit{Signaled: true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			launcher := &cockpitLauncher{status: c.status}
			stdin, stdout := strings.NewReader("\nleft for the cockpit"), &bytes.Buffer{}
			runner := cockpitRunnerFor(launcher, stdin, stdout)
			err := runner.Command(args).Run()
			if err != c.result {
				t.Fatalf("result %v, want %v", err, c.result)
			}
			if launcher.spec.Name != "/bin/ai-dev" || !reflect.DeepEqual(launcher.spec.Args, args) ||
				launcher.spec.Stdin != stdin || launcher.spec.Stdout != stdout || launcher.spec.Stderr != stdout {
				t.Fatalf("spec %+v", launcher.spec)
			}
			if launcher.ctxErr != nil {
				t.Fatalf("the launch context was cancelled with main's: %v", launcher.ctxErr)
			}
			if !strings.Contains(stdout.String(), c.message) {
				t.Fatalf("stdout %q", stdout.String())
			}
			if rest := stdin.Len(); rest != len("left for the cockpit") {
				t.Fatalf("read past the line: %d bytes left", rest)
			}
		})
	}
}

// TestCockpitRunnerLaunchFailure: a launch failure is reported and nothing
// else runs or waits.
func TestCockpitRunnerLaunchFailure(t *testing.T) {
	launcher := &cockpitLauncher{launchErr: errors.New("no such file")}
	stdin, stdout := strings.NewReader("\n"), &bytes.Buffer{}
	err := cockpitRunnerFor(launcher, stdin, stdout).Command([]string{"setup"}).Run()
	if err == nil || !strings.Contains(err.Error(), "no such file") || launcher.launches != 1 || stdout.Len() != 0 || stdin.Len() != 1 {
		t.Fatalf("err=%v launches=%d stdout=%q", err, launcher.launches, stdout.String())
	}
}

// TestCockpitConfigIsDetached: the loaders' context outlives main's.
func TestCockpitConfigIsDetached(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	config := cockpitConfig(parent, Dependencies{}, "/bin/ai-dev", &signalGate{})
	cancel()
	if config.Context.Err() != nil {
		t.Fatal("the cockpit context was cancelled with main's")
	}
}

// TestCockpitPreview: plain words bare, every other argument in the 004
// shell-inert form, so the review is one inert line.
func TestCockpitPreview(t *testing.T) {
	runner := cockpitRunner{}
	cases := map[string][]string{
		`ai-dev agent run -- claude-code "-x \"a b\""`:                                      {"agent", "run", "--", "claude-code", `-x "a b"`},
		`ai-dev workflow approve "--approver=Germán" -- 012-x planned`:                      {"workflow", "approve", "--approver=Germán", "--", "012-x", "planned"},
		`ai-dev git commit validate "--message=it's \$(x)\nbody \` + "`" + `y\` + "`" + `"`: {"git", "commit", "validate", "--message=it's $(x)\nbody `y`"},
		`ai-dev workflow abandon --approver=a --reason= -- "" abandoned`:                    {"workflow", "abandon", "--approver=a", "--reason=", "--", "", "abandoned"},
	}
	for want, args := range cases {
		if got := runner.Preview(args); got != want {
			t.Errorf("Preview(%q) = %s, want %s", args, got, want)
		}
	}
}

// TestCockpitAreasEqualTheCommands proves SC-002 for Environment, Agents
// and Repository, and F012-FR-018: no loaded line holds an environment
// value.
func TestCockpitAreasEqualTheCommands(t *testing.T) {
	secret := fmt.Sprintf("sentinel-%d", time.Now().UnixNano())
	t.Setenv("AI_DEV_TEST_SECRET", secret)
	root := fstest.MapFS{
		".ai-dev/agents.json": &fstest.MapFile{Data: []byte(`{"schema_version":"ai-dev.agents/v1","agents":["bogus"]}`)},
	}
	executor := &fakeExecutor{paths: map[string]string{"claude": "/fake/claude", "git": "/usr/bin/git"}, stdout: "true"}
	source := cockpitSource{executor: executor, root: root, git: unavailableGitManagementDependencies()}
	ctx := context.Background()

	var doctorOut, setupOut bytes.Buffer
	if err := report.WriteText(&doctorOut, RunDoctor(ctx, executor, root)); err != nil {
		t.Fatal(err)
	}
	RunSetupShow(nil, executor, root, &setupOut, &bytes.Buffer{})
	environment, err := source.LoadEnvironment(ctx)
	if err != nil || !reflect.DeepEqual(texts(environment), texts(cockpit.SanitizeLines(doctorOut.String()))) {
		t.Fatalf("environment %q (err %v), want %q", texts(environment), err, doctorOut.String())
	}
	agents, err := source.LoadAgents(ctx)
	if err != nil || !reflect.DeepEqual(texts(agents), texts(cockpit.SanitizeLines(setupOut.String()))) {
		t.Fatalf("agents %q (err %v), want %q", texts(agents), err, setupOut.String())
	}
	if !strings.Contains(setupOut.String(), "configuration_invalid") {
		t.Fatalf("the fixture is not an invalid configuration: %q", setupOut.String())
	}
	if _, err := source.LoadRepository(ctx); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("a failing inspection is not a failed area: %v", err)
	}

	if _, err := exec.LookPath("git"); err == nil {
		real := cockpitSource{executor: executor, root: root, git: DefaultGitManagementDependencies()}
		repository, err := real.LoadRepository(ctx)
		var inspectOut bytes.Buffer
		code := RunGitInit(ctx, []string{"inspect"}, DefaultGitManagementDependencies(), bytes.NewReader(nil), false, &inspectOut, &bytes.Buffer{})
		if err != nil || code != 0 || !reflect.DeepEqual(texts(repository), texts(cockpit.SanitizeLines(inspectOut.String()))) {
			t.Fatalf("repository %q (err %v), want %q (exit %d)", texts(repository), err, inspectOut.String(), code)
		}
		environment = append(environment, repository...)
	}
	for _, line := range append(environment, agents...) {
		if strings.Contains(line.String(), secret) {
			t.Fatalf("an environment value reached a view: %q", line.String())
		}
	}
}

// TestSignalGate proves R-04 at the moment of reception (review F1): while
// a command runs, interrupts are dropped and a termination is handed to
// the model only when the command ends; otherwise both are forwarded.
func TestSignalGate(t *testing.T) {
	sent := make(chan any, 8)
	gate := &signalGate{send: func(msg tea.Msg) { sent <- msg }}
	gate.received(os.Interrupt)
	if msg := <-sent; msg != (cockpit.SignalMsg{}) {
		t.Fatalf("idle interrupt: %v", msg)
	}
	gate.received(syscall.SIGTERM)
	if msg := <-sent; msg != (cockpit.SignalMsg{Terminate: true}) {
		t.Fatalf("idle termination: %v", msg)
	}
	gate.begin()
	gate.received(os.Interrupt)
	gate.received(os.Interrupt)
	gate.received(syscall.SIGTERM)
	select {
	case msg := <-sent:
		t.Fatalf("a signal reached the model while a command ran: %v", msg)
	case <-time.After(50 * time.Millisecond):
	}
	if !gate.terminating() {
		t.Fatal("termination not remembered")
	}
	gate.end()
	if msg := <-sent; msg != (cockpit.SignalMsg{Terminate: true}) {
		t.Fatalf("deferred termination: %v", msg)
	}
	gate.begin()
	gate.received(os.Interrupt)
	gate.end()
	select {
	case msg := <-sent:
		t.Fatalf("an interrupt meant for the command reached the model: %v", msg)
	case <-time.After(50 * time.Millisecond):
	}
}

// signalingLauncher receives a signal while the command runs.
type signalingLauncher struct {
	gate *signalGate
	sig  os.Signal
}

func (l *signalingLauncher) Launch(context.Context, system.Spec) (system.Handle, error) {
	return signalingHandle{l}, nil
}

type signalingHandle struct{ l *signalingLauncher }

func (h signalingHandle) Wait() (system.ExitStatus, error) {
	h.l.gate.received(h.l.sig)
	return system.ExitStatus{ExitCode: -1}, nil
}

// TestCockpitRunnerHonorsTerminationWithoutEnter proves review F4: after a
// termination, the runner does not wait for a person to press Enter.
func TestCockpitRunnerHonorsTerminationWithoutEnter(t *testing.T) {
	launcher := &signalingLauncher{sig: syscall.SIGTERM}
	stdin, stdout := strings.NewReader("\n"), &bytes.Buffer{}
	runner := cockpitRunnerFor(launcher, stdin, stdout)
	launcher.gate = runner.gate
	runner.gate.send = func(tea.Msg) {}
	if err := runner.Command([]string{"setup"}).Run(); err != (cockpit.CommandExit{Signaled: true}) {
		t.Fatalf("result %v", err)
	}
	if stdin.Len() != 1 {
		t.Fatal("waited for Enter after a termination")
	}
	interrupted := &signalingLauncher{sig: os.Interrupt}
	stdin = strings.NewReader("\n")
	runner = cockpitRunnerFor(interrupted, stdin, stdout)
	interrupted.gate = runner.gate
	_ = runner.Command([]string{"setup"}).Run()
	if stdin.Len() != 0 {
		t.Fatal("an interrupt skipped the Enter wait")
	}
}

// The cockpit's Agents area shows every catalog agent (F013-FR-012).
func TestCockpitAgentsAreaListsTheCatalog(t *testing.T) {
	executor := &fakeExecutor{paths: map[string]string{"vibe": "/fake/vibe"}}
	source := cockpitSource{executor: executor, root: fstest.MapFS{}, git: unavailableGitManagementDependencies()}
	lines, err := source.LoadAgents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(texts(lines), "\n")
	last := -1
	for _, id := range catalogIDs {
		at := strings.Index(text, "agent "+id+" ")
		if at <= last {
			t.Fatalf("agent %s missing or out of order in:\n%s", id, text)
		}
		last = at
	}
	if !strings.Contains(text, "agent mistral-vibe availability=available") {
		t.Fatalf("availability not shown:\n%s", text)
	}
}
