package gitmanager

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const syntheticCredential = "synthetic-credential-for-tests-only"

const (
	pagerExecutionTimeout = 10 * time.Second
	pagerCleanupTimeout   = 2 * time.Second
	pagerPollInterval     = 5 * time.Millisecond
	syntheticHolderLimit  = 5 * time.Second
	syntheticExitTimeout  = 2 * time.Second
)

var errPagerTimeout = errors.New("pager execution timed out")

type hostileGitSentinels struct {
	MarkerPath  string
	WitnessPath string
	Commands    map[string]string
	Env         []string
	Config      []string
}

func newHostileGitSentinels(t *testing.T) hostileGitSentinels {
	t.Helper()
	marker := filepath.Join(t.TempDir(), "forbidden command 'ran'.log")
	witness := filepath.Join(t.TempDir(), "invocation witness 'log'.txt")
	return newHostileGitSentinelsAt(t, marker, witness)
}

func newHostileGitSentinelsAt(t *testing.T, marker, witness string) hostileGitSentinels {
	t.Helper()
	commandDirectory := t.TempDir()
	commands := make(map[string]string)
	for _, mechanism := range []string{"askpass", "editor", "pager", "ssh", "gpg", "fsmonitor", "filter", "credential"} {
		path := filepath.Join(commandDirectory, mechanism+"-sentinel.sh")
		contents := sentinelScript(mechanism, marker, witness)
		if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
			t.Fatal(err)
		}
		commands[mechanism] = path
	}
	return hostileGitSentinels{
		MarkerPath:  marker,
		WitnessPath: witness,
		Commands:    commands,
		Env: []string{
			"GIT_ASKPASS=" + commands["askpass"],
			"GIT_EDITOR=" + commands["editor"],
			"GIT_PAGER=" + commands["pager"],
			"GIT_SSH_COMMAND=" + commands["ssh"],
		},
		Config: []string{
			"credential.helper=!" + shellSingleQuote(commands["credential"]),
			"core.fsmonitor=" + commands["fsmonitor"],
			"filter.hostile.clean=" + commands["filter"],
			"gpg.program=" + commands["gpg"],
			"http.extraHeader=Authorization: Bearer " + syntheticCredential,
		},
	}
}

