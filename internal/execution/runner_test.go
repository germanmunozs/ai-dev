package execution

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/agent"
	"github.com/germanmunozs/ai-dev/internal/system"
)

type runnerExecutor struct{ paths map[string]string }

func (e runnerExecutor) LookPath(name string) (string, error) {
	if path := e.paths[name]; path != "" {
		return path, nil
	}
	return "", errors.New("not found")
}
func (runnerExecutor) Run(context.Context, string, ...string) (string, error) {
	return "", errors.New("unexpected Run")
}

type recordingAdapter struct {
	name, workdir, instruction string
	args                       []string
	exitCodes                  []int
}

func (a *recordingAdapter) Command(workdir, instruction string) (string, []string) {
	a.workdir, a.instruction = workdir, instruction
	return a.name, append([]string(nil), a.args...)
}
func (a *recordingAdapter) Classify(code int) Outcome {
	a.exitCodes = append(a.exitCodes, code)
	if code == 0 {
		return AgentSucceeded
	}
	return AgentFailed
}

type runnerLauncher struct {
	launches  int
	spec      system.Spec
	launchErr error
	status    system.ExitStatus
	waitErr   error
}

func (l *runnerLauncher) Launch(_ context.Context, spec system.Spec) (system.Handle, error) {
	l.launches++
	l.spec = spec
	if l.launchErr != nil {
		return nil, l.launchErr
	}
	return runnerHandle{status: l.status, err: l.waitErr}, nil
}

type runnerHandle struct {
	status system.ExitStatus
	err    error
}

func (h runnerHandle) Wait() (system.ExitStatus, error) { return h.status, h.err }

func TestRunnerPropagatesRequestAndLaunchesExactlyOnce(t *testing.T) {
	adapter := &recordingAdapter{name: "fixture-agent", args: []string{"one-shot"}}
	launcher := &runnerLauncher{status: system.ExitStatus{Exited: true, ExitCode: 0}}
	runner := Runner{
		Registry: agent.NewRegistry(runnerExecutor{paths: map[string]string{"claude": "/bin/claude"}}),
		Launcher: launcher,
		Adapters: map[agent.ID]Adapter{agent.ClaudeCode: adapter},
	}
	var stdout, stderr bytes.Buffer
	report := runner.Run(context.Background(), Request{
		AgentID: agent.ClaudeCode, Instruction: "literal $HOME\nsecond line",
	}, "/work tree", &stdout, &stderr)

	if report.Outcome != AgentSucceeded || report.RequestedAgentID != "claude-code" {
		t.Fatalf("report = %#v", report)
	}
	if launcher.launches != 1 {
		t.Fatalf("launches = %d, want 1", launcher.launches)
	}
	if adapter.workdir != "/work tree" || adapter.instruction != "literal $HOME\nsecond line" {
		t.Fatalf("adapter received workdir=%q instruction=%q", adapter.workdir, adapter.instruction)
	}
	if launcher.spec.Name != "fixture-agent" || !reflect.DeepEqual(launcher.spec.Args, []string{"one-shot"}) || launcher.spec.Dir != "/work tree" {
		t.Fatalf("spec = %#v", launcher.spec)
	}
	if launcher.spec.Stdout != io.Writer(&stdout) || launcher.spec.Stderr != io.Writer(&stderr) {
		t.Fatal("runner did not preserve stream destinations")
	}
	if !reflect.DeepEqual(adapter.exitCodes, []int{0}) {
		t.Fatalf("classified exit codes = %v", adapter.exitCodes)
	}
}

func TestRunnerPassesStdinThroughToAgentFixture(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell fixture")
	}
	fixture, err := filepath.Abs(filepath.Join("..", "system", "testdata", "stdin-agent.sh"))
	if err != nil {
		t.Fatalf("fixture path: %v", err)
	}
	adapter := &recordingAdapter{name: fixture}
	stdin := bytes.NewBufferString("approved by user\n")
	runner := Runner{
		Registry: agent.NewRegistry(runnerExecutor{paths: map[string]string{"claude": "/bin/claude"}}),
		Launcher: system.OSLauncher{},
		Adapters: map[agent.ID]Adapter{agent.ClaudeCode: adapter},
		Stdin:    stdin,
	}
	var stdout bytes.Buffer
	report := runner.Run(context.Background(), Request{
		AgentID: agent.ClaudeCode, Instruction: "prompt first",
	}, "", &stdout, io.Discard)

	if report.Outcome != AgentSucceeded {
		t.Fatalf("report = %#v", report)
	}
	if got := stdout.String(); got != "stdin=approved by user\n" {
		t.Fatalf("stdout = %q", got)
	}
}

