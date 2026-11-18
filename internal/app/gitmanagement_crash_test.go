//go:build (darwin && !ios) || (linux && !android)

package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"

	"ai-dev/internal/gitmanager"
)

// The crash child is this test binary re-executed with these variables.
const (
	crashBaseEnv  = "AI_DEV_CRASH_BASE"
	crashArgsEnv  = "AI_DEV_CRASH_ARGS"
	crashPointEnv = "AI_DEV_CRASH_POINT"
	crashTraceEnv = "AI_DEV_CRASH_TRACE"
)

// crashPoints counts every durable step of one process and kills the
// process with SIGKILL at the targeted one, before or after its effect. In
// record mode it appends every step label to a trace file instead.
type crashPoints struct {
	mu     sync.Mutex
	counts map[string]int
	target string
	trace  *os.File
}

func (c *crashPoints) step(name string, run func() error) error {
	c.mu.Lock()
	if c.counts == nil {
		c.counts = map[string]int{}
	}
	c.counts[name]++
	label := fmt.Sprintf("%s#%d", name, c.counts[name])
	if c.trace != nil {
		fmt.Fprintln(c.trace, label)
	}
	c.mu.Unlock()
	if c.target == label+":before" {
		crashNow()
	}
	err := run()
	if c.target == label+":after" {
		crashNow()
	}
	return err
}

func crashNow() {
	syscall.Kill(os.Getpid(), syscall.SIGKILL)
	select {}
}

// crashingStore wraps every durable store step of the real store.
type crashingStore struct {
	gitmanager.GitStore
	c *crashPoints
}

func (s crashingStore) CreateAttempt(ctx context.Context, attempt gitmanager.GitApplyAttempt) error {
	return s.c.step("CreateAttempt", func() error { return s.GitStore.CreateAttempt(ctx, attempt) })
}

func (s crashingStore) RecordAttemptResult(ctx context.Context, id gitmanager.ApplyID, data []byte) (digest gitmanager.ContentDigest, err error) {
	err = s.c.step("RecordAttemptResult", func() (err error) { digest, err = s.GitStore.RecordAttemptResult(ctx, id, data); return err })
	return digest, err
}

func (s crashingStore) IssueAuthorization(ctx context.Context, authorization gitmanager.GitAuthorization) error {
	return s.c.step("IssueAuthorization", func() error { return s.GitStore.IssueAuthorization(ctx, authorization) })
}

func (s crashingStore) TransitionAuthorization(ctx context.Context, req gitmanager.AuthorizationTransitionRequest) (out gitmanager.GitAuthorization, err error) {
	err = s.c.step("TransitionAuthorization", func() (err error) { out, err = s.GitStore.TransitionAuthorization(ctx, req); return err })
	return out, err
}

func (s crashingStore) AppendAuditEvent(ctx context.Context, event gitmanager.AuditEvent) (digest gitmanager.ContentDigest, err error) {
	err = s.c.step("AppendAuditEvent:"+string(event.Kind), func() (err error) { digest, err = s.GitStore.AppendAuditEvent(ctx, event); return err })
	return digest, err
}

func (s crashingStore) AcquireApplyLock(ctx context.Context, project gitmanager.PhysicalProjectIdentity, id gitmanager.ApplyID) (lock gitmanager.ApplyLock, err error) {
	err = s.c.step("AcquireApplyLock", func() (err error) { lock, err = s.GitStore.AcquireApplyLock(ctx, project, id); return err })
	if err != nil {
		return lock, err
	}
	return crashingLock{lock, s.c}, nil
}

type crashingLock struct {
	gitmanager.ApplyLock
	c *crashPoints
}

func (l crashingLock) Release(ctx context.Context) error {
	return l.c.step("ApplyLock.Release", func() error { return l.ApplyLock.Release(ctx) })
}

// crashingRunner wraps every Git invocation the dispatcher runs.
type crashingRunner struct {
	gitmanager.GitRunner
	c *crashPoints
}

