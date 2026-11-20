package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/gitmanager"
	"github.com/germanmunozs/ai-dev/internal/gitprocess"
	"github.com/germanmunozs/ai-dev/internal/gitstore"
)

// newRealEngineFixture is the T106 pre-activation harness: the production
// composition (real Git reader, builder and runner, real store, the
// production dispatcher through the composition and the safety kernel)
// over an isolated project, HOME and state root. It does not use
// cmd/ai-dev/main.go and injects no executor, so every operation goes
// through the kernel's permits.
func newRealEngineFixture(t *testing.T) *gitAppFixture {
	f := realEngine(t, newGitAppFixture(t))
	if err := os.WriteFile(filepath.Join(f.project, "README.md"), []byte("# harness\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

// realEngine replaces the fixture's fakes with the production Git reader,
// builder and runner and the production dispatcher.
func realEngine(t *testing.T, f *gitAppFixture) *gitAppFixture {
	path, mode, err := gitprocess.LocateGit(os.Getenv("PATH"))
	if err != nil {
		t.Skipf("no usable Git: %v", err)
	}
	f.deps.Reader, f.deps.Commands, f.deps.Runner = gitprocess.Reader{Runner: gitprocess.Runner{}}, gitprocess.Builder{}, gitprocess.Runner{}
	f.deps.RandomSource, f.deps.Executor = rand.Reader, nil
	f.deps.IdentifyGit = func(ctx context.Context, environment gitmanager.EnvironmentProfile) (gitmanager.GitToolIdentity, error) {
		return gitprocess.Identify(ctx, mode, path, nil, environment, []byte("/"))
	}
	return f
}

func (f *gitAppFixture) runJSON(t *testing.T, value any, args ...string) int {
	t.Helper()
	code, stdout, stderr := f.run(append(args, "--format", "json")...)
	if err := json.Unmarshal([]byte(stdout), value); err != nil {
		t.Fatalf("%v: exit %d, output %q %q", args, code, stdout, stderr)
	}
	return code
}

type applyOutput struct {
	Result struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
		State  string `json:"durable_authorization_state"`
		Commit string `json:"commit_oid"`
	} `json:"result"`
}

func (f *gitAppFixture) terminalClosures(t *testing.T) int {
	matches, err := filepath.Glob(filepath.Join(f.stateRoot, "attempts", "*", "events", "*-terminal_authorization_cas.json"))
	if err != nil {
		t.Fatal(err)
	}
	return len(matches)
}

// T106/AC-004/AC-022: inspect, plan, issuance, apply with real Git effects,
// a repeated apply that is already_applied, the read-only reconcile
// report, a stale plan under a new authorization, and a held lock.
func TestRealEngineNonInteractiveFlow(t *testing.T) {
	f := newRealEngineFixture(t)
	message := filepath.Join(f.base, "message.txt")
	if err := os.WriteFile(message, []byte("chore: initial commit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := f.run("inspect", "--project-dir", f.project); code != 0 {
		t.Fatalf("inspect exit %d %s", code, stderr)
	}
	planPath := filepath.Join(f.base, "plan.json")
	var plan struct {
		Digest    string `json:"plan_digest"`
		Approvals []struct {
			Kind  string `json:"approval_kind"`
			Tuple string `json:"tuple"`
		} `json:"approval_tuples"`
	}
	if code := f.runJSON(t, &plan, "plan", "--project-dir", f.project, "--strategy", "basic", "--initialize-repository",
		"--initial-branch", "main", "--initial-commit", "--initial-commit-message-file", message, "--include", "README.md",
		"--author-name", "Ada", "--author-email", "ada@example.org", "--committer-name", "Ada", "--committer-email", "ada@example.org",
		"--author-date", "2026-11-18T10:00:00Z", "--committer-date", "2026-11-18T10:00:00Z", "--output", planPath); code != 0 {
		t.Fatalf("plan exit %d", code)
	}
	const id = "123e4567-e89b-42d3-a456-426614174106"
	issue := []string{"apply", "--plan", planPath, "--confirm-plan", plan.Digest, "--authorization-id", id, "--issue-authorization",
		"--authorized-by", "operator", "--confirm-initial-commit"}
	for _, approval := range plan.Approvals {
		issue = append(issue, "--approve-file", approval.Tuple)
	}
	var applied applyOutput
	if code := f.runJSON(t, &applied, issue...); code != 0 || applied.Result.Status != "applied" || applied.Result.Commit == "" {
		t.Fatalf("apply exit %d %+v", code, applied)
	}
	if ref, err := os.ReadFile(filepath.Join(f.project, ".git", "refs", "heads", "main")); err != nil || string(ref) != applied.Result.Commit+"\n" {
		t.Fatalf("main %q %v", ref, err)
	}
	var again applyOutput
	if code := f.runJSON(t, &again, "apply", "--plan", planPath, "--confirm-plan", plan.Digest, "--authorization-id", id); code != 0 ||
		again.Result.Status != "already_applied" {
		t.Fatalf("repeated apply exit %d %+v", code, again)
	}
	var reconcile struct {
		Unresolved bool `json:"unresolved"`
		Authority  bool `json:"grants_mutation_authority"`
	}
	if code := f.runJSON(t, &reconcile, "reconcile", "--plan", planPath, "--authorization-id", id); code != 0 || reconcile.Unresolved || reconcile.Authority {
		t.Fatalf("reconcile exit %d %+v", code, reconcile)
	}
	var stale applyOutput
	if code := f.runJSON(t, &stale, "apply", "--plan", planPath, "--confirm-plan", plan.Digest, "--authorization-id",
		"123e4567-e89b-42d3-a456-426614174107", "--issue-authorization", "--authorized-by", "operator"); code != 1 ||
		stale.Result.Status != "refused" || stale.Result.Reason != "plan_stale" {
		t.Fatalf("stale apply exit %d %+v", code, stale)
	}
	if f.terminalClosures(t) != 1 {
		t.Fatalf("%d terminal closures, want exactly one", f.terminalClosures(t))
	}

	// A held apply lock refuses a new project's apply before any effect.
	g := newRealEngineFixture(t)
	gPlan := filepath.Join(g.base, "plan.json")
	var basic struct {
		Digest string `json:"plan_digest"`
	}
	if code := g.runJSON(t, &basic, "plan", "--project-dir", g.project, "--strategy", "basic", "--initialize-repository",
		"--initial-branch", "main", "--output", gPlan); code != 0 {
		t.Fatalf("plan exit %d", code)
	}
	store, err := gitstore.Open(gitmanager.NewObservedPath([]byte(g.stateRoot)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	root, err := gitmanager.ResolveProjectRoot(g.project, "/")
	if err != nil {
		t.Fatal(err)
	}
	lock, err := store.AcquireApplyLock(context.Background(), root.Identity, "123e4567-e89b-42d3-a456-426614174199")
	if err != nil {
		t.Fatal(err)
	}
	var locked applyOutput
	code := g.runJSON(t, &locked, "apply", "--plan", gPlan, "--confirm-plan", basic.Digest, "--authorization-id", id,
		"--issue-authorization", "--authorized-by", "operator")
	lock.Release(context.Background())
	if code != 1 || locked.Result.Reason != "lock_conflict" {
		t.Fatalf("locked apply exit %d %+v", code, locked)
	}
	if _, err := os.Lstat(filepath.Join(g.project, ".git")); !os.IsNotExist(err) {
		t.Fatal("a locked apply created .git")
	}
}

// T106/F008-FR-002–008: the assistant over the real engine creates no
// authority before every confirmation, refuses without a terminal, and
// applies through the kernel once the digest is typed.
func TestRealEngineAssistant(t *testing.T) {
	for name, test := range map[string]struct {
		interactive bool
		answers     []func(string) string
		code        int
	}{
		"no terminal":   {false, nil, 2},
		"cancel":        {true, []func(string) string{literal("1"), literal("no")}, 1},
		"eof":           {true, []func(string) string{literal("1"), literal("yes")}, 1},
		"invalid input": {true, []func(string) string{literal("9")}, 2},
		"wrong digest":  {true, []func(string) string{literal("1"), literal("yes"), literal("main"), literal("plan.json"), literal("yes"), literal("sha256:" + strings.Repeat("0", 64))}, 1},
	} {
		t.Run(name, func(t *testing.T) {
			f := newRealEngineFixture(t)
			code, stdout, stderr := f.runWizard(test.interactive, test.answers...)
			if code != test.code || f.authorizations() != 0 || strings.Contains(stdout, "Authorization ") {
				t.Fatalf("exit %d, authorizations %d\n%s\n%s", code, f.authorizations(), stdout, stderr)
			}
			if _, err := os.Lstat(filepath.Join(f.project, ".git")); !os.IsNotExist(err) {
				t.Fatal("a refused assistant changed the project")
			}
		})
	}
	f := newRealEngineFixture(t)
	code, stdout, stderr := f.runWizard(true, literal("1"), literal("yes"), literal("main"), literal(filepath.Join(f.base, "plan.json")),
		literal("yes"), typeDigest, literal("operator"))
	if code != 0 || !strings.Contains(stdout, "status:         applied") || f.authorizations() != 1 {
		t.Fatalf("assistant exit %d\n%s\n%s", code, stdout, stderr)
	}
	head, err := os.ReadFile(filepath.Join(f.project, ".git", "HEAD"))
	config, configErr := os.ReadFile(filepath.Join(f.project, gitmanager.ProjectConfigPath))
	if err != nil || string(head) != "ref: refs/heads/main\n" || configErr != nil || !bytes.Contains(config, []byte(`"strategy":"basic"`)) {
		t.Fatalf("HEAD %q %v, config %q %v", head, err, config, configErr)
	}
}