func sentinelScript(mechanism, marker, witness string) string {
	response := ""
	switch mechanism {
	case "askpass", "credential":
		response = "printf '%s\\n' 'synthetic-response'\n"
	case "editor":
		response = "if [ \"$#\" -gt 0 ]; then printf '%s\\n' 'synthetic commit message' > \"$1\"; fi\n"
	case "filter":
		response = "while IFS= read -r line || [ -n \"$line\" ]; do printf '%s\\n' \"$line\"; done\n"
	case "ssh", "gpg":
		response = "exit 1\n"
	case "pager":
		// A pager that exits before reading would let Git die of SIGPIPE
		// when it writes after the exit; slow runs (-race) hit that.
		response = "cat > /dev/null\n"
	}
	return "#!/bin/sh\n" +
		"printf '%s\\n' " + shellSingleQuote(mechanism) + " >> " + shellSingleQuote(witness) + "\n" +
		"printf '%s\\n' " + shellSingleQuote(mechanism) + " >> " + shellSingleQuote(marker) + "\n" +
		response
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

type repositoryFixture struct {
	Root   string
	GitDir string
}

func newRepositoryFixture(t *testing.T, withGitDirectory bool) repositoryFixture {
	t.Helper()
	root := t.TempDir()
	fixture := repositoryFixture{Root: root}
	if withGitDirectory {
		fixture.GitDir = filepath.Join(root, ".git")
		if err := os.Mkdir(fixture.GitDir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return fixture
}

func (f repositoryFixture) writeFile(t *testing.T, name string, contents []byte) string {
	t.Helper()
	path := filepath.Join(f.Root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSharedFixturesAreSyntheticAndInert(t *testing.T) {
	sentinels := newHostileGitSentinels(t)
	if len(sentinels.Env) != 4 || len(sentinels.Config) != 5 || len(sentinels.Commands) != 8 {
		t.Fatalf("hostile Git sentinels are incomplete: env=%d config=%d commands=%d", len(sentinels.Env), len(sentinels.Config), len(sentinels.Commands))
	}
	for _, path := range []string{sentinels.MarkerPath, sentinels.WitnessPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("sentinel ran while building fixture at %q: %v", path, err)
		}
	}
	if got := configValue(t, sentinels.Config, "gpg.program"); got != sentinels.Commands["gpg"] {
		t.Fatalf("gpg.program=%q, want executable path %q", got, sentinels.Commands["gpg"])
	}

	fixture := newRepositoryFixture(t, true)
	path := fixture.writeFile(t, "nested/file.txt", []byte("fixture"))
	if fixture.GitDir != filepath.Join(fixture.Root, ".git") {
		t.Fatalf("Git directory = %q", fixture.GitDir)
	}
	if contents, err := os.ReadFile(path); err != nil || string(contents) != "fixture" {
		t.Fatalf("fixture contents = %q, %v", contents, err)
	}
}

func TestHostileGitSentinelsRecordRealGitInvocations(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skipf("Git unavailable: %v", err)
	}
	tests := []struct {
		mechanism string
		trigger   func(*testing.T, string, string, hostileGitSentinels) commandObservation
	}{
		{"editor", triggerEditor},
		{"pager", triggerPager},
		{"askpass", triggerAskpass},
		{"ssh", triggerSSH},
		{"gpg", triggerGPG},
		{"fsmonitor", triggerFSMonitor},
		{"filter", triggerCleanFilter},
	}
	for _, test := range tests {
		test := test
		t.Run(test.mechanism, func(t *testing.T) {
			repository := newIsolatedGitRepository(t, git)
			sentinels := newHostileGitSentinels(t)
			observation := test.trigger(t, git, repository, sentinels)
			if err := verifySentinelInvocation(sentinels, test.mechanism); err != nil {
				t.Fatalf("%s; Git exit error=%v output=%q", err, observation.err, observation.output)
			}
			t.Logf("%s: helper executed and append-only marker recorded (Git error=%v)", test.mechanism, observation.err)
		})
	}
}

func TestSentinelVerificationRejectsExecutionWithoutMarker(t *testing.T) {
	markerParent := t.TempDir()
	marker := filepath.Join(markerParent, "marker")
	witness := filepath.Join(t.TempDir(), "witness")
	sentinels := newHostileGitSentinelsAt(t, marker, witness)
	if err := os.Remove(markerParent); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(markerParent, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = exec.Command(sentinels.Commands["pager"]).Run()
	err := verifySentinelInvocation(sentinels, "pager")
	if err == nil || !strings.Contains(err.Error(), "executed but marker is absent") {
		t.Fatalf("verification error=%v", err)
	}
}

func TestSentinelMarkerIsAppendOnly(t *testing.T) {
	sentinels := newHostileGitSentinels(t)
	for invocation := 0; invocation < 2; invocation++ {
		if output, err := exec.Command(sentinels.Commands["pager"]).CombinedOutput(); err != nil {
			t.Fatalf("sentinel invocation %d: %v: %s", invocation, err, output)
		}
	}
	marker, err := os.ReadFile(sentinels.MarkerPath)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, line := range bytes.Split(marker, []byte{'\n'}) {
		if string(line) == "pager" {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("marker invocation count=%d, want 2; marker=%q", count, marker)
	}
}

func TestPseudoTerminalOpenAndClose(t *testing.T) {
	master, slave, err := openPseudoTerminal()
	if err != nil {
		t.Skipf("pseudo-terminal unavailable: %v", err)
	}
	if master.Fd() == ^uintptr(0) || slave.Fd() == ^uintptr(0) {
		t.Fatal("pseudo-terminal returned an invalid descriptor")
	}
	if err := slave.Close(); err != nil {
		t.Fatalf("close PTY slave: %v", err)
	}
	if err := master.Close(); err != nil {
		t.Fatalf("close PTY master: %v", err)
	}
}

func TestPagerSuccessiveInvocationsAreIndependent(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skipf("Git unavailable: %v", err)
	}
	repository := newIsolatedGitRepository(t, git)
	sentinels := newHostileGitSentinels(t)
	for invocation := 0; invocation < 2; invocation++ {
		observation := triggerPager(t, git, repository, sentinels)
		if observation.err != nil || observation.timedOut || !observation.processReaped || !observation.readerFinished {
			t.Fatalf("pager invocation %d: err=%v timeout=%t reaped=%t reader_finished=%t output=%q", invocation, observation.err, observation.timedOut, observation.processReaped, observation.readerFinished, observation.output)
		}
	}
	assertMarkerCount(t, sentinels.MarkerPath, "pager", 2)
}

func TestPagerTimeoutTerminatesBlockedProcess(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skipf("Git unavailable: %v", err)
	}
	repository := newIsolatedGitRepository(t, git)
	sentinels := newHostileGitSentinels(t)
	pidPath := filepath.Join(t.TempDir(), "blocked-pager.pid")
	naturalExitPath := filepath.Join(t.TempDir(), "blocked-pager-natural-exit")
	replacePagerSentinel(t, sentinels, syntheticPagerHolderCommand(t, pidPath, naturalExitPath, "", false, false))

	observation := triggerPagerOnceReady(t, git, repository, sentinels, pidPath)
	if !errors.Is(observation.err, errPagerTimeout) || !observation.timedOut {
		t.Fatalf("blocked pager result: err=%v timeout=%t output=%q", observation.err, observation.timedOut, observation.output)
	}
	if !observation.processReaped || !observation.readerFinished {
		t.Fatalf("blocked pager cleanup: reaped=%t reader_finished=%t", observation.processReaped, observation.readerFinished)
	}
	if err := verifySentinelInvocation(sentinels, "pager"); err != nil {
		t.Fatal(err)
	}
	assertProcessTerminatedByCleanup(t, readSyntheticPID(t, pidPath), naturalExitPath)
}

func TestPagerTimeoutClosesPTYHeldByDescendant(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skipf("Git unavailable: %v", err)
	}
	repository := newIsolatedGitRepository(t, git)
	sentinels := newHostileGitSentinels(t)
	pidPath := filepath.Join(t.TempDir(), "pty-holder.pid")
	naturalExitPath := filepath.Join(t.TempDir(), "pty-holder-natural-exit")
	launcherExitPath := filepath.Join(t.TempDir(), "pager-launcher-exit")
	replacePagerSentinel(t, sentinels, syntheticPagerHolderCommand(t, pidPath, naturalExitPath, launcherExitPath, false, true))

	observation := triggerPagerOnceReady(t, git, repository, sentinels, pidPath, launcherExitPath)
	if !errors.Is(observation.err, errPagerTimeout) || !observation.timedOut {
		t.Fatalf("PTY holder result: err=%v timeout=%t output=%q", observation.err, observation.timedOut, observation.output)
	}
	if _, err := os.Stat(launcherExitPath); err != nil {
		t.Fatalf("pager launcher did not exit before inherited PTY cleanup: %v", err)
	}
	if !observation.processReaped || !observation.readerFinished {
		t.Fatalf("PTY holder cleanup: reaped=%t reader_finished=%t", observation.processReaped, observation.readerFinished)
	}
	if err := verifySentinelInvocation(sentinels, "pager"); err != nil {
		t.Fatal(err)
	}
	assertProcessTerminatedByCleanup(t, readSyntheticPID(t, pidPath), naturalExitPath)
}

func TestPagerTimeoutWithDetachedPTYHolderIsBounded(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skipf("Git unavailable: %v", err)
	}
	repository := newIsolatedGitRepository(t, git)
	sentinels := newHostileGitSentinels(t)
	pidPath := filepath.Join(t.TempDir(), "detached-pty-holder.pid")
	naturalExitPath := filepath.Join(t.TempDir(), "detached-pty-holder-natural-exit")
	launcherExitPath := filepath.Join(t.TempDir(), "detached-pager-launcher-exit")
	replacePagerSentinel(t, sentinels, syntheticPagerHolderCommand(t, pidPath, naturalExitPath, launcherExitPath, true, true))

	observation := triggerPagerOnceReady(t, git, repository, sentinels, pidPath, launcherExitPath)
	if !errors.Is(observation.err, errPagerTimeout) || !observation.timedOut {
		t.Fatalf("detached PTY holder result: err=%v timeout=%t output=%q", observation.err, observation.timedOut, observation.output)
	}
	if elapsed, limit := time.Since(observation.timedOutAt), pagerCleanupTimeout+500*time.Millisecond; elapsed >= limit {
		t.Fatalf("detached PTY holder exceeded bounded cleanup: elapsed=%s limit=%s", elapsed, limit)
	}
	if !observation.processReaped || !observation.readerFinished {
		t.Fatalf("detached PTY holder cleanup: reaped=%t reader_finished=%t", observation.processReaped, observation.readerFinished)
	}
	if err := verifySentinelInvocation(sentinels, "pager"); err != nil {
		t.Fatal(err)
	}
	pid := readSyntheticPID(t, pidPath)
	if !pseudoTerminalProcessExists(pid) {
		t.Fatalf("detached synthetic process %d did not survive its original process group as expected", pid)
	}
	terminateSyntheticProcess(t, pid)
	assertProcessTerminatedByCleanup(t, pid, naturalExitPath)
}

func TestPagerReportsGroupTerminationFailureWithoutBlocking(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skipf("Git unavailable: %v", err)
	}
	repository := newIsolatedGitRepository(t, git)
	sentinels := newHostileGitSentinels(t)
	pidPath := filepath.Join(t.TempDir(), "bounded-pty-holder.pid")
	naturalExitPath := filepath.Join(t.TempDir(), "bounded-pty-holder-natural-exit")
	replacePagerSentinel(t, sentinels, syntheticPagerHolderCommand(t, pidPath, naturalExitPath, "", false, false))
	simulatedErr := errors.New("simulated process-group termination failure")
	var signaledGroupID int

	observation := triggerPagerWithOptions(t, git, repository, sentinels, pagerRunOptions{
		executionTimeout: 500 * time.Millisecond,
		cleanupTimeout:   pagerCleanupTimeout,
		terminateGroup: func(groupID int) error {
			signaledGroupID = groupID
			return simulatedErr
		},
		ready: filesExist(pidPath),
	})
	if !errors.Is(observation.err, errPagerTimeout) || !errors.Is(observation.err, simulatedErr) {
		t.Fatalf("termination failure result: %v", observation.err)
	}
	if elapsed, limit := time.Since(observation.timedOutAt), pagerCleanupTimeout+500*time.Millisecond; elapsed >= limit {
		t.Fatalf("termination failure exceeded bounded cleanup: elapsed=%s limit=%s", elapsed, limit)
	}
	if !observation.processReaped || !observation.readerFinished {
		t.Fatalf("termination failure cleanup: reaped=%t reader_finished=%t", observation.processReaped, observation.readerFinished)
	}
	if signaledGroupID <= 1 || signaledGroupID != observation.processGroupID {
		t.Fatalf("signaled process group=%d, recorded group=%d", signaledGroupID, observation.processGroupID)
	}
	pid := readSyntheticPID(t, pidPath)
	if !pseudoTerminalProcessExists(pid) {
		t.Fatalf("synthetic process %d exited despite simulated group-termination failure", pid)
	}
	terminateSyntheticProcess(t, pid)
	assertProcessTerminatedByCleanup(t, pid, naturalExitPath)
}