func (r crashingRunner) Run(ctx context.Context, invocation gitmanager.GitInvocation) (result gitmanager.GitRunResult, err error) {
	err = r.c.step("git-"+gitSubcommand(invocation.Args), func() (err error) { result, err = r.GitRunner.Run(ctx, invocation); return err })
	return result, err
}

// countingRunner counts the Git invocations a later attempt runs.
type countingRunner struct {
	gitmanager.GitRunner
	mu   sync.Mutex
	runs []string
}

func (r *countingRunner) Run(ctx context.Context, invocation gitmanager.GitInvocation) (gitmanager.GitRunResult, error) {
	r.mu.Lock()
	r.runs = append(r.runs, gitSubcommand(invocation.Args))
	r.mu.Unlock()
	return r.GitRunner.Run(ctx, invocation)
}

func gitSubcommand(args []string) string {
	i := 0
	for i+1 < len(args) && args[i] == "-c" {
		i += 2
	}
	if i < len(args) {
		return args[i]
	}
	return ""
}

// TestRealEngineCrashChild is the crash child; it is skipped unless the
// parent re-executes the test binary for it.
func TestRealEngineCrashChild(t *testing.T) {
	base := os.Getenv(crashBaseEnv)
	if base == "" {
		t.Skip("runs only as the child of TestRealEngineCrashRecovery")
	}
	f := realEngine(t, newGitAppFixtureAt(t, base))
	c := &crashPoints{target: os.Getenv(crashPointEnv)}
	if path := os.Getenv(crashTraceEnv); path != "" {
		trace, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		defer trace.Close()
		c.trace = trace
	}
	open := f.deps.OpenStore
	f.deps.OpenStore = func(root gitmanager.ObservedPath) (gitmanager.GitStore, func(), error) {
		store, closer, err := open(root)
		if err != nil {
			return nil, nil, err
		}
		return crashingStore{store, c}, closer, nil
	}
	f.deps.Runner = crashingRunner{f.deps.Runner, c}
	var args []string
	if err := json.Unmarshal([]byte(os.Getenv(crashArgsEnv)), &args); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := f.run(args...)
	fmt.Printf("child exit %d\n%s%s", code, stdout, stderr)
}

// crashCase is one isolated project with its plan, ready to apply.
type crashCase struct {
	f       *gitAppFixture
	plan    string
	digest  string
	issue   []string
	planned []string
}

const crashAuthorizationID = "123e4567-e89b-42d3-a456-426614174123"

