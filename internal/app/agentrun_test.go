package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"ai-dev/internal/system"
)

type appRunExecutor struct{ paths map[string]string }

func (e appRunExecutor) LookPath(name string) (string, error) {
	if path := e.paths[name]; path != "" {
		return path, nil
	}
	return "", errors.New("not found")
}
func (appRunExecutor) Run(context.Context, string, ...string) (string, error) {
	return "", errors.New("unexpected Run")
}

type appRunLauncher struct {
	launches  int
	specs     []system.Spec
	launchErr error
	status    system.ExitStatus
	waitErr   error
}

func (l *appRunLauncher) Launch(_ context.Context, spec system.Spec) (system.Handle, error) {
	l.launches++
	l.specs = append(l.specs, spec)
	if l.launchErr != nil {
		return nil, l.launchErr
	}
	_, _ = io.WriteString(spec.Stdout, "stdout-one\nstdout-two\n")
	_, _ = io.WriteString(spec.Stderr, "stderr-one\nstderr-two")
	return appRunHandle{status: l.status, err: l.waitErr}, nil
}

type appRunHandle struct {
	status system.ExitStatus
	err    error
}

func (h appRunHandle) Wait() (system.ExitStatus, error) { return h.status, h.err }

func availableAgents() appRunExecutor {
	return appRunExecutor{paths: map[string]string{"claude": "/tools/claude", "codex": "/tools/codex"}}
}

func TestRunAgentRunBuildsEquivalentArgumentAndStdinRequests(t *testing.T) {
	workdir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name  string
		args  []string
		stdin io.Reader
	}{
		{"argument", []string{"claude-code", "literal $HOME\nline two"}, bytes.NewReader(nil)},
		{"stdin", []string{"claude-code"}, bytes.NewBufferString("literal $HOME\nline two")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			launcher := &appRunLauncher{status: system.ExitStatus{Exited: true, ExitCode: 0}}
			var stdout, stderr bytes.Buffer
			code := RunAgentRun(context.Background(), tt.args, availableAgents(), launcher, tt.stdin, false, workdir, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("code = %d, stderr = %q", code, stderr.String())
			}
			if launcher.launches != 1 || len(launcher.specs) != 1 {
				t.Fatalf("launches = %d, specs = %d", launcher.launches, len(launcher.specs))
			}
			spec := launcher.specs[0]
			if spec.Name != "claude" || !reflect.DeepEqual(spec.Args, []string{"--print", "literal $HOME\nline two"}) || spec.Dir != workdir {
				t.Fatalf("spec = %#v", spec)
			}
			if stdout.String() != "stdout-one\nstdout-two\n" {
				t.Fatalf("stdout = %q", stdout.String())
			}
			if got := stderr.String(); got != "stderr-one\nstderr-two\nai-dev: outcome=agent_succeeded agent=claude-code\n" {
				t.Fatalf("stderr = %q", got)
			}
		})
	}
}

func TestRunAgentRunPassesInteractiveStdinWithArgumentToSelectedAgent(t *testing.T) {
	launcher := &appRunLauncher{status: system.ExitStatus{Exited: true, ExitCode: 0}}
	stdin := bytes.NewBufferString("native interaction")
	var stdout, stderr bytes.Buffer
	code := RunAgentRun(context.Background(), []string{"claude-code", "do it"}, availableAgents(), launcher, stdin, true, "/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if len(launcher.specs) != 1 || launcher.specs[0].Stdin != io.Reader(stdin) {
		t.Fatalf("spec stdin = %#v, want original interactive stdin", launcher.specs)
	}
}