func TestPagerCleanupVerificationRejectsDisabledGroupTermination(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skipf("Git unavailable: %v", err)
	}
	repository := newIsolatedGitRepository(t, git)
	sentinels := newHostileGitSentinels(t)
	pidPath := filepath.Join(t.TempDir(), "disabled-cleanup-holder.pid")
	naturalExitPath := filepath.Join(t.TempDir(), "disabled-cleanup-natural-exit")
	launcherExitPath := filepath.Join(t.TempDir(), "disabled-cleanup-launcher-exit")
	replacePagerSentinel(t, sentinels, syntheticPagerHolderCommand(t, pidPath, naturalExitPath, launcherExitPath, false, true))

	observation := triggerPagerWithOptions(t, git, repository, sentinels, pagerRunOptions{
		executionTimeout: 500 * time.Millisecond,
		cleanupTimeout:   pagerCleanupTimeout,
		terminateGroup:   func(int) error { return nil },
		ready:            filesExist(pidPath, launcherExitPath),
	})
	if !errors.Is(observation.err, errPagerTimeout) {
		t.Fatalf("disabled cleanup result: %v", observation.err)
	}
	pid := readSyntheticPID(t, pidPath)
	if err := verifySyntheticProcessTerminated(pid, naturalExitPath, 250*time.Millisecond); err == nil {
		t.Fatal("cleanup verification accepted a deliberately disabled process-group termination")
	}
	terminateSyntheticProcess(t, pid)
	assertProcessTerminatedByCleanup(t, pid, naturalExitPath)
}