func newCrashCase(t *testing.T) crashCase {
	f := newRealEngineFixture(t)
	message := filepath.Join(f.base, "message.txt")
	if err := os.WriteFile(message, []byte("chore: initial commit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := crashCase{f: f, plan: filepath.Join(f.base, "plan.json")}
	c.planned = []string{"plan", "--project-dir", f.project, "--strategy", "basic", "--initialize-repository",
		"--initial-branch", "main", "--initial-commit", "--initial-commit-message-file", message, "--include", "README.md",
		"--author-name", "Ada", "--author-email", "ada@example.org", "--committer-name", "Ada", "--committer-email", "ada@example.org",
		"--author-date", "2026-11-18T10:00:00Z", "--committer-date", "2026-11-18T10:00:00Z"}
	c.digest, c.issue = c.makePlan(t, c.planned, c.plan, crashAuthorizationID)
	return c
}

// makePlan writes a fresh plan to path and returns its digest and the
// issuing apply arguments for id. A refused plan returns no digest and must
// carry a typed reason.
func (c crashCase) makePlan(t *testing.T, args []string, path, id string) (string, []string) {
	t.Helper()
	var plan struct {
		Digest    string `json:"plan_digest"`
		Reason    string `json:"reason"`
		Approvals []struct {
			Tuple string `json:"tuple"`
		} `json:"approval_tuples"`
	}
	code, stdout, _ := c.f.run(append(append([]string{}, args...), "--output", path, "--format", "json")...)
	if err := json.Unmarshal([]byte(stdout), &plan); err != nil {
		t.Fatalf("plan exit %d, output %q", code, stdout)
	}
	if code != 0 {
		if plan.Reason == "" || !gitmanager.Reason(plan.Reason).Valid() {
			t.Fatalf("plan refused without a typed reason: %q", stdout)
		}
		t.Logf("plan refused: %s", plan.Reason)
		return "", nil
	}
	issue := []string{"apply", "--plan", path, "--confirm-plan", plan.Digest, "--authorization-id", id, "--issue-authorization",
		"--authorized-by", "operator"}
	if slices.Contains(args, "--initial-commit") {
		issue = append(issue, "--confirm-initial-commit")
	}
	for _, approval := range plan.Approvals {
		issue = append(issue, "--approve-file", approval.Tuple)
	}
	return plan.Digest, issue
}

// child runs the issuing apply in a child process with one crash point,
// or with none while recording the trace.
func (c crashCase) child(t *testing.T, point, trace string) (*os.ProcessState, string) {
	args, err := json.Marshal(c.issue)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRealEngineCrashChild$", "-test.count=1", "-test.v")
	cmd.Env = append(os.Environ(), crashBaseEnv+"="+c.f.base, crashArgsEnv+"="+string(args),
		crashPointEnv+"="+point, crashTraceEnv+"="+trace)
	output, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		t.Fatalf("child: %v", err)
	}
	return cmd.ProcessState, string(output)
}

// projectDigest is the digest of every entry, mode and content under the
// project, so a later attempt that changes anything is detected.
func projectDigest(t *testing.T, root string) string {
	h := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		fmt.Fprintf(h, "%s\x00%v\x00", rel, info.Mode())
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			h.Write(data)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

type crashReport struct {
	Unresolved    bool     `json:"unresolved"`
	CrashEvidence bool     `json:"crash_evidence"`
	TrailEvents   []string `json:"trail_events"`
	Authority     bool     `json:"grants_mutation_authority"`
	Prior         struct {
		State string `json:"state"`
	} `json:"prior_authorization"`
}

func validApplyResult(t *testing.T, label string, code int, stdout string) applyOutput {
	t.Helper()
	var out applyOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("%s: exit %d, output %q", label, code, stdout)
	}
	success := out.Result.Status == "applied" || out.Result.Status == "already_applied"
	if (code == 0) != success || (!success && out.Result.Reason == "") {
		t.Fatalf("%s: exit %d for %+v", label, code, out.Result)
	}
	return out
}

// T123/AC-005/AC-006/AC-018/AC-021/AC-026: the issuing apply of a real
// initialization with an initial commit is killed with SIGKILL before and
// after every durable store step and every Git invocation. After each
// crash, with the real store and real Git:
//
//   - a later consumption of the same authorization runs no Git
//     invocation and changes nothing once a mutation intent may be durable,
//     and otherwise ends in a valid result;
//   - the read-only reconcile report grants nothing and reports the
//     authorization as unresolved exactly when it is consumed without a
//     conclusive outcome;
//   - an unresolved authorization is reconciled through a fresh plan and a
//     linked authorization, which completes or refuses with a typed reason.
func TestRealEngineCrashRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("re-executes the test binary once per crash point")
	}
	baseline := newCrashCase(t)
	trace := filepath.Join(baseline.f.base, "trace")
	state, output := baseline.child(t, "", trace)
	if !state.Success() || !strings.Contains(output, "child exit 0") {
		t.Fatalf("baseline child %v\n%s", state, output)
	}
	data, err := os.ReadFile(trace)
	if err != nil {
		t.Fatal(err)
	}
	labels := strings.Fields(string(data))
	if len(labels) < 20 {
		t.Fatalf("the baseline recorded only %d durable steps: %v", len(labels), labels)
	}
	for _, label := range labels {
		for _, phase := range []string{"before", "after"} {
			point := label + ":" + phase
			t.Run(point, func(t *testing.T) {
				t.Parallel()
				c := newCrashCase(t)
				state, output := c.child(t, point, "")
				if status, ok := state.Sys().(syscall.WaitStatus); !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
					t.Fatalf("the child was not killed at %s: %v\n%s", point, state, output)
				}
				checkCrashRecovery(t, c)
			})
		}
	}
}

