package gitmanager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

var kernelOperations = []OperationID{operationID(0), operationID(1)}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

// kernelFixture is one issued authorization in a fake store, with a kernel
// whose clock is inside the authorization's validity.
type kernelFixture struct {
	t             *testing.T
	store         *fakeGitStore
	kernel        safetyKernel
	project       PhysicalProjectIdentity
	authorization GitAuthorization
}

func newKernelFixture(t *testing.T) *kernelFixture {
	t.Helper()
	store := newFakeGitStore()
	in := validAuthorizationInput(t)
	in.SchemaVersion = GitAuthorizationSchema
	in.AuthorizedOperationIDs = kernelOperations
	in.ExpiresAt = time.Unix(3600, 0).UTC()
	authorization, err := NewGitAuthorization(in)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.IssueAuthorization(context.Background(), authorization); err != nil {
		t.Fatal(err)
	}
	return &kernelFixture{
		t: t, store: store, kernel: safetyKernel{store: store, clock: fixedClock{time.Unix(100, 0).UTC()}},
		project: authorization.ProjectIdentity, authorization: authorization,
	}
}

func (f *kernelFixture) journal(applyID ApplyID) *attemptJournal {
	f.t.Helper()
	journal, err := openAttemptJournal(context.Background(), f.store, GitApplyAttempt{
		SchemaVersion: GitApplyAttemptSchema, ApplyID: applyID, AuthorizationID: f.authorization.AuthorizationID,
		ProjectIdentity: f.project, StartedAt: time.Unix(100, 0).UTC(), InvocationMode: InvocationModeDirect,
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return journal
}

func (f *kernelFixture) request(applyID ApplyID) claimRequest {
	return claimRequest{
		journal: f.journal(applyID), project: f.project, authorization: f.authorization.AuthorizationID,
		planDigest: f.authorization.PlanDigest, planID: "plan-1",
	}
}

func (f *kernelFixture) claim(applyID ApplyID) *mutationSession {
	f.t.Helper()
	session, refusal := f.kernel.claim(context.Background(), f.request(applyID))
	if refusal != nil {
		f.t.Fatalf("claim refused: %+v", *refusal)
	}
	return session
}

func (f *kernelFixture) durable() GitAuthorization {
	f.t.Helper()
	authorization, err := f.store.LoadAuthorization(context.Background(), f.project.Digest(), f.authorization.AuthorizationID)
	if err != nil {
		f.t.Fatal(err)
	}
	return authorization
}

func (f *kernelFixture) events(applyID ApplyID) []AuditEventKind {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()
	return f.store.kindsLocked(applyID)
}

func (f *kernelFixture) recordedResult(applyID ApplyID) []byte {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()
	return cloneSlice(f.store.results[applyID])
}

// lockFree proves the apply lock was released by acquiring and releasing it.
func (f *kernelFixture) lockFree() bool {
	lock, err := f.store.AcquireApplyLock(context.Background(), f.project, testOtherApplyID)
	if err != nil {
		return false
	}
	return lock.Release(context.Background()) == nil
}

func mustBegin(t *testing.T, session *mutationSession, op OperationID) MutationPermit {
	t.Helper()
	permit, err := session.begin(context.Background(), op)
	if err != nil {
		t.Fatalf("begin %s: %v", op, err)
	}
	return permit
}

func mustFinish(t *testing.T, session *mutationSession, permit MutationPermit, outcome operationOutcome) {
	t.Helper()
	if err := session.finish(context.Background(), permit, outcome); err != nil {
		t.Fatalf("finish: %v", err)
	}
}

// newTestMutationPermit is the controlled test-only permit constructor: it
// binds a permit to a synthetic started session so dispatcher tests can
// exercise a permitted path without the durable kernel.
func newTestMutationPermit(authorization AuthorizationID, op OperationID) MutationPermit {
	session := &mutationSession{started: true, open: op}
	session.authorization.AuthorizationID = authorization
	return MutationPermit{session: session, authorizationID: authorization, operationID: op}
}

// The permit is emitted only after mutation intent, the mutation-start CAS
// and operation_started are durable, in that order.
func TestSafetyKernelDurableStepsPrecedeThePermit(t *testing.T) {
	f := newKernelFixture(t)
	session := f.claim(testApplyID)
	if claimed := f.durable(); claimed.State != AuthorizationStateClaimed || claimed.OwnerApplyID != testApplyID {
		t.Fatalf("claim left %+v", claimed)
	}
	if f.lockFree() {
		t.Fatal("claimed session does not hold the apply lock")
	}
	callsBefore := len(f.store.callsSnapshot())
	permit := mustBegin(t, session, kernelOperations[0])
	calls := f.store.callsSnapshot()[callsBefore:]
	want := []string{"AppendAuditEvent", "TransitionAuthorization", "AppendAuditEvent", "AppendAuditEvent"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("store calls before the permit %v, want %v", calls, want)
	}
	wantEvents := []AuditEventKind{AuditEventAuthorizationTransition, AuditEventMutationIntent, AuditEventAuthorizationTransition, AuditEventOperationStarted}
	if events := f.events(testApplyID); !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("events %v, want %v", events, wantEvents)
	}
	if started := f.durable(); started.State != AuthorizationStateMutationStarted || started.OwnerApplyID != testApplyID {
		t.Fatalf("durable authorization %+v", started)
	}
	if err := permit.authorizes(f.authorization.AuthorizationID, kernelOperations[0]); err != nil {
		t.Fatalf("permit refused its own operation: %v", err)
	}
	for name, check := range map[string]error{
		"other operation":     permit.authorizes(f.authorization.AuthorizationID, kernelOperations[1]),
		"other authorization": permit.authorizes("123e4567-e89b-42d3-a456-426614174005", kernelOperations[0]),
		"zero permit":         MutationPermit{}.authorizes(f.authorization.AuthorizationID, kernelOperations[0]),
	} {
		if !errors.Is(check, errMutationNotPermitted) {
			t.Errorf("%s: %v", name, check)
		}
	}

	// One operation at a time, each once, and only within scope.
	if _, err := session.begin(context.Background(), kernelOperations[1]); err == nil {
		t.Fatal("second operation began while one is open")
	}
	mustFinish(t, session, permit, operationCompleted)
	if err := permit.authorizes(f.authorization.AuthorizationID, kernelOperations[0]); !errors.Is(err, errMutationNotPermitted) {
		t.Fatal("permit still valid after its operation finished")
	}
	if err := session.finish(context.Background(), permit, operationCompleted); err == nil {
		t.Fatal("operation finished twice")
	}
	if _, err := session.begin(context.Background(), kernelOperations[0]); err == nil {
		t.Fatal("operation began twice")
	}
	if _, err := session.begin(context.Background(), operationID(7)); err == nil {
		t.Fatal("operation outside the authorization scope began")
	}
	second := mustBegin(t, session, kernelOperations[1])
	// A finished operation's permit stays void while another one is open.
	if err := permit.authorizes(f.authorization.AuthorizationID, kernelOperations[0]); !errors.Is(err, errMutationNotPermitted) {
		t.Fatal("stale permit revived by a later open operation")
	}
	if err := session.finish(context.Background(), permit, operationCompleted); err == nil {
		t.Fatal("stale permit finished the open operation")
	}
	events := f.events(testApplyID)
	if events[len(events)-1] != AuditEventOperationStarted || countKind(events, AuditEventMutationIntent) != 1 {
		t.Fatalf("later operation events %v", events)
	}
	mustFinish(t, session, second, operationCompleted)
	result := session.close(context.Background(), terminalClosure{VerifiedComplete: true})
	if result.Status != ApplyStatusApplied {
		t.Fatalf("result %+v", result)
	}
	if err := second.authorizes(f.authorization.AuthorizationID, kernelOperations[1]); !errors.Is(err, errMutationNotPermitted) {
		t.Fatal("permit valid after the session closed")
	}
	if _, err := session.begin(context.Background(), kernelOperations[1]); err == nil {
		t.Fatal("closed session began an operation")
	}
}

func TestTestMutationPermitIsControlled(t *testing.T) {
	permit := newTestMutationPermit(testAuthorizationID, operationID(0))
	if err := permit.authorizes(testAuthorizationID, operationID(0)); err != nil {
		t.Fatal(err)
	}
	if err := permit.authorizes(testAuthorizationID, operationID(1)); err == nil {
		t.Fatal("test permit authorizes another operation")
	}
}

// Only safety.go may build a permit in production code; every other file
// of the package, and every other package, can only receive one.
func TestMutationPermitHasOneProductionConstructor(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") || file == "safety.go" {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "MutationPermit{") {
			t.Errorf("%s constructs a MutationPermit", file)
		}
	}
	permitType := reflect.TypeOf(MutationPermit{})
	for i := 0; i < permitType.NumField(); i++ {
		if permitType.Field(i).IsExported() {
			t.Errorf("MutationPermit exports field %s", permitType.Field(i).Name)
		}
	}
}

