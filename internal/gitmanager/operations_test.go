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

// scriptedRunner records every invocation and returns a scripted result.
type scriptedRunner struct {
	runs   []GitInvocation
	result GitRunResult
	err    error
}

func (r *scriptedRunner) Run(_ context.Context, invocation GitInvocation) (GitRunResult, error) {
	r.runs = append(r.runs, invocation)
	return r.result, r.err
}

type dispatcherFixture struct {
	*planFixture
	plan   GitInitializationPlan
	runner *scriptedRunner
	d      *OperationDispatcher
}

func newDispatcherFixture(t *testing.T, req StrategyRequest) *dispatcherFixture {
	f := &dispatcherFixture{planFixture: newPlanFixture(t), runner: &scriptedRunner{}}
	f.plan = mustPlan(t, f.planFixture, req).Plan
	f.d = NewOperationDispatcher(DispatcherDependencies{Runner: f.runner, Commands: f.planFixture.deps.Commands,
		Inspect: func(context.Context, []byte, [][]byte) (GitInspection, error) { return f.inspect(), nil }})
	return f
}

// plannedPermit is a test-only permit bound to a synthetic started session
// whose authorization was issued for plan.
func plannedPermit(t *testing.T, plan GitInitializationPlan, op OperationID) MutationPermit {
	t.Helper()
	issue := IssueRequest{AuthorizationID: issuedID, AuthorizedBy: "op", ConfirmInitialCommit: plan.ImmutableIntent.InitialCommit}
	for _, file := range plan.ImmutableIntent.AuthorizedFiles() {
		tuple := file.PathUTF8 + "|" + file.Mode + "|" + string(file.ContentSHA256) + "|blob:" + string(file.BlobOID)
		if file.Ignored {
			issue.IgnoredApprovals = append(issue.IgnoredApprovals, tuple)
		} else {
			issue.Approvals = append(issue.Approvals, tuple)
		}
	}
	authorization, rejection := newAuthorization(issue, plan, time.Unix(1000, 0))
	if rejection != nil {
		t.Fatalf("authorization: %s", rejection.Detail)
	}
	session := &mutationSession{started: true, open: op, authorization: authorization}
	return MutationPermit{session: session, authorizationID: authorization.AuthorizationID, operationID: op}
}

func (f *dispatcherFixture) operation(kind OperationKind) DerivedOperation {
	for _, operation := range f.plan.DerivedOperations() {
		if operation.Kind == kind {
			return operation
		}
	}
	f.t.Fatalf("no %s operation", kind)
	return DerivedOperation{}
}

// T074: every mutation entrypoint fails closed, before invoking Git,
// without a permit for exactly this operation of this plan.
func TestDispatcherRequiresTheMutationPermit(t *testing.T) {
	f := newDispatcherFixture(t, basicNewRepo())
	init := f.operation(OperationKindRepositoryInit)
	other := newDispatcherFixture(t, newRepo("basic", map[string]string{"initial-branch": "other"}))
	stopped := plannedPermit(t, f.plan, init.OperationID)
	stopped.session.stop = &stopOutcome{}
	closed := plannedPermit(t, f.plan, init.OperationID)
	closed.session.closed = true
	foreign := plannedPermit(t, f.plan, init.OperationID)
	foreign.authorizationID = "123e4567-e89b-42d3-a456-426614174099"
	for name, permit := range map[string]MutationPermit{
		"zero permit":            {},
		"other operation":        plannedPermit(t, f.plan, f.operation(OperationKindProjectConfigWrite).OperationID),
		"other plan":             plannedPermit(t, other.plan, init.OperationID),
		"stopped session":        stopped,
		"closed session":         closed,
		"other authorization":    foreign,
		"kernel-only permit":     newTestMutationPermit(issuedID, init.OperationID),
		"not-started session":    {session: &mutationSession{open: init.OperationID}, operationID: init.OperationID},
		"operation not yet open": {session: &mutationSession{started: true}, operationID: init.OperationID},
	} {
		t.Run(name, func(t *testing.T) {
			if outcome := f.d.Execute(context.Background(), permit, f.plan, init); outcome != OperationFailed || len(f.runner.runs) != 0 {
				t.Fatalf("outcome %v after %d runs", outcome, len(f.runner.runs))
			}
		})
	}
}

