//go:build (darwin && !ios) || (linux && !android)

package gitmanager_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"ai-dev/internal/gitmanager"
	"ai-dev/internal/gitstore"
)

const integrationAuthorizationID = "123e4567-e89b-42d3-a456-426614174077"

type refFormatReader struct{}

func (refFormatReader) Read(context.Context, gitmanager.GitReadRequest) (gitmanager.GitRunResult, error) {
	return gitmanager.GitRunResult{}, nil
}

type argvBuilder struct{}

func (argvBuilder) BuildCommand(c gitmanager.GitCommand) (gitmanager.GitInvocation, error) {
	if c.Kind == gitmanager.GitCommandInit {
		return gitmanager.GitInvocation{Args: []string{"init", "--template=", "--object-format=sha1", "--ref-format=files",
			"--initial-branch=" + string(c.BranchName), "--", string(c.PhysicalRoot)}}, nil
	}
	return gitmanager.GitInvocation{Args: []string{string(c.Kind), string(c.BranchName), c.ConfigKey, c.ConfigValue}}, nil
}

type completingExecutor struct{ executed int }

func (e *completingExecutor) Revalidate(context.Context, gitmanager.GitInitializationPlan, gitmanager.DerivedOperation) *gitmanager.PlanRejection {
	return nil
}
func (e *completingExecutor) Execute(context.Context, gitmanager.MutationPermit, gitmanager.GitInitializationPlan, gitmanager.DerivedOperation) gitmanager.OperationOutcome {
	e.executed++
	return gitmanager.OperationCompleted
}
func (e *completingExecutor) Verify(context.Context, gitmanager.GitInitializationPlan) gitmanager.ApplyEvidence {
	return gitmanager.ApplyEvidence{VerifiedComplete: true}
}

type fixedTime time.Time

func (t fixedTime) Now() time.Time { return time.Time(t) }

type integration struct {
	t          *testing.T
	stateRoot  string
	store      *gitstore.Store
	executor   *completingExecutor
	deps       gitmanager.ApplyDependencies
	plan       gitmanager.GitInitializationPlan
	file       []byte
	inspection func() gitmanager.GitInspection
}