// A refusal before intent leaves the authorization usable: the claim is
// released to issued, the lock is free and the refusal is recorded.
func TestSafetyKernelAbandonBeforeIntent(t *testing.T) {
	f := newKernelFixture(t)
	session := f.claim(testApplyID)
	result := session.abandon(context.Background(), ReasonPlanStale)
	if result.Status != ApplyStatusRefused || result.Reason != ReasonPlanStale || result.DurableAuthorizationState != AuthorizationStateIssued ||
		result.PriorAuthorizationState != AuthorizationStateIssued || result.ExitCode != 1 || result.RecoveryRequired {
		t.Fatalf("abandon result %+v", result)
	}
	if durable := f.durable(); durable.State != AuthorizationStateIssued || durable.OwnerApplyID != "" {
		t.Fatalf("durable %+v", durable)
	}
	if !f.lockFree() || len(f.recordedResult(testApplyID)) == 0 {
		t.Fatal("abandon left the lock held or the refusal unrecorded")
	}
	// The released authorization can be claimed by a later attempt.
	f.claim(testOtherApplyID)
}

// Refusals the kernel decides under the lock are recorded and release it.
func TestSafetyKernelClaimRefusals(t *testing.T) {
	ctx := context.Background()
	t.Run("lock held", func(t *testing.T) {
		f := newKernelFixture(t)
		lock, err := f.store.AcquireApplyLock(ctx, f.project, testOtherApplyID)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Release(ctx)
		_, refusal := f.kernel.claim(ctx, f.request(testApplyID))
		if refusal == nil || refusal.Reason != ReasonLockConflict || refusal.AuthorizationID != "" {
			t.Fatalf("refusal %+v", refusal)
		}
		if durable := f.durable(); durable.State != AuthorizationStateIssued {
			t.Fatalf("lock conflict changed authorization %+v", durable)
		}
	})
	for name, test := range map[string]struct {
		mutate func(*claimRequest, *kernelFixture)
		reason Reason
	}{
		"missing":       {func(r *claimRequest, _ *kernelFixture) { r.authorization = "123e4567-e89b-42d3-a456-426614174005" }, ReasonAuthorizationMissing},
		"plan mismatch": {func(r *claimRequest, _ *kernelFixture) { r.planDigest = testOtherSHA256 }, ReasonAuthorizationMismatch},
		"expired": {func(_ *claimRequest, f *kernelFixture) {
			f.kernel.clock = fixedClock{time.Unix(3600, 0).UTC()}
		}, ReasonAuthorizationExpired},
	} {
		t.Run(name, func(t *testing.T) {
			f := newKernelFixture(t)
			request := f.request(testApplyID)
			test.mutate(&request, f)
			_, refusal := f.kernel.claim(ctx, request)
			if refusal == nil || refusal.Status != ApplyStatusRefused || refusal.Reason != test.reason || refusal.ExitCode != 1 {
				t.Fatalf("refusal %+v", refusal)
			}
			if durable := f.durable(); durable.State != AuthorizationStateIssued || durable.StateVersion != 1 {
				t.Fatalf("refusal changed authorization %+v", durable)
			}
			if !f.lockFree() || len(f.recordedResult(testApplyID)) == 0 {
				t.Fatal("refusal left the lock held or was not recorded")
			}
		})
	}
}

func countKind(events []AuditEventKind, kind AuditEventKind) int {
	n := 0
	for _, event := range events {
		if event == kind {
			n++
		}
	}
	return n
}
