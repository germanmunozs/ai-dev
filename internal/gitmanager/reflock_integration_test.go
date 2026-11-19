//go:build (darwin && !ios) || (linux && !android)

package gitmanager_test

import (
	"context"
	"crypto/rand"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ai-dev/internal/gitmanager"
	"ai-dev/internal/gitprocess"
	"ai-dev/internal/gitstore"
)

// T128/T129: a reference lock left by an interrupted Git update, HEAD.lock,
// packed-refs.lock or a lock below refs, refuses the plan with
// lock_conflict; one that appears between plan and apply refuses the apply
// before the first blob, leaving the lock and the object database
// untouched. The repository is the one ai-dev itself initialized, so the
// commit plan over it also exercises the adopted configuration of T136.
func TestReferenceLocksRefuseBeforeTheFirstBlob(t *testing.T) {
	ctx := context.Background()
	g := newRealGit(t)
	inspection, err := g.inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range inspection.Incompatibilities() {
		if item.Reason == gitmanager.ReasonFilesystemUnsupported {
			t.Skip("the temporary directory is not on a production filesystem")
		}
	}
	store, err := gitstore.Open(gitmanager.NewObservedPath([]byte(g.state)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	planner := gitmanager.PlannerDependencies{Git: g.reader, Commands: gitprocess.Builder{}, Random: rand.Reader}
	meta := gitmanager.PlanMetadata{PlanID: "plan-1", GeneratedAt: time.Unix(200, 0).UTC()}
	initialized, rejection := gitmanager.Plan(ctx, planner, gitmanager.StrategyRequest{Strategy: "basic", InitializeRepository: true,
		Branches: map[string]string{"initial-branch": "main"}}, inspection, g.environment, meta)
	if rejection != nil {
		t.Fatalf("init plan: %s", rejection.Detail)
	}
	file, err := gitmanager.EncodePlanFile(initialized.Plan)
	if err != nil {
		t.Fatal(err)
	}
	if result := g.apply(store, file, initialized.Plan.PlanDigest, "123e4567-e89b-42d3-a456-426614174128"); result.Status != gitmanager.ApplyStatusApplied {
		t.Fatalf("initialization %s/%s", result.Status, result.Reason)
	}
	if err := os.WriteFile(filepath.Join(g.project, "README.md"), []byte("# locks\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 18, 10, 30, 0, 0, time.UTC)
	identity := gitmanager.GitIdentity{Name: "Ada Lovelace", Email: "ada@example.org"}
	request := gitmanager.StrategyRequest{Strategy: "basic", InitialCommit: &gitmanager.CommitRequest{
		MessageBytes: []byte("chore: initial commit\n"), Includes: []string{"README.md"},
		Author: identity, AuthorDate: at, Committer: identity, CommitterDate: at,
	}}
	candidates := gitmanager.CommitCandidates(request.InitialCommit.Includes)
	commitPlan := func() (gitmanager.PlanResult, *gitmanager.PlanRejection) {
		inspection, err := g.inspectCandidates(ctx, candidates)
		if err != nil {
			t.Fatal(err)
		}
		return gitmanager.Plan(ctx, planner, request, inspection, g.environment, meta)
	}
	gitDir := filepath.Join(g.project, ".git")
	for _, lock := range []string{"HEAD.lock", "packed-refs.lock", "refs/heads/main.lock"} {
		if err := os.WriteFile(filepath.Join(gitDir, lock), []byte("left by an interrupted update\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, rejection := commitPlan(); rejection == nil || rejection.Reason != gitmanager.ReasonLockConflict {
			t.Fatalf("%s: rejection %+v", lock, rejection)
		}
		os.Remove(filepath.Join(gitDir, lock))
	}

	planned, rejection := commitPlan()
	if rejection != nil {
		t.Fatalf("commit plan over the initialized repository: %s: %s", rejection.Reason, rejection.Detail)
	}
	commitFile, err := gitmanager.EncodePlanFile(planned.Plan)
	if err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(gitDir, "refs", "heads", "main.lock")
	if err := os.WriteFile(lock, []byte("appeared\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	objects := objectCount(t, gitDir)
	inspect := func(ctx context.Context, _ []byte, candidates [][]byte) (gitmanager.GitInspection, error) {
		return g.inspectCandidates(ctx, candidates)
	}
	// The revalidation before every operation refuses too, whatever the
	// operation: after the claim this is the only check (LCK-01).
	dispatcher := gitmanager.NewOperationDispatcher(gitmanager.DispatcherDependencies{
		Runner: gitprocess.Runner{}, Commands: gitprocess.Builder{}, Inspect: inspect, Random: rand.Reader})
	for _, operation := range planned.Plan.DerivedOperations() {
		if rejection := dispatcher.Revalidate(ctx, planned.Plan, operation); rejection == nil || rejection.Reason != gitmanager.ReasonLockConflict {
			t.Fatalf("revalidation of %s with a reference lock: %+v", operation.Kind, rejection)
		}
	}
	var approvals []string
	for _, file := range planned.Plan.ImmutableIntent.AuthorizedFiles() {
		approvals = append(approvals, file.PathUTF8+"|"+file.Mode+"|"+string(file.ContentSHA256)+"|blob:"+string(file.BlobOID))
	}
	result := gitmanager.Apply(ctx, gitmanager.ApplyDependencies{
		Store: store, Clock: fixedTime(time.Unix(1000, 0).UTC()), Random: rand.Reader, Planner: planner, Tool: g.tool,
		Environment: g.environment, Inspect: inspect,
		Executor: gitmanager.NewOperationDispatcher(gitmanager.DispatcherDependencies{
			Runner: gitprocess.Runner{}, Commands: gitprocess.Builder{}, Inspect: inspect, Random: rand.Reader}),
	}, gitmanager.ApplyRequest{PlanFile: commitFile, ConfirmPlan: string(planned.Plan.PlanDigest), AuthorizationID: "123e4567-e89b-42d3-a456-426614174129",
		Issue: &gitmanager.IssueRequest{AuthorizedBy: "operator", ConfirmInitialCommit: true, Approvals: approvals}})
	if result.Status != gitmanager.ApplyStatusRefused || result.Reason != gitmanager.ReasonPlanStale {
		t.Fatalf("apply with a reference lock %s/%s", result.Status, result.Reason)
	}
	if data, err := os.ReadFile(lock); err != nil || string(data) != "appeared\n" {
		t.Fatalf("the reference lock was touched: %q %v", data, err)
	}
	if after := objectCount(t, gitDir); after != objects {
		t.Fatalf("the refused apply wrote %d objects", after-objects)
	}
}

func objectCount(t *testing.T, gitDir string) int {
	n := 0
	err := filepath.WalkDir(filepath.Join(gitDir, "objects"), func(_ string, entry fs.DirEntry, err error) error {
		if err == nil && entry.Type().IsRegular() {
			n++
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}