func TestRunAgentRunJSONRoutesAgentStreamsToStderrAndIsolatesReport(t *testing.T) {
	launcher := &appRunLauncher{status: system.ExitStatus{Exited: true, ExitCode: 0}}
	var stdout, stderr bytes.Buffer
	code := RunAgentRun(context.Background(), []string{"--format", "json", "codex-cli", "do it"}, availableAgents(), launcher, bytes.NewReader(nil), false, "/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if got := stderr.String(); got != "stdout-one\nstdout-two\nstderr-one\nstderr-two" {
		t.Fatalf("stderr = %q", got)
	}
	var got struct {
		AgentID     string `json:"agent_id"`
		Outcome     string `json:"outcome"`
		Explanation string `json:"explanation"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout = %q: %v", stdout.String(), err)
	}
	if got.AgentID != "codex-cli" || got.Outcome != "agent_succeeded" {
		t.Fatalf("report = %#v", got)
	}
	if len(launcher.specs) != 1 || !reflect.DeepEqual(launcher.specs[0].Args, []string{"exec", "do it"}) {
		t.Fatalf("specs = %#v", launcher.specs)
	}
}

func TestRunAgentRunJSONContractAcrossAllOutcomes(t *testing.T) {
	tests := []struct {
		name        string
		status      system.ExitStatus
		waitErr     error
		wantOutcome string
		wantCode    int
	}{
		{
			name:        "agent succeeded",
			status:      system.ExitStatus{Exited: true, ExitCode: 0},
			wantOutcome: "agent_succeeded",
			wantCode:    0,
		},
		{
			name:        "agent failed",
			status:      system.ExitStatus{Exited: true, ExitCode: 7},
			waitErr:     errors.New("exit status 7"),
			wantOutcome: "agent_failed",
			wantCode:    1,
		},
		{
			name:        "execution failed",
			status:      system.ExitStatus{Exited: false, ExitCode: -1},
			waitErr:     errors.New("signal: terminated"),
			wantOutcome: "execution_failed",
			wantCode:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			launcher := &appRunLauncher{status: tt.status, waitErr: tt.waitErr}
			var stdout, stderr bytes.Buffer
			code := RunAgentRun(
				context.Background(),
				[]string{"--format", "json", "codex-cli", "do it"},
				availableAgents(),
				launcher,
				bytes.NewReader(nil),
				false,
				"/project",
				&stdout,
				&stderr,
			)
			if code != tt.wantCode {
				t.Fatalf("code = %d, want %d; stdout=%q stderr=%q", code, tt.wantCode, stdout.String(), stderr.String())
			}
			if got := stderr.String(); got != "stdout-one\nstdout-two\nstderr-one\nstderr-two" {
				t.Fatalf("live agent output = %q", got)
			}
			var got struct {
				AgentID     string `json:"agent_id"`
				Outcome     string `json:"outcome"`
				Explanation string `json:"explanation"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
				t.Fatalf("stdout is not one isolated JSON report: %q: %v", stdout.String(), err)
			}
			if got.AgentID != "codex-cli" || got.Outcome != tt.wantOutcome {
				t.Fatalf("report = %#v, want outcome %q", got, tt.wantOutcome)
			}
			if tt.wantOutcome != "agent_succeeded" && got.Explanation == "" {
				t.Fatalf("failure report has empty explanation: %#v", got)
			}
			if tt.wantOutcome != "agent_succeeded" && strings.Contains(stdout.String(), `"outcome":"agent_succeeded"`) {
				t.Fatalf("failure report resembles success: %q", stdout.String())
			}
		})
	}
}