func checkCrashRecovery(t *testing.T, c crashCase) {
	var report crashReport
	reconcileCode, reconcileOut, _ := c.f.run("reconcile", "--plan", c.plan, "--authorization-id", crashAuthorizationID, "--format", "json")
	issued := json.Unmarshal([]byte(reconcileOut), &report) == nil && report.Prior.State != ""
	if issued && (reconcileCode != 0 || report.Authority) {
		t.Fatalf("reconcile exit %d %+v", reconcileCode, report)
	}
	intent := report.CrashEvidence
	for _, event := range report.TrailEvents {
		intent = intent || event == string(gitmanager.AuditEventMutationIntent)
	}
	consumed := report.Prior.State != "" && report.Prior.State != string(gitmanager.AuthorizationStateIssued) &&
		report.Prior.State != string(gitmanager.AuthorizationStateClaimed)
	conclusive := report.Prior.State == "applied" || report.Prior.State == "failed" || report.Prior.State == "partially_applied"
	if issued && report.Unresolved != (consumed && !conclusive || report.Prior.State == "claimed" && intent) {
		t.Fatalf("unresolved %v for state %s, intent %v", report.Unresolved, report.Prior.State, intent)
	}

	// A later consumption of the same authorization.
	counter := &countingRunner{GitRunner: c.f.deps.Runner}
	c.f.deps.Runner = counter
	before := projectDigest(t, c.f.project)
	code, stdout, _ := c.f.run("apply", "--plan", c.plan, "--confirm-plan", c.digest, "--authorization-id", crashAuthorizationID, "--format", "json")
	later := validApplyResult(t, "later consumption", code, stdout)
	if intent || consumed {
		if len(counter.runs) != 0 || projectDigest(t, c.f.project) != before {
			t.Fatalf("a later consumption after a possible intent ran %v and changed the project", counter.runs)
		}
		if later.Result.Status == "applied" {
			t.Fatalf("a consumed authorization was applied again: %+v", later.Result)
		}
	}
	path := fmt.Sprintf("state=%q intent=%v unresolved=%v later=%s/%s", report.Prior.State, intent, report.Unresolved,
		later.Result.Status, later.Result.Reason)
	if !report.Unresolved {
		t.Log(path)
		return
	}

	// Supervised reconciliation: a fresh plan for the state the crash left,
	// as an operator would request it after inspecting, and a new linked
	// authorization. A repository that exists is not initialized again,
	// and a commit group is dropped when it can no longer be planned.
	const linked = "123e4567-e89b-42d3-a456-426614174124"
	noInit := withoutOptions(c.planned, map[string]int{"--initialize-repository": 0, "--initial-branch": 1})
	noCommit := withoutOptions(noInit, map[string]int{"--initial-commit": 0, "--initial-commit-message-file": 1, "--include": 1,
		"--author-name": 1, "--author-email": 1, "--committer-name": 1, "--committer-email": 1, "--author-date": 1, "--committer-date": 1})
	var digest string
	var issue []string
	for i, args := range [][]string{c.planned, noInit, noCommit} {
		if digest, issue = c.makePlan(t, args, filepath.Join(c.f.base, fmt.Sprintf("reconcile-plan-%d.json", i)), linked); digest != "" {
			path += fmt.Sprintf(" plan=%d", i)
			break
		}
	}
	if digest == "" {
		t.Log(path + " reconciliation=plan_refused")
		return
	}
	code, stdout, _ = c.f.run(append(issue, "--reconciles-authorization", crashAuthorizationID, "--format", "json")...)
	reconciled := validApplyResult(t, "reconciliation", code, stdout)
	t.Logf("%s reconciliation=%s/%s", path, reconciled.Result.Status, reconciled.Result.Reason)
}

// withoutOptions removes each named option and the number of values it
// takes.
func withoutOptions(args []string, drop map[string]int) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		if values, ok := drop[args[i]]; ok {
			i += values
			continue
		}
		out = append(out, args[i])
	}
	return out
}
