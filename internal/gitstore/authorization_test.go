//go:build (darwin && !ios) || (linux && !android)

package gitstore

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/germanmunozs/ai-dev/internal/gitmanager"
)

const (
	testDigest                                     = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	testOtherDigest                                = "sha256:fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
	testAuthorizationID gitmanager.AuthorizationID = "123e4567-e89b-42d3-a456-426614174000"
	testApplyID         gitmanager.ApplyID         = "123e4567-e89b-42d3-a456-426614174001"
	testOtherApplyID    gitmanager.ApplyID         = "123e4567-e89b-42d3-a456-426614174009"
)

// openTestStore opens a store beneath a private temporary state root.
func openTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	root, path := stateRootPath(t)
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store, path
}

func testProject(t *testing.T, path string) gitmanager.PhysicalProjectIdentity {
	t.Helper()
	obs, err := ObserveFilesystem(path)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := gitmanager.NewPhysicalProjectIdentity(path, []byte(path), obs.DeviceDecimal, obs.InodeDecimal)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func testAuthorization(t *testing.T, project gitmanager.PhysicalProjectIdentity) gitmanager.GitAuthorization {
	t.Helper()
	authorization, err := gitmanager.NewGitAuthorization(gitmanager.GitAuthorizationInput{
		SchemaVersion: gitmanager.GitAuthorizationSchema, AuthorizationID: testAuthorizationID, PlanDigest: testDigest,
		ProjectIdentity: project, AuthorizedOperationIDs: []gitmanager.OperationID{"0:" + testDigest, "1:" + testOtherDigest},
		Strategy: gitmanager.StrategyBasic, AuthorizedRefs: []gitmanager.ObservedRef{gitmanager.NewObservedRef([]byte("refs/heads/main"))},
		DeclaredOperator: "operator", IssuedAt: time.Unix(10, 0).UTC(), ExpiresAt: time.Unix(3610, 0).UTC(),
		State: gitmanager.AuthorizationStateIssued, StateVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return authorization
}

func encoded(t *testing.T, authorization gitmanager.GitAuthorization) []byte {
	t.Helper()
	data, err := gitmanager.EncodeAuthorizationRecord(authorization)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func transition(authorization gitmanager.GitAuthorization, to gitmanager.AuthorizationState, owner gitmanager.ApplyID) gitmanager.AuthorizationTransitionRequest {
	return gitmanager.AuthorizationTransitionRequest{
		ProjectIdentityDigest: authorization.ProjectIdentity.Digest(), AuthorizationID: authorization.AuthorizationID,
		ExpectedState: authorization.State, ExpectedVersion: authorization.StateVersion, ExpectedOwner: authorization.OwnerApplyID,
		To: to, Owner: owner,
	}
}

func authorizationFile(path string, authorization gitmanager.GitAuthorization) string {
	return filepath.Join(path, "projects", strings.TrimPrefix(string(authorization.ProjectIdentity.Digest()), "sha256:"),
		"authorizations", string(authorization.AuthorizationID)+".json")
}

func TestIssueAndLoadAuthorization(t *testing.T) {
	ctx := context.Background()
	store, path := openTestStore(t)
	authorization := testAuthorization(t, testProject(t, privateTempDir(t)))
	if _, err := store.LoadAuthorization(ctx, authorization.ProjectIdentity.Digest(), testAuthorizationID); !errors.Is(err, gitmanager.ErrAuthorizationMissing) {
		t.Fatalf("load before issue: %v", err)
	}
	if err := store.IssueAuthorization(ctx, authorization); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(authorizationFile(path, authorization))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("authorization record: %v %v", info, err)
	}
	loaded, err := store.LoadAuthorization(ctx, authorization.ProjectIdentity.Digest(), testAuthorizationID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded(t, loaded), encoded(t, authorization)) {
		t.Fatal("loaded authorization differs from the issued one")
	}
	// The ID is never overwritten, merged or widened.
	wider := testAuthorization(t, authorization.ProjectIdentity)
	wider.DeclaredOperator = "someone-else"
	if err := store.IssueAuthorization(ctx, wider); !errors.Is(err, gitmanager.ErrAuthorizationExists) {
		t.Fatalf("second issue: %v", err)
	}
	if data, _ := os.ReadFile(authorizationFile(path, authorization)); !bytes.Equal(data, encoded(t, authorization)) {
		t.Fatal("existing authorization was rewritten")
	}
	// Records are partitioned by full project identity.
	if _, err := store.LoadAuthorization(ctx, testDigest, testAuthorizationID); !errors.Is(err, gitmanager.ErrAuthorizationMissing) {
		t.Fatalf("load from another partition: %v", err)
	}
	// Only a fresh issued record can be issued.
	claimed, err := authorization.Transition(transition(authorization, gitmanager.AuthorizationStateClaimed, testApplyID))
	if err != nil {
		t.Fatal(err)
	}
	claimed.AuthorizationID = "123e4567-e89b-42d3-a456-426614174005"
	if err := store.IssueAuthorization(ctx, claimed); err == nil {
		t.Fatal("claimed authorization issued")
	}
}

func TestAuthorizationLifecycleCAS(t *testing.T) {
	ctx := context.Background()
	store, path := openTestStore(t)
	current := testAuthorization(t, testProject(t, privateTempDir(t)))
	if err := store.IssueAuthorization(ctx, current); err != nil {
		t.Fatal(err)
	}
	step := func(to gitmanager.AuthorizationState, owner gitmanager.ApplyID, digest gitmanager.ContentDigest) {
		t.Helper()
		request := transition(current, to, owner)
		request.TerminalResultDigest = digest
		next, err := store.TransitionAuthorization(ctx, request)
		if err != nil {
			t.Fatalf("%s -> %s: %v", current.State, to, err)
		}
		loaded, err := store.LoadAuthorization(ctx, current.ProjectIdentity.Digest(), current.AuthorizationID)
		if err != nil || !bytes.Equal(encoded(t, loaded), encoded(t, next)) || next.StateVersion != current.StateVersion+1 {
			t.Fatalf("durable %+v, returned %+v, %v", loaded, next, err)
		}
		current = next
	}
	step(gitmanager.AuthorizationStateClaimed, testApplyID, "")
	// A clean pre-mutation release returns the authority unowned.
	step(gitmanager.AuthorizationStateIssued, testApplyID, "")
	if current.OwnerApplyID != "" {
		t.Fatalf("released authorization keeps owner %q", current.OwnerApplyID)
	}
	step(gitmanager.AuthorizationStateClaimed, testApplyID, "")
	// A proven orphan claim moves to a new owner.
	step(gitmanager.AuthorizationStateClaimed, testOtherApplyID, "")
	step(gitmanager.AuthorizationStateMutationStarted, testOtherApplyID, "")
	step(gitmanager.AuthorizationStateApplied, testOtherApplyID, testOtherDigest)
	if current.TerminalResultDigest != testOtherDigest || len(current.TransitionHistory()) != 6 {
		t.Fatalf("terminal authorization %+v", current)
	}

	// Terminal states never transition and the record is untouched.
	before, _ := os.ReadFile(authorizationFile(path, current))
	for _, to := range []gitmanager.AuthorizationState{gitmanager.AuthorizationStateFailed, gitmanager.AuthorizationStateClaimed, gitmanager.AuthorizationStateIssued} {
		if _, err := store.TransitionAuthorization(ctx, transition(current, to, testOtherApplyID)); err == nil {
			t.Fatalf("terminal authorization moved to %s", to)
		}
	}
	if after, _ := os.ReadFile(authorizationFile(path, current)); !bytes.Equal(before, after) {
		t.Fatal("terminal authorization record changed")
	}
}

// Each compared member of the CAS tuple independently refuses the swap.
func TestAuthorizationCASMismatches(t *testing.T) {
	ctx := context.Background()
	store, path := openTestStore(t)
	issued := testAuthorization(t, testProject(t, privateTempDir(t)))
	if err := store.IssueAuthorization(ctx, issued); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.TransitionAuthorization(ctx, transition(issued, gitmanager.AuthorizationStateClaimed, testApplyID))
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(authorizationFile(path, claimed))
	start := transition(claimed, gitmanager.AuthorizationStateMutationStarted, testApplyID)
	for name, mutate := range map[string]func(*gitmanager.AuthorizationTransitionRequest){
		"stale version":  func(r *gitmanager.AuthorizationTransitionRequest) { r.ExpectedVersion = 1 },
		"future version": func(r *gitmanager.AuthorizationTransitionRequest) { r.ExpectedVersion = 3 },
		"other owner": func(r *gitmanager.AuthorizationTransitionRequest) {
			r.ExpectedOwner, r.Owner = testOtherApplyID, testOtherApplyID
		},
		"stale state": func(r *gitmanager.AuthorizationTransitionRequest) {
			*r = transition(issued, gitmanager.AuthorizationStateClaimed, testOtherApplyID)
		},
	} {
		request := start
		mutate(&request)
		if _, err := store.TransitionAuthorization(ctx, request); !errors.Is(err, gitmanager.ErrAuthorizationConflict) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// Forbidden transitions are refused before the store is touched.
	for name, request := range map[string]gitmanager.AuthorizationTransitionRequest{
		"issued jump": transition(issued, gitmanager.AuthorizationStateMutationStarted, testApplyID),
		"owner change on start": func() gitmanager.AuthorizationTransitionRequest {
			r := start
			r.Owner = testOtherApplyID
			return r
		}(),
		"premature digest": func() gitmanager.AuthorizationTransitionRequest {
			r := start
			r.TerminalResultDigest = testDigest
			return r
		}(),
	} {
		if _, err := store.TransitionAuthorization(ctx, request); err == nil || errors.Is(err, gitmanager.ErrAuthorizationConflict) {
			t.Errorf("%s: %v", name, err)
		}
	}
	missing := start
	missing.AuthorizationID = "123e4567-e89b-42d3-a456-426614174005"
	if _, err := store.TransitionAuthorization(ctx, missing); !errors.Is(err, gitmanager.ErrAuthorizationMissing) {
		t.Errorf("missing: %v", err)
	}
	if after, _ := os.ReadFile(authorizationFile(path, claimed)); !bytes.Equal(before, after) {
		t.Fatal("refused transition changed the record")
	}
}

// Many attempts racing for the same issued tuple: exactly one claims it.
func TestConcurrentClaimsHaveOneWinner(t *testing.T) {
	ctx := context.Background()
	store, _ := openTestStore(t)
	issued := testAuthorization(t, testProject(t, privateTempDir(t)))
	if err := store.IssueAuthorization(ctx, issued); err != nil {
		t.Fatal(err)
	}
	owners := []gitmanager.ApplyID{
		"123e4567-e89b-42d3-a456-426614174011", "123e4567-e89b-42d3-a456-426614174012", "123e4567-e89b-42d3-a456-426614174013",
		"123e4567-e89b-42d3-a456-426614174014", "123e4567-e89b-42d3-a456-426614174015", "123e4567-e89b-42d3-a456-426614174016",
		"123e4567-e89b-42d3-a456-426614174017", "123e4567-e89b-42d3-a456-426614174018",
	}
	var mu sync.Mutex
	var winners []gitmanager.ApplyID
	var wait sync.WaitGroup
	for _, owner := range owners {
		wait.Add(1)
		go func(owner gitmanager.ApplyID) {
			defer wait.Done()
			_, err := store.TransitionAuthorization(ctx, transition(issued, gitmanager.AuthorizationStateClaimed, owner))
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				winners = append(winners, owner)
			} else if !errors.Is(err, gitmanager.ErrAuthorizationConflict) {
				t.Errorf("%s: %v", owner, err)
			}
		}(owner)
	}
	wait.Wait()
	loaded, err := store.LoadAuthorization(ctx, issued.ProjectIdentity.Digest(), issued.AuthorizationID)
	if len(winners) != 1 || err != nil || loaded.OwnerApplyID != winners[0] || loaded.StateVersion != 2 {
		t.Fatalf("winners %v, durable %+v, %v", winners, loaded, err)
	}
}

func TestAuthorizationDurabilityFaults(t *testing.T) {
	ctx := context.Background()
	for _, step := range []string{"write", "sync-file", "link"} {
		store, path := openTestStore(t)
		authorization := testAuthorization(t, testProject(t, privateTempDir(t)))
		store.records.ops = failAt(store.records.ops, step)
		if err := store.IssueAuthorization(ctx, authorization); !errors.Is(err, errInjectedFault) {
			t.Fatalf("issue with %s fault: %v", step, err)
		}
		if _, err := os.Lstat(authorizationFile(path, authorization)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s fault published a record", step)
		}
	}
	store, _ := openTestStore(t)
	authorization := testAuthorization(t, testProject(t, privateTempDir(t)))
	if err := store.IssueAuthorization(ctx, authorization); err != nil {
		t.Fatal(err)
	}
	store.records.ops = failAt(store.records.ops, "rename")
	if _, err := store.TransitionAuthorization(ctx, transition(authorization, gitmanager.AuthorizationStateClaimed, testApplyID)); !errors.Is(err, errInjectedFault) {
		t.Fatalf("rename fault: %v", err)
	}
	if loaded, err := store.LoadAuthorization(ctx, authorization.ProjectIdentity.Digest(), authorization.AuthorizationID); err != nil || loaded.State != gitmanager.AuthorizationStateIssued {
		t.Fatalf("proven failure changed the record: %+v %v", loaded, err)
	}
	store.records.ops = failAt(defaultRecordOps(), "sync-directory")
	if _, err := store.TransitionAuthorization(ctx, transition(authorization, gitmanager.AuthorizationStateClaimed, testApplyID)); !errors.Is(err, gitmanager.ErrStoreOutcomeUnverifiable) {
		t.Fatalf("directory sync fault: %v", err)
	}
}

// A record that the encoder could not have written, or that sits under the
// wrong name or partition, is refused rather than trusted.
func TestCorruptAuthorizationRecordsAreRefused(t *testing.T) {
	ctx := context.Background()
	store, path := openTestStore(t)
	authorization := testAuthorization(t, testProject(t, privateTempDir(t)))
	if err := store.IssueAuthorization(ctx, authorization); err != nil {
		t.Fatal(err)
	}
	file := authorizationFile(path, authorization)
	original, _ := os.ReadFile(file)
	other := testAuthorization(t, testProject(t, privateTempDir(t)))
	for name, data := range map[string][]byte{
		"truncated":         original[:len(original)/2],
		"non-canonical":     append([]byte(" "), original...),
		"foreign partition": encoded(t, other),
	} {
		if err := os.WriteFile(file, data, 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := store.LoadAuthorization(ctx, authorization.ProjectIdentity.Digest(), authorization.AuthorizationID)
		if err == nil || errors.Is(err, gitmanager.ErrAuthorizationMissing) {
			t.Errorf("%s: %v", name, err)
		}
		if _, err := store.TransitionAuthorization(ctx, transition(authorization, gitmanager.AuthorizationStateClaimed, testApplyID)); err == nil {
			t.Errorf("%s: transitioned", name)
		}
	}
}