func TestRunDispatchesAgentRunToExactlyOneSelectedAgent(t *testing.T) {
	launcher := &appRunLauncher{status: system.ExitStatus{Exited: true, ExitCode: 0}}
	var stdout, stderr bytes.Buffer
	code := Run(
		context.Background(),
		[]string{"agent", "run", "codex-cli", "do it"},
		availableAgents(),
		launcher,
		readyFS(),
		bytes.NewReader(nil),
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if launcher.launches != 1 || len(launcher.specs) != 1 || launcher.specs[0].Name != "codex" {
		t.Fatalf("launches = %d, specs = %#v", launcher.launches, launcher.specs)
	}
}

type appCancellationLauncher struct{ launches int }

func (l *appCancellationLauncher) Launch(ctx context.Context, _ system.Spec) (system.Handle, error) {
	l.launches++
	return appCancellationHandle{ctx: ctx}, nil
}

type appCancellationHandle struct{ ctx context.Context }

func (h appCancellationHandle) Wait() (system.ExitStatus, error) {
	<-h.ctx.Done()
	return system.ExitStatus{Exited: false, ExitCode: -1, Canceled: true}, h.ctx.Err()
}

func TestRunAgentRunHandledCancellationReportsFailureAndExitsOne(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	launcher := &appCancellationLauncher{}
	var stdout, stderr bytes.Buffer
	code := RunAgentRun(ctx, []string{"claude-code", "stop"}, availableAgents(), launcher, bytes.NewReader(nil), false, "/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if launcher.launches != 1 || stdout.Len() != 0 {
		t.Fatalf("launches=%d stdout=%q", launcher.launches, stdout.String())
	}
	if got := stderr.String(); !bytes.Contains([]byte(got), []byte("outcome=execution_failed")) || !bytes.Contains([]byte(got), []byte("agent=claude-code")) {
		t.Fatalf("stderr = %q", got)
	}
}

func TestRunAgentRunNormalNonZeroExitReportsAgentFailureAndExitsOne(t *testing.T) {
	launcher := &appRunLauncher{status: system.ExitStatus{Exited: true, ExitCode: 9}}
	var stdout, stderr bytes.Buffer
	code := RunAgentRun(context.Background(), []string{"codex-cli", "fail"}, availableAgents(), launcher, bytes.NewReader(nil), false, "/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if launcher.launches != 1 {
		t.Fatalf("launches = %d, want 1", launcher.launches)
	}
	if got := stderr.String(); !bytes.Contains([]byte(got), []byte("outcome=agent_failed")) || bytes.Contains([]byte(got), []byte("outcome=agent_succeeded")) {
		t.Fatalf("stderr = %q", got)
	}
}

func TestRunAgentRunFailureScenariosProduceOnlyTheExpectedOutcome(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		stdin       io.Reader
		interactive bool
		executor    system.Executor
		launcher    *appRunLauncher
		wantOutcome string
		wantAgent   string
		wantLaunch  int
		wantOutput  bool
	}{
		{
			name: "missing instruction", args: []string{"claude-code"}, stdin: bytes.NewReader(nil),
			executor: availableAgents(), launcher: &appRunLauncher{}, wantOutcome: "execution_failed", wantAgent: "claude-code",
		},
		{
			name: "zero-length instruction", args: []string{"claude-code", ""}, stdin: bytes.NewReader(nil),
			executor: availableAgents(), launcher: &appRunLauncher{}, wantOutcome: "execution_failed", wantAgent: "claude-code",
		},
		{
			name: "whitespace-only instruction", args: []string{"claude-code", " \t\n"}, stdin: bytes.NewReader(nil),
			executor: availableAgents(), launcher: &appRunLauncher{}, wantOutcome: "execution_failed", wantAgent: "claude-code",
		},
		{
			name: "interactive stdin", args: []string{"claude-code"}, stdin: bytes.NewBufferString("must not read"), interactive: true,
			executor: availableAgents(), launcher: &appRunLauncher{}, wantOutcome: "execution_failed", wantAgent: "claude-code",
		},
		{
			name: "ambiguous argument and stdin", args: []string{"claude-code", "argument"}, stdin: bytes.NewBufferString("stdin"),
			executor: availableAgents(), launcher: &appRunLauncher{}, wantOutcome: "execution_failed", wantAgent: "claude-code",
		},
		{
			name: "unsupported agent", args: []string{"not-supported", "do it"}, stdin: bytes.NewReader(nil),
			executor: availableAgents(), launcher: &appRunLauncher{}, wantOutcome: "execution_failed", wantAgent: "not-supported",
		},
		{
			name: "unavailable agent", args: []string{"codex-cli", "do it"}, stdin: bytes.NewReader(nil),
			executor: appRunExecutor{paths: map[string]string{"claude": "/tools/claude"}}, launcher: &appRunLauncher{}, wantOutcome: "execution_failed", wantAgent: "codex-cli",
		},
		{
			name: "process launch failure", args: []string{"claude-code", "do it"}, stdin: bytes.NewReader(nil),
			executor: availableAgents(), launcher: &appRunLauncher{launchErr: errors.New("start failed")}, wantOutcome: "execution_failed", wantAgent: "claude-code", wantLaunch: 1,
		},
		{
			name: "agent failed", args: []string{"codex-cli", "do it"}, stdin: bytes.NewReader(nil),
			executor: availableAgents(), launcher: &appRunLauncher{status: system.ExitStatus{Exited: true, ExitCode: 4}, waitErr: errors.New("exit status 4")}, wantOutcome: "agent_failed", wantAgent: "codex-cli", wantLaunch: 1, wantOutput: true,
		},
		{
			name: "abnormal execution failure", args: []string{"claude-code", "do it"}, stdin: bytes.NewReader(nil),
			executor: availableAgents(), launcher: &appRunLauncher{status: system.ExitStatus{Exited: false, ExitCode: -1}, waitErr: errors.New("signal: killed")}, wantOutcome: "execution_failed", wantAgent: "claude-code", wantLaunch: 1, wantOutput: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := RunAgentRun(context.Background(), tt.args, tt.executor, tt.launcher, tt.stdin, tt.interactive, "/project", &stdout, &stderr)
			if code != 1 {
				t.Fatalf("code = %d, want 1; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if tt.launcher.launches != tt.wantLaunch {
				t.Fatalf("launches = %d, want %d", tt.launcher.launches, tt.wantLaunch)
			}
			if tt.wantOutput && stdout.String() != "stdout-one\nstdout-two\n" {
				t.Fatalf("agent stdout = %q", stdout.String())
			}
			if !tt.wantOutput && stdout.Len() != 0 {
				t.Fatalf("pre-launch failure wrote stdout: %q", stdout.String())
			}
			got := stderr.String()
			wantRecord := "outcome=" + tt.wantOutcome + " agent=" + tt.wantAgent
			if !strings.Contains(got, wantRecord) {
				t.Fatalf("stderr %q missing %q", got, wantRecord)
			}
			if strings.Contains(got, "outcome=agent_succeeded") {
				t.Fatalf("failure report contains success outcome: %q", got)
			}
		})
	}
}