func TestProcessGroupTerminationRejectsUnsafeIdentifiers(t *testing.T) {
	invocations := 0
	terminator := func(int) error {
		invocations++
		return nil
	}
	for _, groupID := range []int{-1, 0, 1} {
		if err := terminatePseudoTerminalProcessGroupWith(groupID, terminator); err == nil {
			t.Errorf("unsafe process group %d was accepted", groupID)
		}
	}
	if invocations != 0 {
		t.Fatalf("injected terminator called %d times for unsafe identifiers", invocations)
	}

	const validGroupID = 42
	observedGroupID := 0
	if err := terminatePseudoTerminalProcessGroupWith(validGroupID, func(groupID int) error {
		observedGroupID = groupID
		return nil
	}); err != nil {
		t.Fatalf("valid process group rejected: %v", err)
	}
	if observedGroupID != validGroupID {
		t.Fatalf("injected terminator received group %d, want %d", observedGroupID, validGroupID)
	}
}

func TestPseudoTerminalProcessGroupCaptureOutcomes(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("POSIX process-group capture is not supported on this platform")
	}
	const pid = 4242
	process := &os.Process{Pid: pid}
	attributes := pseudoTerminalProcessAttributes()
	unreaped := func(*os.Process) error { return nil }

	t.Run("normal", func(t *testing.T) {
		groupID, err := pseudoTerminalProcessGroupIDWithLookup(process, attributes, func(gotPID int) (int, error) {
			if gotPID != pid {
				t.Fatalf("lookup PID=%d, want %d", gotPID, pid)
			}
			return pid, nil
		}, unreaped)
		if err != nil || groupID != pid {
			t.Fatalf("group=%d err=%v", groupID, err)
		}
	})

	t.Run("exited unreaped leader", func(t *testing.T) {
		groupID, err := pseudoTerminalProcessGroupIDWithLookup(process, attributes, func(int) (int, error) {
			return 0, syscall.ESRCH
		}, unreaped)
		if err != nil || groupID != pid {
			t.Fatalf("ESRCH fallback group=%d err=%v", groupID, err)
		}
	})

	t.Run("unexpected lookup error", func(t *testing.T) {
		_, err := pseudoTerminalProcessGroupIDWithLookup(process, attributes, func(int) (int, error) {
			return 0, syscall.EPERM
		}, unreaped)
		if !errors.Is(err, syscall.EPERM) {
			t.Fatalf("unexpected lookup error=%v", err)
		}
	})

	t.Run("mismatched group", func(t *testing.T) {
		if _, err := pseudoTerminalProcessGroupIDWithLookup(process, attributes, func(int) (int, error) {
			return pid + 1, nil
		}, unreaped); err == nil {
			t.Fatal("mismatched process group was accepted")
		}
	})

	t.Run("reaped lifecycle", func(t *testing.T) {
		lookupCalled := false
		reapedErr := errors.New("injected reaped process")
		_, err := pseudoTerminalProcessGroupIDWithLookup(process, attributes, func(int) (int, error) {
			lookupCalled = true
			return 0, syscall.ESRCH
		}, func(*os.Process) error { return reapedErr })
		if !errors.Is(err, reapedErr) {
			t.Fatalf("reaped lifecycle error=%v", err)
		}
		if lookupCalled {
			t.Fatal("PGID lookup called after lifecycle rejected a reaped process")
		}
	})
}

func TestPseudoTerminalProcessGroupCaptureRejectsInvalidInputs(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("POSIX process-group capture is not supported on this platform")
	}
	validProcess := &os.Process{Pid: 4242}
	validAttributes := pseudoTerminalProcessAttributes()
	tests := []struct {
		name       string
		process    *os.Process
		attributes *syscall.SysProcAttr
	}{
		{name: "nil process", process: nil, attributes: validAttributes},
		{name: "negative PID", process: &os.Process{Pid: -2}, attributes: validAttributes},
		{name: "released PID", process: &os.Process{Pid: -1}, attributes: validAttributes},
		{name: "zero PID", process: &os.Process{Pid: 0}, attributes: validAttributes},
		{name: "PID one", process: &os.Process{Pid: 1}, attributes: validAttributes},
		{name: "nil attributes", process: validProcess, attributes: nil},
		{name: "Setpgid false", process: validProcess, attributes: &syscall.SysProcAttr{}},
		{name: "nonzero Pgid", process: validProcess, attributes: pseudoTerminalNonzeroProcessGroupAttributesForTest()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			probeCalls := 0
			if _, err := pseudoTerminalProcessGroupIDWithLookup(test.process, test.attributes, func(int) (int, error) {
				probeCalls++
				return 0, syscall.ESRCH
			}, func(*os.Process) error {
				probeCalls++
				return nil
			}); err == nil {
				t.Fatal("invalid process-group input was accepted")
			}
			if probeCalls != 0 {
				t.Fatalf("identity probes called %d times for invalid input", probeCalls)
			}
		})
	}

	for _, missing := range []struct {
		name          string
		lookup        func(int) (int, error)
		checkUnreaped func(*os.Process) error
	}{
		{name: "nil lookup", checkUnreaped: func(*os.Process) error { return nil }},
		{name: "nil lifecycle check", lookup: func(int) (int, error) { return 4242, nil }},
	} {
		t.Run(missing.name, func(t *testing.T) {
			if _, err := pseudoTerminalProcessGroupIDWithLookup(validProcess, validAttributes, missing.lookup, missing.checkUnreaped); err == nil {
				t.Fatal("missing identity probe was accepted")
			}
		})
	}
}

