package gitprocess

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/germanmunozs/ai-dev/internal/gitmanager"
)

var (
	// ErrInvalidInvocation reports an invocation the runner refuses to
	// start: it was not produced by the operation builder.
	ErrInvalidInvocation = errors.New("invalid Git invocation")
	// ErrEnvironmentLeafPresent reports that a private HOME or
	// XDG_CONFIG_HOME leaf existed before or after an invocation; the
	// private environment can no longer be proven inert.
	ErrEnvironmentLeafPresent = errors.New("private Git environment leaf exists")
)

// processStopDelay bounds how long Run waits for output pipes after the
// process is stopped.
const processStopDelay = time.Second

// Runner executes exactly one validated Git invocation: the executable is
// started directly with its argv (no shell), with only the listed
// environment, and its output is captured within the invocation bounds.
type Runner struct{}

var _ gitmanager.GitRunner = Runner{}

func (Runner) Run(ctx context.Context, invocation gitmanager.GitInvocation) (gitmanager.GitRunResult, error) {
	leaves, err := validateInvocation(invocation)
	if err != nil {
		return gitmanager.GitRunResult{}, err
	}
	if err := requireAbsent(leaves); err != nil {
		return gitmanager.GitRunResult{}, err
	}

	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	stdout := &boundedBuffer{limit: invocation.StdoutLimit, overflow: stop}
	stderr := &boundedBuffer{limit: invocation.StderrLimit, overflow: stop}
	cmd := exec.CommandContext(runCtx, invocation.Executable, invocation.Args...)
	cmd.Dir = invocation.Directory
	// A non-nil environment is essential: nil would inherit ours.
	cmd.Env = append([]string{}, invocation.Environment...)
	cmd.Stdin = bytes.NewReader(invocation.Stdin)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	cmd.WaitDelay = processStopDelay
	isolateProcessGroup(cmd)
	runErr := cmd.Run()

	result := gitmanager.GitRunResult{
		Stdout: stdout.data, Stderr: stderr.data, ExitCode: -1,
		StdoutObserved: stdout.observed, StderrObserved: stderr.observed,
	}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	if err := requireAbsent(leaves); err != nil {
		return result, err
	}
	if stdout.overflowed || stderr.overflowed {
		return result, fmt.Errorf("%w: stdout %d of %d bytes, stderr %d of %d bytes", gitmanager.ErrCapturedOutputExceeded,
			stdout.observed, stdout.limit, stderr.observed, stderr.limit)
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	var exitErr *exec.ExitError
	if runErr != nil && !errors.As(runErr, &exitErr) {
		return result, fmt.Errorf("run git: %w", runErr)
	}
	// A normal non-zero exit is a result, not a runner failure.
	return result, nil
}

// validateInvocation checks the invocation's shape and returns the private
// HOME and XDG_CONFIG_HOME leaves it names.
func validateInvocation(invocation gitmanager.GitInvocation) ([]string, error) {
	refuse := func(detail string) ([]string, error) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidInvocation, detail)
	}
	if !strings.HasPrefix(invocation.Executable, "/") || strings.ContainsRune(invocation.Executable, 0) {
		return refuse("executable is not an absolute path")
	}
	if !strings.HasPrefix(invocation.Directory, "/") || strings.ContainsRune(invocation.Directory, 0) {
		return refuse("working directory is not an absolute path")
	}
	for _, arg := range invocation.Args {
		if strings.ContainsRune(arg, 0) {
			return refuse("argument contains NUL")
		}
	}
	max := gitmanager.LimitCapturedStreamBytes.Max()
	if invocation.StdoutLimit <= 0 || invocation.StdoutLimit > max || invocation.StderrLimit <= 0 || invocation.StderrLimit > max {
		return refuse("capture limits outside the captured-stream bound")
	}
	values := map[string]string{}
	for _, entry := range invocation.Environment {
		name, value, found := strings.Cut(entry, "=")
		if !found || name == "" || strings.ContainsRune(entry, 0) {
			return refuse(fmt.Sprintf("malformed environment entry %q", entry))
		}
		if _, duplicate := values[name]; duplicate {
			return refuse("duplicate environment variable " + name)
		}
		values[name] = value
	}
	var leaves []string
	for _, name := range []string{"HOME", "XDG_CONFIG_HOME"} {
		leaf, present := values[name]
		if !present || !strings.HasPrefix(leaf, "/") {
			return refuse(name + " is not the private absolute leaf")
		}
		leaves = append(leaves, leaf)
	}
	return leaves, nil
}

// requireAbsent proves each leaf is absent. Anything other than a clean
// "does not exist" fails closed.
func requireAbsent(leaves []string) error {
	for _, leaf := range leaves {
		_, err := os.Lstat(leaf)
		if err == nil {
			return fmt.Errorf("%w: %s", ErrEnvironmentLeafPresent, leaf)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s cannot be proven absent: %v", ErrEnvironmentLeafPresent, leaf, err)
		}
	}
	return nil
}

// boundedBuffer keeps at most limit bytes and counts every byte written.
// Exceeding the limit stops the process instead of retaining more output.
// exec gives each stream its own goroutine, so a buffer has one writer.
type boundedBuffer struct {
	limit      int64
	data       []byte
	observed   int64
	overflowed bool
	overflow   func()
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.observed += int64(len(p))
	if room := b.limit - int64(len(b.data)); room > 0 {
		keep := p
		if int64(len(keep)) > room {
			keep = keep[:room]
		}
		b.data = append(b.data, keep...)
	}
	if b.observed > b.limit && !b.overflowed {
		b.overflowed = true
		b.overflow()
	}
	return len(p), nil
}