type cancellationLauncher struct {
	launches int
	started  chan struct{}
	stopped  chan struct{}
}

func (l *cancellationLauncher) Launch(ctx context.Context, _ system.Spec) (system.Handle, error) {
	l.launches++
	close(l.started)
	return cancellationHandle{ctx: ctx, stopped: l.stopped}, nil
}

type cancellationHandle struct {
	ctx     context.Context
	stopped chan struct{}
}

func (h cancellationHandle) Wait() (system.ExitStatus, error) {
	<-h.ctx.Done()
	close(h.stopped)
	return system.ExitStatus{Exited: false, ExitCode: -1, Canceled: true}, h.ctx.Err()
}

func TestRunnerWaitsForHandledCancellationAndDoesNotRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	launcher := &cancellationLauncher{started: make(chan struct{}), stopped: make(chan struct{})}
	adapter := &recordingAdapter{name: "fixture-agent"}
	runner := Runner{
		Registry: agent.NewRegistry(runnerExecutor{paths: map[string]string{"claude": "/bin/claude", "codex": "/bin/codex"}}),
		Launcher: launcher,
		Adapters: map[agent.ID]Adapter{
			agent.ClaudeCode: adapter,
			agent.CodexCLI:   &recordingAdapter{name: "must-not-run"},
		},
	}
	reportCh := make(chan Report, 1)
	go func() {
		reportCh <- runner.Run(ctx, Request{AgentID: agent.ClaudeCode, Instruction: "stop"}, "/work", io.Discard, io.Discard)
	}()
	<-launcher.started
	cancel()
	report := <-reportCh
	if report.Outcome != ExecutionFailed {
		t.Fatalf("outcome = %q, want %q", report.Outcome, ExecutionFailed)
	}
	if launcher.launches != 1 {
		t.Fatalf("launches = %d, want 1", launcher.launches)
	}
	select {
	case <-launcher.stopped:
	default:
		t.Fatal("Run returned before child stop was observed")
	}
	if len(adapter.exitCodes) != 0 {
		t.Fatalf("adapter classified canceled process: %v", adapter.exitCodes)
	}
}

type completedThenCanceledHandle struct {
	cancel context.CancelFunc
}

func (h completedThenCanceledHandle) Wait() (system.ExitStatus, error) {
	h.cancel()
	return system.ExitStatus{Exited: true, ExitCode: 0}, nil
}

type completedThenCanceledLauncher struct {
	cancel context.CancelFunc
}

func (l completedThenCanceledLauncher) Launch(context.Context, system.Spec) (system.Handle, error) {
	return completedThenCanceledHandle{cancel: l.cancel}, nil
}

func TestRunnerRetainsNaturalCompletionWhenCancellationBecomesObservableAfterWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	adapter := &recordingAdapter{name: "fixture-agent"}
	runner := Runner{
		Registry: agent.NewRegistry(runnerExecutor{paths: map[string]string{"claude": "/bin/claude"}}),
		Launcher: completedThenCanceledLauncher{cancel: cancel},
		Adapters: map[agent.ID]Adapter{agent.ClaudeCode: adapter},
	}

	report := runner.Run(ctx, Request{AgentID: agent.ClaudeCode, Instruction: "finish"}, "/work", io.Discard, io.Discard)
	if report.Outcome != AgentSucceeded {
		t.Fatalf("report = %#v, want natural completion retained", report)
	}
	if !reflect.DeepEqual(adapter.exitCodes, []int{0}) {
		t.Fatalf("classified exit codes = %v, want [0]", adapter.exitCodes)
	}
}