func TestPseudoTerminalProcessGroupCaptureLifecycle(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("POSIX process-group capture is not supported on this platform")
	}

	t.Run("live unreaped", func(t *testing.T) {
		command := syntheticProcessLifecycleCommand(t, true)
		stdin, err := command.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		attributes := pseudoTerminalProcessAttributes()
		command.SysProcAttr = attributes
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		groupID, captureErr := pseudoTerminalProcessGroupID(command.Process, attributes)
		_ = stdin.Close()
		waitErr := command.Wait()
		if captureErr != nil || groupID <= 1 {
			t.Fatalf("live process group=%d err=%v", groupID, captureErr)
		}
		if waitErr != nil {
			t.Fatalf("wait live fixture: %v", waitErr)
		}
	})

	t.Run("exited unreaped", func(t *testing.T) {
		command := syntheticProcessLifecycleCommand(t, false)
		output, err := command.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		attributes := pseudoTerminalProcessAttributes()
		command.SysProcAttr = attributes
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, output); err != nil {
			t.Fatal(err)
		}
		groupID, captureErr := pseudoTerminalProcessGroupIDWithLookup(command.Process, attributes, func(int) (int, error) {
			return 0, syscall.ESRCH
		}, pseudoTerminalProcessIsUnreaped)
		waitErr := command.Wait()
		if captureErr != nil || groupID <= 1 {
			t.Fatalf("exited unreaped process group=%d err=%v", groupID, captureErr)
		}
		if waitErr != nil {
			t.Fatalf("wait exited fixture: %v", waitErr)
		}
	})

	t.Run("reaped by Wait", func(t *testing.T) {
		command := syntheticProcessLifecycleCommand(t, false)
		attributes := pseudoTerminalProcessAttributes()
		command.SysProcAttr = attributes
		if err := command.Run(); err != nil {
			t.Fatal(err)
		}
		if _, err := pseudoTerminalProcessGroupIDWithLookup(command.Process, attributes, func(int) (int, error) {
			return 0, syscall.ESRCH
		}, pseudoTerminalProcessIsUnreaped); !errors.Is(err, os.ErrProcessDone) {
			t.Fatalf("reaped process error=%v, want os.ErrProcessDone", err)
		}
	})

	t.Run("released", func(t *testing.T) {
		process := &os.Process{Pid: 4242}
		if err := process.Release(); err != nil {
			t.Fatal(err)
		}
		probeCalls := 0
		if _, err := pseudoTerminalProcessGroupIDWithLookup(process, pseudoTerminalProcessAttributes(), func(int) (int, error) {
			probeCalls++
			return 0, syscall.ESRCH
		}, func(*os.Process) error {
			probeCalls++
			return nil
		}); err == nil {
			t.Fatal("released process was accepted")
		}
		if probeCalls != 0 {
			t.Fatalf("identity probes called %d times for released process", probeCalls)
		}
	})
}

func syntheticProcessLifecycleCommand(t *testing.T, waitForInput bool) *exec.Cmd {
	t.Helper()
	mode := "exit"
	if waitForInput {
		mode = "wait-stdin"
	}
	command := exec.Command(os.Args[0], "-test.run=^TestSyntheticProcessLifecycleFixture$")
	command.Env = append(os.Environ(), "AI_DEV_SYNTHETIC_PROCESS_LIFECYCLE="+mode)
	return command
}

func TestSyntheticProcessLifecycleFixture(t *testing.T) {
	if os.Getenv("AI_DEV_SYNTHETIC_PROCESS_LIFECYCLE") == "wait-stdin" {
		_, _ = io.Copy(io.Discard, os.Stdin)
	}
}