func newIntegration(t *testing.T) *integration {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project, home := filepath.Join(base, "project"), filepath.Join(base, "home")
	for _, dir := range []string{project, home} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// A credential in the user's configuration must never reach a record.
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[remote \"x\"]\n\turl = https://u:SECRET-TOKEN@h/r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool, err := gitmanager.NewGitToolIdentity(gitmanager.GitToolIdentityInput{
		InvocationMode: gitmanager.InvocationModeDirect, InvocationPath: "/usr/bin/git",
		InvocationPathDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", InvocationDevice: "1",
		InvocationInode: "2", EffectiveExecutablePath: "/usr/bin/git",
		EffectiveExecutableDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Version:                   "2.54.0", ExecPath: "/usr/libexec/git-core",
		Capabilities:         []string{"config-end-of-options", "initial-branch", "object-format", "path-format", "ref-format"},
		EnvironmentSelectors: map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	stateRoot := filepath.Join(base, "state", "git")
	environment, err := gitmanager.NewEnvironmentProfile(gitmanager.NewObservedPath([]byte(stateRoot)))
	if err != nil {
		t.Fatal(err)
	}
	store, err := gitstore.Open(gitmanager.NewObservedPath([]byte(stateRoot)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	inspect := func() gitmanager.GitInspection {
		inspection, err := gitmanager.Inspect(context.Background(), gitmanager.InspectorDependencies{Git: refFormatReader{}, ObserveFilesystem: gitstore.ObserveFilesystem},
			gitmanager.InspectRequest{ProjectDir: project, WorkingDir: "/", Tool: tool, Environment: environment,
				User: gitmanager.UserContext{Home: []byte(home)}, InspectionID: "inspection-1", InspectedAt: time.Unix(100, 0).UTC()})
		if err != nil {
			t.Fatal(err)
		}
		return inspection
	}
	planner := gitmanager.PlannerDependencies{Git: refFormatReader{}, Commands: argvBuilder{}, Random: rand.Reader}
	first := inspect()
	for _, item := range first.Incompatibilities() {
		if item.Reason == gitmanager.ReasonFilesystemUnsupported {
			t.Skip("the temporary directory is not on a production filesystem")
		}
	}
	return &integration{t: t, stateRoot: stateRoot, store: store, inspection: inspect, executor: &completingExecutor{},
		deps: gitmanager.ApplyDependencies{Store: store, Clock: fixedTime(time.Unix(1000, 0).UTC()), Random: rand.Reader, Planner: planner,
			Tool: tool, Environment: environment, Executor: nil,
			Inspect: func(context.Context, []byte) (gitmanager.GitInspection, error) { return inspect(), nil }}}
}

func (it *integration) planBasic() {
	it.t.Helper()
	inspection := it.inspection()
	result, rejection := gitmanager.Plan(context.Background(), it.deps.Planner,
		gitmanager.StrategyRequest{Strategy: "basic", InitializeRepository: true, Branches: map[string]string{"initial-branch": "main"}},
		inspection, it.deps.Environment, gitmanager.PlanMetadata{PlanID: "plan-1", GeneratedAt: time.Unix(200, 0).UTC()})
	if rejection != nil {
		it.t.Fatalf("planning refused: %s: %s", rejection.Reason, rejection.Detail)
	}
	data, err := gitmanager.EncodePlanFile(result.Plan)
	if err != nil {
		it.t.Fatal(err)
	}
	it.plan, it.file = result.Plan, data
	it.deps.Executor = it.executor
}

func (it *integration) apply(issue *gitmanager.IssueRequest) gitmanager.GitApplyResult {
	return gitmanager.Apply(context.Background(), it.deps, gitmanager.ApplyRequest{PlanFile: it.file,
		ConfirmPlan: string(it.plan.PlanDigest), AuthorizationID: integrationAuthorizationID, Issue: issue})
}

func (it *integration) attemptDir(id gitmanager.ApplyID) string {
	return filepath.Join(it.stateRoot, "attempts", string(id))
}

func (it *integration) events(id gitmanager.ApplyID) []string {
	entries, err := os.ReadDir(filepath.Join(it.attemptDir(id), "events"))
	if err != nil {
		it.t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name()[strings.IndexByte(entry.Name(), '-')+1:strings.LastIndexByte(entry.Name(), '.')])
	}
	return names
}

// T061: issuance, scope binding and consumption through the real store.
func TestAuthorizationStoreIntegration(t *testing.T) {
	it := newIntegration(t)
	it.planBasic()
	result := it.apply(&gitmanager.IssueRequest{AuthorizedBy: "operator"})
	if result.Status != gitmanager.ApplyStatusApplied || result.ExitCode != 0 {
		t.Fatalf("result %+v", result)
	}
	authorization, err := it.store.LoadAuthorization(context.Background(), it.plan.PreconditionSnapshot.ProjectIdentity.Digest(), integrationAuthorizationID)
	if err != nil || authorization.State != gitmanager.AuthorizationStateApplied || authorization.PlanDigest != it.plan.PlanDigest ||
		len(authorization.AuthorizedOperationIDs()) != len(it.plan.DerivedOperations()) || len(authorization.TransitionHistory()) != 3 {
		t.Fatalf("durable authorization %+v %v", authorization, err)
	}
	// Owner-visible outcome of a later attempt: consumed, recorded,
	// nothing executed and the authorization unchanged.
	executed := it.executor.executed
	again := it.apply(nil)
	if again.Status != gitmanager.ApplyStatusRefused || again.Reason != gitmanager.ReasonAuthorizationConsumed ||
		it.executor.executed != executed {
		t.Fatalf("later attempt %+v", again)
	}
	reissue := it.apply(&gitmanager.IssueRequest{AuthorizedBy: "someone"})
	if reissue.Reason != gitmanager.ReasonAuthorizationIDExists {
		t.Fatalf("reissue %+v", reissue)
	}
	if after, _ := it.store.LoadAuthorization(context.Background(), it.plan.PreconditionSnapshot.ProjectIdentity.Digest(), integrationAuthorizationID); after.StateVersion != authorization.StateVersion {
		t.Fatal("a later attempt transitioned the authorization")
	}
}

// T062: the audit trail through the single writer, on disk.
func TestAuditStoreIntegration(t *testing.T) {
	it := newIntegration(t)
	it.planBasic()
	result := it.apply(&gitmanager.IssueRequest{AuthorizedBy: "operator"})
	dir := it.attemptDir(result.ApplyID)
	for _, name := range []string{"attempt.json", "result.json"} {
		if info, err := os.Stat(filepath.Join(dir, name)); err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("%s: %v %v", name, info, err)
		}
	}
	events := it.events(result.ApplyID)
	want := []string{"authorization_issued", "authorization_transition", "mutation_intent", "authorization_transition"}
	for i := 0; i < len(it.plan.DerivedOperations()); i++ {
		want = append(want, "operation_started", "operation_completed")
	}
	want = append(want, "resource_release_started", "resource_release_result", "prepared_terminal_result", "terminal_authorization_cas")
	if strings.Join(events, ",") != strings.Join(want, ",") {
		t.Fatalf("events\n%v\nwant\n%v", events, want)
	}
	// The terminal CAS bound the digest of the durable prepared record.
	entries, _ := os.ReadDir(filepath.Join(dir, "events"))
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	prepared, err := os.ReadFile(filepath.Join(dir, "events", entries[len(entries)-2].Name()))
	if err != nil {
		t.Fatal(err)
	}
	authorization, _ := it.store.LoadAuthorization(context.Background(), it.plan.PreconditionSnapshot.ProjectIdentity.Digest(), integrationAuthorizationID)
	if string(authorization.TerminalResultDigest) != fmt.Sprintf("sha256:%x", sha256.Sum256(prepared)) {
		t.Fatal("terminal digest does not bind the prepared record")
	}
	// A refused attempt is recorded before its result is returned.
	refused := it.apply(nil)
	if _, err := os.Stat(filepath.Join(it.attemptDir(refused.ApplyID), "result.json")); err != nil {
		t.Fatalf("refusal not recorded: %v", err)
	}
	// No credential reaches any durable record.
	_ = filepath.WalkDir(it.stateRoot, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && entry.Type().IsRegular() {
			if data, _ := os.ReadFile(path); strings.Contains(string(data), "SECRET-TOKEN") {
				t.Errorf("credential in %s", path)
			}
		}
		return nil
	})
}
