package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

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

const (
	cockpitUsage   = "Usage: ai-dev tui"
	cockpitRefusal = "ai-dev: tui: refused: interactive_terminal_required; use the ai-dev commands directly"
)

// RunCockpit implements "ai-dev tui" (Feature 012). The cockpit presents;
// this function composes its data from the commands' own functions and its
// actions from the ai-dev binary itself, launched through system.Launcher.
func RunCockpit(ctx context.Context, args []string, deps Dependencies) int {
	if len(args) != 0 {
		fmt.Fprintln(deps.Stderr, cockpitUsage)
		return 2
	}
	if !isTerminal(deps.Stdin) || !writerIsTerminal(deps.Stdout) {
		fmt.Fprintln(deps.Stderr, cockpitRefusal)
		return 2
	}
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintf(deps.Stderr, "ai-dev: tui: %v\n", err)
		return 2
	}
	gate := &signalGate{}
	return runCockpitProgram(cockpit.New(cockpitConfig(ctx, deps, executable, gate)), gate, deps.Stdin, deps.Stdout, deps.Stderr)
}

// cockpitConfig composes the cockpit. main cancels ctx on every SIGINT,
// including one the terminal sends to a command in the foreground, so the
// loaders and the launches run on a context that outlives it (R-04).
func cockpitConfig(ctx context.Context, deps Dependencies, executable string, gate *signalGate) cockpit.Config {
	ctx = context.WithoutCancel(ctx)
	return cockpit.Config{
		Context: ctx,
		Source:  cockpitSource{executor: deps.Executor, root: deps.Root, git: deps.Git},
		Runner: cockpitRunner{ctx: ctx, launcher: deps.Launcher, executable: executable, gate: gate,
			stdin: deps.Stdin, stdout: deps.Stdout, stderr: deps.Stderr},
	}
}

// signalGate decides, when a signal is received, whether a command owns
// the terminal (R-04). The model cannot decide it: Bubble Tea runs a command
// on its event loop, so a signal sent while the command runs is delivered
// only after it returns, when the model no longer knows it was meant for
// the command (review F1).
type signalGate struct {
	mu          sync.Mutex
	running     bool
	termPending bool
	send        func(tea.Msg)
}

// received drops an interrupt and defers a termination while a command
// runs, and forwards either to the model otherwise.
func (g *signalGate) received(sig os.Signal) {
	terminate := sig == syscall.SIGTERM
	g.mu.Lock()
	if g.running {
		g.termPending = g.termPending || terminate
		g.mu.Unlock()
		return
	}
	send := g.send
	g.mu.Unlock()
	if send != nil {
		send(cockpit.SignalMsg{Terminate: terminate})
	}
}

func (g *signalGate) begin() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.running = true
}

func (g *signalGate) terminating() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.termPending
}

// end hands a deferred termination to the model. It is called on Bubble
// Tea's event loop, so the message is sent from another goroutine.
func (g *signalGate) end() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.running = false
	if g.termPending && g.send != nil {
		g.termPending = false
		go g.send(cockpit.SignalMsg{Terminate: true})
	}
}

// runCockpitProgram runs the program with Bubble Tea's signal handler
// disabled and forwards SIGINT and SIGTERM through the gate (R-04). Bubble
// Tea restores the terminal on every exit, panics included.
func runCockpitProgram(model cockpit.Model, gate *signalGate, stdin io.Reader, stdout, stderr io.Writer) int {
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	program := tea.NewProgram(model, tea.WithInput(stdin), tea.WithOutput(stdout), tea.WithoutSignalHandler())
	gate.mu.Lock()
	gate.send = program.Send
	gate.mu.Unlock()
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case received := <-signals:
				gate.received(received)
			case <-done:
				return
			}
		}
	}()
	final, err := program.Run()
	if err != nil {
		fmt.Fprintf(stderr, "ai-dev: tui: %v\n", err)
		return 2
	}
	return final.(cockpit.Model).ExitCode()
}

// cockpitSource evaluates the views with the functions the commands use, so
// each view is the command's own report (R-02, SC-002).
type cockpitSource struct {
	executor system.Executor
	root     fs.FS
	git      GitManagementDependencies
}

// LoadFeatures lists specs/* as Features 004 and 005 recognize them, in
// name order, each with the state and guide reports.
func (s cockpitSource) LoadFeatures(context.Context) ([]cockpit.FeatureRow, error) {
	if _, err := fs.Stat(s.root, "."); err != nil {
		return nil, fmt.Errorf("the project directory cannot be read: %v", err)
	}
	entries, err := fs.ReadDir(s.root, "specs")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("specs cannot be read: %v", err)
	}
	assignment, _, _, _ := loadAssignment(s.root)
	selector := selection.NewSelectorFor(agent.NewRegistry(s.executor), assignment)
	var rows []cockpit.FeatureRow
	for _, entry := range entries {
		if workflow.FeatureExists(s.root, entry.Name()) {
			rows = append(rows, featureRow(s.root, entry.Name(), selector))
		}
	}
	return rows, nil
}