// The dispatcher runs only operations of the regenerated plan, rebuilt to
// exactly the planned argv.
func TestDispatcherRunsOnlyThePlannedArgv(t *testing.T) {
	f := newDispatcherFixture(t, basicNewRepo())
	init := f.operation(OperationKindRepositoryInit)
	permit := plannedPermit(t, f.plan, init.OperationID)

	// An operation that is not the plan's, under a permit naming its ID.
	tampered := init
	tampered.Ordinal = 7
	if outcome := f.d.Execute(context.Background(), permit, f.plan, tampered); outcome != OperationFailed || len(f.runner.runs) != 0 {
		t.Fatalf("tampered operation ran: %v", outcome)
	}
	// A builder that no longer yields the planned argv.
	drifted := NewOperationDispatcher(DispatcherDependencies{Runner: f.runner, Commands: argvSuffix{f.planFixture.deps.Commands},
		Inspect: f.d.deps.Inspect})
	if outcome := drifted.Execute(context.Background(), permit, f.plan, init); outcome != OperationFailed || len(f.runner.runs) != 0 {
		t.Fatalf("drifted argv ran: %v", outcome)
	}

	f.d.Execute(context.Background(), permit, f.plan, init)
	if len(f.runner.runs) != 1 || !equalLists(f.runner.runs[0].Args, init.ExactArgv()) {
		t.Fatalf("runs %+v", f.runner.runs)
	}
}

type argvSuffix struct{ inner GitCommandBuilder }

func (b argvSuffix) BuildCommand(c GitCommand) (GitInvocation, error) {
	invocation, err := b.inner.BuildCommand(c)
	invocation.Args = append(invocation.Args, "--extra")
	return invocation, err
}

// Exit status alone proves nothing: completion requires the operation's
// expected postconditions on a fresh inspection, and a runner failure is
// indeterminate (F008-FR-166).
func TestDispatcherOutcomes(t *testing.T) {
	for name, test := range map[string]struct {
		result GitRunResult
		err    error
		want   OperationOutcome
	}{
		"git failed":                 {GitRunResult{ExitCode: 128}, nil, OperationFailed},
		"runner failed":              {GitRunResult{ExitCode: -1}, errors.New("killed"), OperationIndeterminate},
		"output exceeded":            {GitRunResult{}, ErrCapturedOutputExceeded, OperationIndeterminate},
		"success without the effect": {GitRunResult{}, nil, OperationFailed},
	} {
		t.Run(name, func(t *testing.T) {
			f := newDispatcherFixture(t, basicNewRepo())
			f.runner.result, f.runner.err = test.result, test.err
			init := f.operation(OperationKindRepositoryInit)
			if outcome := f.d.Execute(context.Background(), plannedPermit(t, f.plan, init.OperationID), f.plan, init); outcome != test.want {
				t.Fatalf("outcome %v, want %v", outcome, test.want)
			}
		})
	}
}

// Verifications run nothing, and project-configuration writes are not
// dispatched until their filesystem protocol exists (T093).
func TestDispatcherNonGitOperations(t *testing.T) {
	f := newDispatcherFixture(t, basicNewRepo())
	verification := f.operation(OperationKindVerification)
	if outcome := f.d.Execute(context.Background(), plannedPermit(t, f.plan, verification.OperationID), f.plan, verification); outcome != OperationCompleted {
		t.Fatalf("verification %v", outcome)
	}
	write := f.operation(OperationKindProjectConfigWrite)
	if outcome := f.d.Execute(context.Background(), plannedPermit(t, f.plan, write.OperationID), f.plan, write); outcome != OperationFailed {
		t.Fatalf("project config write %v", outcome)
	}
	if len(f.runner.runs) != 0 {
		t.Fatalf("runs %+v", f.runner.runs)
	}
}

// Immediate revalidation reads fresh state: an appearing .git refuses the
// initialization, so an existing repository is never reinitialized.
func TestDispatcherRevalidation(t *testing.T) {
	f := newDispatcherFixture(t, basicNewRepo())
	init := f.operation(OperationKindRepositoryInit)
	if rejection := f.d.Revalidate(context.Background(), f.plan, init); rejection != nil {
		t.Fatalf("revalidation of unchanged state: %s", rejection.Detail)
	}
	if err := os.Mkdir(filepath.Join(f.root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if rejection := f.d.Revalidate(context.Background(), f.plan, init); rejection == nil || rejection.Reason != ReasonConcurrentChange {
		t.Fatalf("revalidation after .git appeared: %+v", rejection)
	}
	failing := NewOperationDispatcher(DispatcherDependencies{Inspect: func(context.Context, []byte, [][]byte) (GitInspection, error) {
		return GitInspection{}, errors.New("unreadable")
	}})
	if failing.Revalidate(context.Background(), f.plan, init) == nil {
		t.Fatal("an uninspectable project revalidated")
	}
	if evidence := failing.Verify(context.Background(), f.plan); !evidence.EffectsUnverifiable || evidence.ZeroEffectsProven {
		t.Fatalf("uninspectable evidence %+v", evidence)
	}
}

// Verification of an untouched project proves zero effects; nothing is
// complete.
func TestDispatcherVerifyUntouchedProject(t *testing.T) {
	f := newDispatcherFixture(t, basicNewRepo())
	evidence := f.d.Verify(context.Background(), f.plan)
	if evidence.VerifiedComplete || !evidence.ZeroEffectsProven || evidence.EffectsUnverifiable || len(evidence.VerifiedEffects) != 0 {
		t.Fatalf("evidence %+v", evidence)
	}
}
