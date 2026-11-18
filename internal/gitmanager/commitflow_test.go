//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newCommitDispatcher(t *testing.T, files map[string]string, includes ...string) *dispatcherFixture {
	f := &dispatcherFixture{planFixture: newPlanFixture(t), runner: &scriptedRunner{}}
	f.plan = mustCommitPlan(t, f.planFixture, files, commitRequest(includes...))
	f.d = NewOperationDispatcher(DispatcherDependencies{Runner: f.runner, Commands: f.planFixture.deps.Commands,
		Inspect: func(context.Context, []byte, [][]byte) (GitInspection, error) { return f.inspect(), nil }})
	return f
}

// writeConfig writes the planned project configuration, as the attempt's
// configuration write would before the commit group.
func (f *dispatcherFixture) writeConfig() {
	approved, rejection := approvedBytes(f.plan, false)
	if rejection != nil {
		f.t.Fatal(rejection)
	}
	mustWrite(f.t, filepath.Join(f.root, ProjectConfigPath), string(approved[ProjectConfigPath]))
}

// T081/AC-008/AC-028: the barrier immediately before the first blob stops
// the group with zero Git invocations when an approved file differs.
func TestCommitBarrierStopsBeforeTheFirstBlob(t *testing.T) {
	f := newCommitDispatcher(t, map[string]string{"a.txt": "a\n"}, "a.txt")
	commit := f.operation(OperationKindInitialCommit)
	permit := plannedPermit(t, f.plan, commit.OperationID)
	// The configuration this attempt writes is not there yet.
	if outcome := f.d.Execute(context.Background(), permit, f.plan, commit); outcome != OperationFailed || len(f.runner.runs) != 0 {
		t.Fatalf("outcome %v after %d runs", outcome, len(f.runner.runs))
	}
	f.writeConfig()
	mustWrite(t, filepath.Join(f.root, "a.txt"), "b\n")
	if outcome := f.d.Execute(context.Background(), permit, f.plan, commit); outcome != OperationFailed || len(f.runner.runs) != 0 {
		t.Fatalf("outcome %v after %d runs", outcome, len(f.runner.runs))
	}
	if err := os.Chmod(filepath.Join(f.root, "a.txt"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(f.root, "a.txt"), "a\n")
	if outcome := f.d.Execute(context.Background(), permit, f.plan, commit); outcome != OperationFailed || len(f.runner.runs) != 0 {
		t.Fatalf("a changed Git mode ran: %v after %d runs", outcome, len(f.runner.runs))
	}
}

// T081: Git must return exactly each planned OID; exit status alone proves
// nothing, and a runner failure is indeterminate (F008-FR-110, 166).
func TestCommitObjectOutcomes(t *testing.T) {
	for name, test := range map[string]struct {
		result GitRunResult
		err    error
		want   OperationOutcome
	}{
		"different OID":  {GitRunResult{Stdout: []byte(testCommitOID + "\n")}, nil, OperationFailed},
		"git failed":     {GitRunResult{ExitCode: 128}, nil, OperationFailed},
		"runner failed":  {GitRunResult{ExitCode: -1}, errors.New("killed"), OperationIndeterminate},
		"missing output": {GitRunResult{}, nil, OperationFailed},
	} {
		t.Run(name, func(t *testing.T) {
			f := newCommitDispatcher(t, map[string]string{"a.txt": "a\n"}, "a.txt")
			f.writeConfig()
			f.runner.result, f.runner.err = test.result, test.err
			commit := f.operation(OperationKindInitialCommit)
			if outcome := f.d.Execute(context.Background(), plannedPermit(t, f.plan, commit.OperationID), f.plan, commit); outcome != test.want {
				t.Fatalf("outcome %v, want %v", outcome, test.want)
			}
			if len(f.runner.runs) != 1 || f.runner.runs[0].Args[2] != string(GitCommandHashBlob) {
				t.Fatalf("runs %+v", f.runner.runs)
			}
		})
	}
}

// T081/AC-010: the plan names the exact objects, the absent-old CAS, the
// prepared-index roles and the final verification (F008-FR-111–125, 192).
func TestCommitGroupPlan(t *testing.T) {
	f := newCommitDispatcher(t, map[string]string{"a.txt": "a\n", "keep.txt": "keep\n"}, "a.txt")
	objects, err := planCommit(f.plan.ImmutableIntent)
	if err != nil {
		t.Fatal(err)
	}
	commit := f.operation(OperationKindInitialCommit)
	if commit.TargetIdentity != "commit:"+string(objects.commit.oid) || commit.StdinDigest != contentDigest([]byte("chore: initial commit\n")) ||
		commit.PartialEffectClassification != PartialEffectPartialPossible {
		t.Fatalf("commit operation %+v", commit)
	}
	cas := f.operation(OperationKindInitialRefCAS)
	argv := cas.ExactArgv()
	if argv[len(argv)-3] != "refs/heads/main" || argv[len(argv)-2] != string(objects.commit.oid) || argv[len(argv)-1] != "" {
		t.Fatalf("CAS argv %q", argv)
	}
	index := f.operation(OperationKindPreparedIndex)
	if !equalLists(index.TemporaryResourceRoles(), []ResourceRole{ResourceRoleIndexLock, ResourceRolePreparedIndex}) {
		t.Fatalf("index roles %v", index.TemporaryResourceRoles())
	}
	// The plan-level postconditions are the final state: HEAD attached,
	// the ref at the commit, the index at the tree, clean authorized paths.
	final := map[ConditionKind]bool{}
	for _, condition := range f.plan.ExpectedPostconditions() {
		final[condition.Kind] = true
		if condition.Kind == ConditionHeadState && condition.Expected != string(HeadStateAttached) {
			t.Fatalf("final HEAD state %s", condition.Expected)
		}
		if condition.Kind == ConditionPathAbsent || condition.Kind == ConditionRefAbsent || condition.Kind == ConditionIndexAbsent {
			t.Fatalf("contradicted condition %s remains", condition.Kind)
		}
	}
	for _, kind := range []ConditionKind{ConditionRefTarget, ConditionIndexTree, ConditionPathStatusClean, ConditionObjectType} {
		if !final[kind] {
			t.Errorf("final postconditions lack %s", kind)
		}
	}
	// An initial-ref CAS that Git refuses leaves the outcome failed.
	f.runner.result = GitRunResult{ExitCode: 1}
	if outcome := f.d.Execute(context.Background(), plannedPermit(t, f.plan, cas.OperationID), f.plan, cas); outcome != OperationFailed {
		t.Fatalf("refused CAS %v", outcome)
	}
}

// T081: an intent naming bytes the snapshot did not freeze cannot be
// regenerated into a plan.
func TestCommitIntentBoundToSnapshot(t *testing.T) {
	f := newCommitDispatcher(t, map[string]string{"a.txt": "a\n"}, "a.txt")
	intent := f.plan.ImmutableIntent
	files := intent.AuthorizedFiles()
	for i := range files {
		if files[i].PathUTF8 == "a.txt" {
			files[i].ContentSHA256 = contentDigest([]byte("other\n"))
		}
	}
	tampered, err := NewImmutableIntent(ImmutableIntentInput{
		ProjectID: intent.ProjectID, Strategy: intent.Strategy, BranchParameters: intent.BranchParameters(), PrefixParameters: intent.PrefixParameters(),
		InitializeRepository: true, ObjectFormat: intent.ObjectFormat, RefFormat: intent.RefFormat, ToolIdentity: intent.ToolIdentity,
		ProjectConfigBytesDigest: intent.ProjectConfigBytesDigest, InitialCommit: true, Author: intent.Author, Committer: intent.Committer,
		AuthorTimestamp: intent.AuthorTimestamp, CommitterTimestamp: intent.CommitterTimestamp, CommitMessageBytes: intent.CommitMessageBytes(),
		AuthorizedFiles: files, RequiredCapabilities: intent.RequiredCapabilities(),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, rejection := Regenerate(context.Background(), f.deps, tampered, f.inspect(), validEnvironmentProfile(), PlanMetadata{PlanID: "p", GeneratedAt: time.Unix(1, 0)})
	expectRejection(t, rejection, ReasonPlanInvalid, 1)
}

// unbornRepository makes the project an existing clean repository whose
// HEAD names the unborn refs/heads/main.
func (f *planFixture) unbornRepository(refs string) {
	f.repository()
	f.git.inner.results[GitReadHeadCommit] = GitRunResult{ExitCode: 128}
	f.git.inner.results[GitReadShowRefs] = GitRunResult{Stdout: []byte(refs), ExitCode: map[bool]int{true: 1}[refs == ""]}
	f.git.inner.results[GitReadStatus] = GitRunResult{}
}

// T081/AC-029: in an existing repository the commit goes to the unborn
// branch HEAD names, which must be the strategy's branch; history refuses
// the group (F008-FR-112).
func TestCommitInExistingUnbornRepository(t *testing.T) {
	trunk := func(branch string) StrategyRequest {
		return StrategyRequest{Strategy: "trunk-based", Branches: map[string]string{"trunk-branch": branch}, InitialCommit: commitRequest("staged.txt")}
	}
	f := newPlanFixture(t)
	f.unbornRepository("")
	f.request.Candidates = CommitCandidates([]string{"staged.txt"})
	plan := mustPlan(t, f, trunk("main")).Plan
	if plan.DerivedOperations()[0].Kind == OperationKindRepositoryInit {
		t.Fatal("an existing repository is initialized")
	}
	for _, operation := range plan.DerivedOperations() {
		if operation.Kind == OperationKindInitialRefCAS && operation.TargetIdentity != "ref:refs/heads/main" {
			t.Fatalf("initial ref %s", operation.TargetIdentity)
		}
	}
	_, rejection := f.plan(trunk("other"))
	expectRejection(t, rejection, ReasonStrategyConflict, 1)

	f = newPlanFixture(t)
	f.unbornRepository(testCommitOID + " refs/heads/old\n")
	f.request.Candidates = CommitCandidates([]string{"staged.txt"})
	_, rejection = f.plan(trunk("main"))
	expectRejection(t, rejection, ReasonRepositoryStateIncompatible, 1)
}

// T081/F008-FR-192: with Git Flow the development branch is a separate CAS
// after the commit group, at the planned commit, and is not pending.
func TestCommitGitFlowDevelopmentAfterGroup(t *testing.T) {
	f := newPlanFixture(t)
	req := gitFlowRequest("avh")
	req.InitialCommit = commitRequest()
	f.request.Candidates = CommitCandidates(nil)
	result := mustPlan(t, f, req)
	operations := result.Plan.DerivedOperations()
	last := operations[len(operations)-1]
	objects, _ := planCommit(result.Plan.ImmutableIntent)
	if last.Kind != OperationKindDevelopmentRefCAS || last.TargetIdentity != "ref:refs/heads/develop" ||
		!containsValue(last.ExactArgv(), string(objects.commit.oid)) || operations[len(operations)-2].Kind != OperationKindVerification {
		t.Fatalf("last operations %s %s", operations[len(operations)-2].Kind, last.Kind)
	}
	if len(result.PendingDependencies) != 0 || developmentPending(result.Plan) {
		t.Fatalf("pending %v", result.PendingDependencies)
	}
}