func featureRow(root fs.FS, featureID string, selector guide.Selector) cockpit.FeatureRow {
	state := workflow.EvaluateState(root, featureID)
	guidance := guide.Evaluate(root, featureID, selector)
	// The detail is what "workflow state" and "workflow guide" print.
	var detail bytes.Buffer
	if err := report.WriteWorkflowStateText(&detail, state); err != nil {
		fmt.Fprintf(&detail, "write workflow state report: %v\n", err)
	}
	if !guidance.Valid() {
		fmt.Fprintf(&detail, "ai-dev: internal error: workflow guide report cannot be rendered for feature %q\n", featureID)
	} else if err := report.WriteGuideText(&detail, guidance); err != nil {
		fmt.Fprintf(&detail, "write workflow guide report: %v\n", err)
	}
	row := cockpit.FeatureRow{
		Name:   cockpit.Sanitize(featureID),
		ID:     featureID,
		State:  cockpit.Sanitize(string(state.Outcome)),
		Next:   cockpit.Sanitize(string(guidance.Outcome)),
		Detail: cockpit.SanitizeLines(detail.String()),
	}
	if state.Outcome == workflow.StateReported {
		row.State, row.Recognized, row.Terminal = cockpit.Sanitize(string(state.State)), true, state.Terminal
	}
	if guidance.Outcome == sdd.StepRecommended {
		row.Next = cockpit.Sanitize(string(guidance.Outcome) + ": " + string(guidance.Activity))
		row.Role = cockpit.Sanitize(string(guidance.Role))
		row.Agent = cockpit.Sanitize(string(guidance.Agent))
		row.Availability = cockpit.Sanitize(string(guidance.Availability))
		row.Source = cockpit.Sanitize(string(guidance.Source))
		row.AgentID = string(guidance.Agent)
		row.StepRecommended = true
		row.AgentAvailable = guidance.Availability == guide.Available
	}
	return row
}

// LoadEnvironment is the text of "ai-dev doctor".
func (s cockpitSource) LoadEnvironment(ctx context.Context) ([]cockpit.Text, error) {
	var out bytes.Buffer
	if err := report.WriteText(&out, RunDoctor(ctx, s.executor, s.root)); err != nil {
		return nil, err
	}
	return cockpit.SanitizeLines(out.String()), nil
}

// LoadAgents is the text of "ai-dev setup show".
func (s cockpitSource) LoadAgents(context.Context) ([]cockpit.Text, error) {
	var out bytes.Buffer
	if err := report.WriteSetupText(&out, setupShowReport(s.executor, s.root)); err != nil {
		return nil, err
	}
	return cockpit.SanitizeLines(out.String()), nil
}

// LoadRepository is the text of "ai-dev init inspect".
func (s cockpitSource) LoadRepository(ctx context.Context) ([]cockpit.Text, error) {
	inspection, err := inspectProject(ctx, s.git, ".")
	if err != nil {
		return nil, fmt.Errorf("init inspect: %v", err)
	}
	var out bytes.Buffer
	if err := report.WriteGitInspectionText(&out, inspection); err != nil {
		return nil, err
	}
	return cockpit.SanitizeLines(out.String()), nil
}

// cockpitRunner runs this ai-dev binary with an action's argument vector,
// on the terminal itself (R-03). There is no shell anywhere.
type cockpitRunner struct {
	ctx            context.Context
	launcher       system.Launcher
	executable     string
	gate           *signalGate
	stdin          io.Reader
	stdout, stderr io.Writer
}

func (r cockpitRunner) Command(args []string) tea.ExecCommand {
	return &cockpitCommand{runner: r, args: append([]string(nil), args...)}
}

// Preview renders the command line of the review: plain words bare, any
// other argument in the 004 shell-inert quoted form.
func (r cockpitRunner) Preview(args []string) string {
	words := []string{"ai-dev"}
	for _, arg := range args {
		if plainWord(arg) {
			words = append(words, arg)
		} else {
			words = append(words, report.ShellInertQuote(arg))
		}
	}
	return strings.Join(words, " ")
}

// plainWord reports an argument a POSIX shell reads as itself.
func plainWord(arg string) bool {
	if arg == "" {
		return false
	}
	for _, r := range arg {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./=:,+@%", r)) {
			return false
		}
	}
	return true
}

// cockpitCommand is one action handed to the terminal. It uses the
// terminal streams RunCockpit received, so Bubble Tea's Set methods are
// ignored.
type cockpitCommand struct {
	runner cockpitRunner
	args   []string
}

func (*cockpitCommand) SetStdin(io.Reader)  {}
func (*cockpitCommand) SetStdout(io.Writer) {}
func (*cockpitCommand) SetStderr(io.Writer) {}

// Run launches the command, waits for it, shows its exit code and waits
// for Enter so its output can be read (F012-FR-010). The launch context is
// never cancelled by the cockpit: only the terminal's own interrupt reaches
// the command.
func (c *cockpitCommand) Run() error {
	r := c.runner
	r.gate.begin()
	defer r.gate.end()
	handle, err := r.launcher.Launch(r.ctx, system.Spec{Name: r.executable, Args: c.args, Stdin: r.stdin, Stdout: r.stdout, Stderr: r.stderr})
	if err != nil {
		return fmt.Errorf("launch: %v", err)
	}
	status, waitErr := handle.Wait()
	var result error
	switch {
	case status.Exited:
		fmt.Fprintf(r.stdout, "\nai-dev tui: the command exited with code %d. Press Enter to return.\n", status.ExitCode)
		if status.ExitCode != 0 {
			result = cockpit.CommandExit{Code: status.ExitCode}
		}
	case status.ExitCode == -1:
		fmt.Fprintln(r.stdout, "\nai-dev tui: the command was terminated by a signal. Press Enter to return.")
		result = cockpit.CommandExit{Signaled: true}
	default:
		return fmt.Errorf("wait: %v", waitErr)
	}
	// A termination received meanwhile is honored without waiting for a
	// person (review F4).
	if !r.gate.terminating() {
		readLine(r.stdin)
	}
	return result
}

// readLine consumes input up to and including a newline, one byte at a
// time, so nothing meant for the cockpit is buffered away.
func readLine(reader io.Reader) {
	var b [1]byte
	for {
		n, err := reader.Read(b[:])
		if err != nil || (n == 1 && (b[0] == '\n' || b[0] == '\r')) {
			return
		}
	}
}