func TestRunnerLifecycleOutcomeClassification(t *testing.T) {
	waitFailure := errors.New("wait communication failed")
	launchFailure := errors.New("executable disappeared")
	abnormalFailure := errors.New("terminated by signal")
	tests := []struct {
		name             string
		requestID        agent.ID
		paths            map[string]string
		includeAdapter   bool
		launcher         *runnerLauncher
		wantOutcome      Outcome
		wantLaunches     int
		wantClassifyCode []int
	}{
		{
			name: "unsupported agent", requestID: "future-agent", paths: map[string]string{"claude": "/bin/claude"},
			includeAdapter: true, launcher: &runnerLauncher{}, wantOutcome: ExecutionFailed, wantLaunches: 0,
		},
		{
			name: "supported but unavailable", requestID: agent.ClaudeCode, paths: map[string]string{},
			includeAdapter: true, launcher: &runnerLauncher{}, wantOutcome: ExecutionFailed, wantLaunches: 0,
		},
		{
			name: "missing adapter invariant", requestID: agent.ClaudeCode, paths: map[string]string{"claude": "/bin/claude"},
			launcher: &runnerLauncher{}, wantOutcome: ExecutionFailed, wantLaunches: 0,
		},
		{
			name: "process launch failure", requestID: agent.ClaudeCode, paths: map[string]string{"claude": "/bin/claude"},
			includeAdapter: true, launcher: &runnerLauncher{launchErr: launchFailure}, wantOutcome: ExecutionFailed, wantLaunches: 1,
		},
		{
			name: "wait communication failure", requestID: agent.ClaudeCode, paths: map[string]string{"claude": "/bin/claude"},
			includeAdapter: true, launcher: &runnerLauncher{status: system.ExitStatus{Exited: true, ExitCode: 0}, waitErr: waitFailure}, wantOutcome: ExecutionFailed, wantLaunches: 1,
		},
		{
			name: "abnormal termination", requestID: agent.ClaudeCode, paths: map[string]string{"claude": "/bin/claude"},
			includeAdapter: true, launcher: &runnerLauncher{status: system.ExitStatus{Exited: false, ExitCode: -1}, waitErr: abnormalFailure}, wantOutcome: ExecutionFailed, wantLaunches: 1,
		},
		{
			name: "normal non-zero agent failure", requestID: agent.ClaudeCode, paths: map[string]string{"claude": "/bin/claude"},
			includeAdapter: true, launcher: &runnerLauncher{status: system.ExitStatus{Exited: true, ExitCode: 7}, waitErr: errors.New("exit status 7")}, wantOutcome: AgentFailed, wantLaunches: 1, wantClassifyCode: []int{7},
		},
		{
			name: "normal zero agent success", requestID: agent.ClaudeCode, paths: map[string]string{"claude": "/bin/claude"},
			includeAdapter: true, launcher: &runnerLauncher{status: system.ExitStatus{Exited: true, ExitCode: 0}}, wantOutcome: AgentSucceeded, wantLaunches: 1, wantClassifyCode: []int{0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := &recordingAdapter{name: "fixture-agent"}
			adapters := map[agent.ID]Adapter{}
			if tt.includeAdapter {
				adapters[agent.ClaudeCode] = adapter
			}
			runner := Runner{
				Registry: agent.NewRegistry(runnerExecutor{paths: tt.paths}),
				Launcher: tt.launcher,
				Adapters: adapters,
			}
			report := runner.Run(context.Background(), Request{AgentID: tt.requestID, Instruction: "do it"}, "/work", io.Discard, io.Discard)
			if report.Outcome != tt.wantOutcome {
				t.Fatalf("outcome = %q, want %q; report=%#v", report.Outcome, tt.wantOutcome, report)
			}
			if report.RequestedAgentID != string(tt.requestID) {
				t.Fatalf("requested agent = %q, want exact %q", report.RequestedAgentID, tt.requestID)
			}
			if tt.wantOutcome != AgentSucceeded && report.Explanation == "" {
				t.Fatalf("failure report has empty explanation: %#v", report)
			}
			if tt.launcher.launches != tt.wantLaunches {
				t.Fatalf("launches = %d, want %d", tt.launcher.launches, tt.wantLaunches)
			}
			if !reflect.DeepEqual(adapter.exitCodes, tt.wantClassifyCode) {
				t.Fatalf("classified exit codes = %v, want %v", adapter.exitCodes, tt.wantClassifyCode)
			}
		})
	}
}