func TestRunAgentRunWorkingDirectoryFailureUsesExecutionReport(t *testing.T) {
	original := getWorkingDirectory
	getWorkingDirectory = func() (string, error) { return "", errors.New("cwd unavailable") }
	t.Cleanup(func() { getWorkingDirectory = original })

	for _, formatArgs := range [][]string{nil, {"--format", "json"}} {
		t.Run(strings.Join(formatArgs, "_"), func(t *testing.T) {
			launcher := &appRunLauncher{}
			var stdout, stderr bytes.Buffer
			args := append(append([]string(nil), formatArgs...), "claude-code", "do it")
			code := RunAgentRun(context.Background(), args, availableAgents(), launcher, bytes.NewReader(nil), false, "", &stdout, &stderr)
			if code != 1 || launcher.launches != 0 {
				t.Fatalf("code=%d launches=%d stdout=%q stderr=%q", code, launcher.launches, stdout.String(), stderr.String())
			}
			if len(formatArgs) == 0 {
				if !strings.Contains(stderr.String(), "outcome=execution_failed") || !strings.Contains(stderr.String(), "determine current working directory") {
					t.Fatalf("stderr = %q", stderr.String())
				}
				return
			}
			var got struct {
				AgentID     string `json:"agent_id"`
				Outcome     string `json:"outcome"`
				Explanation string `json:"explanation"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
				t.Fatalf("stdout = %q: %v", stdout.String(), err)
			}
			if got.AgentID != "claude-code" || got.Outcome != "execution_failed" || !strings.Contains(got.Explanation, "determine current working directory") {
				t.Fatalf("report = %#v", got)
			}
		})
	}

	t.Run("usage failure remains distinct", func(t *testing.T) {
		launcher := &appRunLauncher{}
		var stdout, stderr bytes.Buffer
		code := RunAgentRun(context.Background(), []string{"--bogus", "claude-code", "do it"}, availableAgents(), launcher, bytes.NewReader(nil), false, "", &stdout, &stderr)
		if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "Usage:") || strings.Contains(stderr.String(), "outcome=") {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
		if launcher.launches != 0 {
			t.Fatalf("usage failure launched %d processes", launcher.launches)
		}
	})
}

type countingRunExecutor struct {
	lookups int
	paths   map[string]string
}

func (e *countingRunExecutor) LookPath(name string) (string, error) {
	e.lookups++
	if path := e.paths[name]; path != "" {
		return path, nil
	}
	return "", errors.New("not found")
}

func (*countingRunExecutor) Run(context.Context, string, ...string) (string, error) {
	return "", errors.New("unexpected Run")
}

func TestRunAgentRunSeparatesUsageFailuresFromRecognizedExecutionFailures(t *testing.T) {
	usageCases := []struct {
		name string
		args []string
	}{
		{name: "missing agent id", args: nil},
		{name: "too many positionals", args: []string{"claude-code", "one", "two"}},
		{name: "unknown flag", args: []string{"--bogus", "claude-code", "do it"}},
		{name: "invalid format", args: []string{"--format", "yaml", "claude-code", "do it"}},
	}
	for _, tt := range usageCases {
		t.Run("usage/"+tt.name, func(t *testing.T) {
			executor := &countingRunExecutor{paths: map[string]string{"claude": "/tools/claude"}}
			launcher := &appRunLauncher{}
			var stdout, stderr bytes.Buffer
			code := RunAgentRun(context.Background(), tt.args, executor, launcher, bytes.NewReader(nil), false, "/project", &stdout, &stderr)
			if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "Usage:") {
				t.Fatalf("code/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
			}
			if executor.lookups != 0 || launcher.launches != 0 {
				t.Fatalf("usage failure performed discovery/launch: lookups=%d launches=%d", executor.lookups, launcher.launches)
			}
			if strings.Contains(stderr.String(), "outcome=") {
				t.Fatalf("usage failure emitted execution report: %q", stderr.String())
			}
		})
	}

	recognizedCases := []struct {
		name string
		args []string
	}{
		{name: "missing instruction", args: []string{"claude-code"}},
		{name: "unsupported agent", args: []string{"not-supported", "do it"}},
	}
	for _, tt := range recognizedCases {
		t.Run("recognized/"+tt.name, func(t *testing.T) {
			executor := &countingRunExecutor{paths: map[string]string{"claude": "/tools/claude"}}
			launcher := &appRunLauncher{}
			var stdout, stderr bytes.Buffer
			code := RunAgentRun(context.Background(), tt.args, executor, launcher, bytes.NewReader(nil), false, "/project", &stdout, &stderr)
			if code != 1 || stdout.Len() != 0 {
				t.Fatalf("code/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
			}
			if strings.Count(stderr.String(), "ai-dev: outcome=execution_failed") != 1 {
				t.Fatalf("recognized failure did not emit exactly one report: %q", stderr.String())
			}
			if launcher.launches != 0 {
				t.Fatalf("recognized validation failure launched %d processes", launcher.launches)
			}
		})
	}
}