func TestSyntheticPagerHolderProcess(t *testing.T) {
	if os.Getenv("AI_DEV_SYNTHETIC_PTY_HOLDER") != "1" {
		return
	}
	if os.Getenv("AI_DEV_SYNTHETIC_PTY_DETACH") == "1" {
		if err := detachPseudoTerminalSession(); err != nil {
			t.Fatalf("detach synthetic PTY holder: %v", err)
		}
	}
	pidPath := os.Getenv("AI_DEV_SYNTHETIC_PTY_PID")
	if pidPath == "" {
		t.Fatal("missing synthetic PTY holder PID path")
	}
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	timer := time.NewTimer(syntheticHolderLimit)
	defer timer.Stop()
	<-timer.C
	naturalExitPath := os.Getenv("AI_DEV_SYNTHETIC_PTY_NATURAL_EXIT")
	if naturalExitPath == "" {
		t.Fatal("missing synthetic PTY holder natural-exit path")
	}
	if err := os.WriteFile(naturalExitPath, []byte("natural\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

type commandObservation struct {
	output         string
	err            error
	timedOut       bool
	timedOutAt     time.Time
	processReaped  bool
	readerFinished bool
	processGroupID int
}

func runIsolatedGit(t *testing.T, git, repository string, sentinels hostileGitSentinels, stdin string, extraEnv []string, args ...string) commandObservation {
	t.Helper()
	command := exec.Command(git, args...)
	command.Dir = repository
	command.Env = append(isolatedGitEnvironment(t), extraEnv...)
	command.Stdin = strings.NewReader(stdin)
	output, err := command.CombinedOutput()
	return commandObservation{output: string(output), err: err}
}

func isolatedGitEnvironment(t *testing.T) []string {
	t.Helper()
	home := t.TempDir()
	xdg := t.TempDir()
	return []string{
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + xdg,
		"PATH=" + os.Getenv("PATH"),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=never",
		"LC_ALL=C",
	}
}

func newIsolatedGitRepository(t *testing.T, git string) string {
	t.Helper()
	repository := t.TempDir()
	observation := runIsolatedGit(t, git, repository, hostileGitSentinels{}, "", nil, "init", "--template=", "--quiet", "--initial-branch=main", "--", repository)
	if observation.err != nil {
		t.Fatalf("initialize isolated Git fixture: %v: %s", observation.err, observation.output)
	}
	return repository
}

func gitIdentityEnvironment() []string {
	return []string{
		"GIT_AUTHOR_NAME=Synthetic Author",
		"GIT_AUTHOR_EMAIL=author@example.invalid",
		"GIT_COMMITTER_NAME=Synthetic Committer",
		"GIT_COMMITTER_EMAIL=committer@example.invalid",
	}
}

func triggerEditor(t *testing.T, git, repository string, sentinels hostileGitSentinels) commandObservation {
	return runIsolatedGit(t, git, repository, sentinels, "", append(gitIdentityEnvironment(), "GIT_EDITOR="+sentinels.Commands["editor"]), "commit", "--allow-empty")
}

func triggerPager(t *testing.T, git, repository string, sentinels hostileGitSentinels) commandObservation {
	return triggerPagerWithTimeout(t, git, repository, sentinels, pagerExecutionTimeout)
}

func triggerPagerWithTimeout(t *testing.T, git, repository string, sentinels hostileGitSentinels, timeout time.Duration) commandObservation {
	return triggerPagerWithOptions(t, git, repository, sentinels, pagerRunOptions{
		executionTimeout: timeout,
		cleanupTimeout:   pagerCleanupTimeout,
		terminateGroup:   terminatePseudoTerminalProcessGroup,
	})
}

// triggerPagerOnceReady runs the pager with the 500 ms synthetic timeout,
// counted from the moment every readiness file exists.
func triggerPagerOnceReady(t *testing.T, git, repository string, sentinels hostileGitSentinels, readinessFiles ...string) commandObservation {
	return triggerPagerWithOptions(t, git, repository, sentinels, pagerRunOptions{
		executionTimeout: 500 * time.Millisecond,
		cleanupTimeout:   pagerCleanupTimeout,
		terminateGroup:   terminatePseudoTerminalProcessGroup,
		ready:            filesExist(readinessFiles...),
	})
}

func filesExist(paths ...string) func() bool {
	return func() bool {
		for _, path := range paths {
			if _, err := os.Stat(path); err != nil {
				return false
			}
		}
		return true
	}
}

type pagerRunOptions struct {
	executionTimeout time.Duration
	cleanupTimeout   time.Duration
	terminateGroup   func(int) error
	// ready, when set, delays the execution timeout until the synthetic
	// fixture reaches the state the test exercises, so a slow start under
	// load cannot be killed before it gets there. Readiness is bounded by
	// pagerExecutionTimeout.
	ready func() bool
}

func triggerPagerWithOptions(t *testing.T, git, repository string, sentinels hostileGitSentinels, options pagerRunOptions) commandObservation {
	t.Helper()
	seed := runIsolatedGit(t, git, repository, sentinels, "", gitIdentityEnvironment(), "commit", "--allow-empty", "-m", "seed")
	if seed.err != nil {
		t.Fatalf("seed pager fixture: %v: %s", seed.err, seed.output)
	}
	master, slave, err := openPseudoTerminal()
	if err != nil {
		t.Skipf("pseudo-terminal unavailable: %v", err)
	}
	if err := setPseudoTerminalNonblocking(master); err != nil {
		_ = slave.Close()
		_ = master.Close()
		return commandObservation{err: fmt.Errorf("set PTY master nonblocking: %w", err), readerFinished: true}
	}
	command := exec.Command(git, "--paginate", "log", "-1", "--oneline")
	command.Dir = repository
	command.Env = append(isolatedGitEnvironment(t), "GIT_PAGER="+sentinels.Commands["pager"])
	command.Stdout = slave
	command.Stderr = slave
	processAttributes := pseudoTerminalProcessAttributes()
	command.SysProcAttr = processAttributes
	if err := command.Start(); err != nil {
		_ = slave.Close()
		_ = master.Close()
		return commandObservation{err: err}
	}
	processGroupID, err := pseudoTerminalProcessGroupID(command.Process, processAttributes)
	if err != nil {
		_ = command.Process.Kill()
		_, _ = command.Process.Wait()
		_ = slave.Close()
		_ = master.Close()
		return commandObservation{err: err, readerFinished: true}
	}
	_ = slave.Close()
	ready := options.ready == nil
	timer := time.NewTimer(options.executionTimeout)
	if !ready {
		timer.Reset(pagerExecutionTimeout)
	}
	defer timer.Stop()
	poll := time.NewTicker(pagerPollInterval)
	defer poll.Stop()
	var output bytes.Buffer
	var waitErr error
	waitFinished := false
	readFinished := false
	var readErr error
	for {
		if !readFinished {
			readFinished, readErr = drainPseudoTerminal(master, &output)
		}
		// Reap only after the PTY is drained. If a descendant still owns the
		// descriptor, the leader remains unreaped so its PID/PGID cannot be
		// reused before timeout cleanup signals the original group.
		if readFinished && !waitFinished {
			waitFinished, waitErr = pollPseudoTerminalProcess(command.Process)
		}
		if waitFinished && readFinished {
			_ = master.Close()
			return commandObservation{
				output:         output.String(),
				err:            errors.Join(waitErr, readErr),
				processReaped:  true,
				readerFinished: true,
				processGroupID: processGroupID,
			}
		}
		if !ready && options.ready() {
			ready = true
			timer.Reset(options.executionTimeout)
		}
		select {
		case <-poll.C:
		case <-timer.C:
			timedOutAt := time.Now()
			// Signal while the leader is deliberately still unreaped whenever
			// the reader is open. This keeps the process-group identity reserved
			// and prevents signaling through a released or reused PID.
			killErr := options.terminateGroup(processGroupID)
			if !waitFinished {
				directKillErr := command.Process.Kill()
				if directKillErr != nil && !errors.Is(directKillErr, os.ErrProcessDone) {
					killErr = errors.Join(killErr, fmt.Errorf("terminate process leader: %w", directKillErr))
				}
			}

			cleanupTimer := time.NewTimer(options.cleanupTimeout)
			for !waitFinished {
				select {
				case <-poll.C:
					waitFinished, waitErr = pollPseudoTerminalProcess(command.Process)
				case <-cleanupTimer.C:
					_ = master.Close()
					timeoutErr := fmt.Errorf("%w after %s; process cleanup exceeded %s", errPagerTimeout, options.executionTimeout, options.cleanupTimeout)
					return commandObservation{
						output:         output.String(),
						err:            errors.Join(timeoutErr, killErr, waitErr, readErr),
						timedOut:       true,
						timedOutAt:     timedOutAt,
						processReaped:  false,
						readerFinished: true,
						processGroupID: processGroupID,
					}
				}
			}
			if !cleanupTimer.Stop() {
				select {
				case <-cleanupTimer.C:
				default:
				}
			}
			_, finalReadErr := drainPseudoTerminal(master, &output)
			_ = master.Close()
			timeoutErr := fmt.Errorf("%w after %s", errPagerTimeout, options.executionTimeout)
			return commandObservation{
				output:         output.String(),
				err:            errors.Join(timeoutErr, killErr, waitErr, readErr, finalReadErr),
				timedOut:       true,
				timedOutAt:     timedOutAt,
				processReaped:  waitFinished,
				readerFinished: true,
				processGroupID: processGroupID,
			}
		}
	}
}

func drainPseudoTerminal(master *os.File, output *bytes.Buffer) (bool, error) {
	ready, err := pollPseudoTerminalReadable(master)
	if err != nil || !ready {
		return false, err
	}
	var buffer [4096]byte
	n, err := readPseudoTerminalNonblocking(master, buffer[:])
	if n > 0 {
		_, _ = output.Write(buffer[:n])
	}
	switch {
	case err == nil && n > 0:
		return false, nil
	case err == nil:
		return true, nil
	case pseudoTerminalReadWouldBlock(err):
		return false, nil
	case errors.Is(err, io.EOF), errors.Is(err, os.ErrClosed), pseudoTerminalReadComplete(err):
		return true, nil
	default:
		return true, err
	}
}

func syntheticPagerHolderCommand(t *testing.T, pidPath, naturalExitPath, launcherExitPath string, detach, background bool) string {
	t.Helper()
	executable, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	detachValue := "0"
	if detach {
		detachValue = "1"
	}
	command := "AI_DEV_SYNTHETIC_PTY_HOLDER=1 " +
		"AI_DEV_SYNTHETIC_PTY_DETACH=" + detachValue + " " +
		"AI_DEV_SYNTHETIC_PTY_PID=" + shellSingleQuote(pidPath) + " " +
		"AI_DEV_SYNTHETIC_PTY_NATURAL_EXIT=" + shellSingleQuote(naturalExitPath) + " " +
		shellSingleQuote(executable) + " -test.run=" + shellSingleQuote("^TestSyntheticPagerHolderProcess$")
	if background {
		return command + " &\nprintf '%s\\n' 'exited' > " + shellSingleQuote(launcherExitPath) + "\nexit 0\n"
	}
	return command + "\n"
}

func replacePagerSentinel(t *testing.T, sentinels hostileGitSentinels, behavior string) {
	t.Helper()
	contents := sentinelScript("pager", sentinels.MarkerPath, sentinels.WitnessPath) + behavior
	if err := os.WriteFile(sentinels.Commands["pager"], []byte(contents), 0o700); err != nil {
		t.Fatal(err)
	}
}

func readSyntheticPID(t *testing.T, filename string) int {
	t.Helper()
	deadline := time.NewTimer(pagerCleanupTimeout)
	defer deadline.Stop()
	poll := time.NewTicker(pagerPollInterval)
	defer poll.Stop()
	for {
		contents, err := os.ReadFile(filename)
		if err == nil {
			pid, conversionErr := strconv.Atoi(strings.TrimSpace(string(contents)))
			if conversionErr != nil || pid <= 0 {
				t.Fatalf("invalid synthetic process ID %q: %v", contents, conversionErr)
			}
			return pid
		}
		if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		select {
		case <-poll.C:
		case <-deadline.C:
			t.Fatalf("synthetic process did not publish its PID at %q", filename)
		}
	}
}

func verifySyntheticProcessTerminated(pid int, naturalExitPath string, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	poll := time.NewTicker(10 * time.Millisecond)
	defer poll.Stop()
	for pseudoTerminalProcessExists(pid) {
		select {
		case <-poll.C:
		case <-deadline.C:
			return fmt.Errorf("synthetic pager process %d remains alive after cleanup", pid)
		}
	}
	if _, err := os.Stat(naturalExitPath); err == nil {
		return fmt.Errorf("synthetic pager process %d reached its natural self-cleanup limit", pid)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect synthetic natural-exit marker: %w", err)
	}
	return nil
}

func assertProcessTerminatedByCleanup(t *testing.T, pid int, naturalExitPath string) {
	t.Helper()
	if err := verifySyntheticProcessTerminated(pid, naturalExitPath, syntheticExitTimeout); err != nil {
		terminateSyntheticProcess(t, pid)
		t.Fatal(err)
	}
}

func terminateSyntheticProcess(t *testing.T, pid int) {
	t.Helper()
	if pid <= 1 {
		t.Fatalf("refusing to terminate unsafe synthetic PID %d", pid)
	}
	if !pseudoTerminalProcessExists(pid) {
		return
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		t.Fatalf("find synthetic process %d: %v", pid, err)
	}
	if err := process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("terminate synthetic process %d: %v", pid, err)
	}
}

func assertMarkerCount(t *testing.T, filename, mechanism string, want int) {
	t.Helper()
	contents, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, line := range bytes.Split(contents, []byte{'\n'}) {
		if string(line) == mechanism {
			count++
		}
	}
	if count != want {
		t.Fatalf("%s invocation count=%d, want %d; marker=%q", mechanism, count, want, contents)
	}
}

func triggerAskpass(t *testing.T, git, repository string, sentinels hostileGitSentinels) commandObservation {
	return runIsolatedGit(t, git, repository, sentinels, "protocol=https\nhost=synthetic.invalid\n\n", []string{"GIT_ASKPASS=" + sentinels.Commands["askpass"]}, "-c", "credential.helper=", "credential", "fill")
}

func triggerSSH(t *testing.T, git, repository string, sentinels hostileGitSentinels) commandObservation {
	return runIsolatedGit(t, git, repository, sentinels, "", []string{"GIT_SSH_COMMAND=" + sentinels.Commands["ssh"]}, "ls-remote", "ssh://synthetic.invalid/repository")
}

func triggerGPG(t *testing.T, git, repository string, sentinels hostileGitSentinels) commandObservation {
	return runIsolatedGit(t, git, repository, sentinels, "", gitIdentityEnvironment(), "-c", "commit.gpgSign=true", "-c", "gpg.program="+sentinels.Commands["gpg"], "commit", "--allow-empty", "-m", "signed")
}

func triggerFSMonitor(t *testing.T, git, repository string, sentinels hostileGitSentinels) commandObservation {
	return runIsolatedGit(t, git, repository, sentinels, "", nil, "-c", "core.fsmonitor="+sentinels.Commands["fsmonitor"], "status", "--porcelain")
}

func triggerCleanFilter(t *testing.T, git, repository string, sentinels hostileGitSentinels) commandObservation {
	if err := os.WriteFile(filepath.Join(repository, ".gitattributes"), []byte("payload filter=hostile\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "payload"), []byte("synthetic payload\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return runIsolatedGit(t, git, repository, sentinels, "", nil, "-c", "filter.hostile.clean="+sentinels.Commands["filter"], "hash-object", "--path=payload", "payload")
}

func verifySentinelInvocation(sentinels hostileGitSentinels, mechanism string) error {
	witness, witnessErr := os.ReadFile(sentinels.WitnessPath)
	marker, markerErr := os.ReadFile(sentinels.MarkerPath)
	witnessed := witnessErr == nil && linePresent(witness, mechanism)
	marked := markerErr == nil && linePresent(marker, mechanism)
	switch {
	case !witnessed:
		return fmt.Errorf("%s helper was not executed by a valid trigger", mechanism)
	case !marked:
		return fmt.Errorf("%s helper executed but marker is absent", mechanism)
	default:
		return nil
	}
}

func linePresent(contents []byte, want string) bool {
	for _, line := range bytes.Split(contents, []byte{'\n'}) {
		if string(line) == want {
			return true
		}
	}
	return false
}

func configValue(t *testing.T, values []string, key string) string {
	t.Helper()
	prefix := key + "="
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimPrefix(value, prefix)
		}
	}
	t.Fatalf("missing configuration key %s", key)
	return ""
}
