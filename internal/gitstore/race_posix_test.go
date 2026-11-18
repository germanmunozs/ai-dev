//go:build (darwin && !ios) || (linux && !android)

package gitstore

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"ai-dev/internal/gitmanager"
)

func applyID(n int) gitmanager.ApplyID {
	return gitmanager.ApplyID(fmt.Sprintf("123e4567-e89b-42d3-a456-%012d", 426614175000+n))
}

// T111: many writers race one authorization CAS; exactly one wins, every
// other attempt is a proven conflict, and the record names the winner.
func TestRaceAuthorizationCAS(t *testing.T) {
	ctx := context.Background()
	for round := 0; round < 20; round++ {
		store, _ := openTestStore(t)
		authorization := testAuthorization(t, testProject(t, privateTempDir(t)))
		if err := store.IssueAuthorization(ctx, authorization); err != nil {
			t.Fatal(err)
		}
		var wins atomic.Int64
		var winner atomic.Value
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, err := store.TransitionAuthorization(ctx, transition(authorization, gitmanager.AuthorizationStateClaimed, applyID(i)))
				switch {
				case err == nil:
					wins.Add(1)
					winner.Store(applyID(i))
				case !errors.Is(err, gitmanager.ErrAuthorizationConflict):
					t.Errorf("writer %d: %v", i, err)
				}
			}(i)
		}
		wg.Wait()
		final, err := store.LoadAuthorization(ctx, authorization.ProjectIdentity.Digest(), authorization.AuthorizationID)
		if err != nil || wins.Load() != 1 || final.OwnerApplyID != winner.Load() || final.StateVersion != 2 {
			t.Fatalf("round %d: %d winners, final %+v %v", round, wins.Load(), final, err)
		}
	}
}

// T111: the persistent apply lock is reused under contention: at no time
// do two attempts hold it, and it stays acquirable after every release.
func TestRaceApplyLockReuse(t *testing.T) {
	ctx := context.Background()
	store, _ := openTestStore(t)
	project := testProject(t, privateTempDir(t))
	var holders, acquired atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for n := 0; n < 25; n++ {
				lock, err := store.AcquireApplyLock(ctx, project, applyID(i*100+n))
				if err != nil {
					if !errors.Is(err, gitmanager.ErrApplyLockHeld) {
						t.Errorf("acquire: %v", err)
					}
					continue
				}
				if holders.Add(1) != 1 {
					t.Error("two attempts hold the apply lock")
				}
				acquired.Add(1)
				holders.Add(-1)
				if err := lock.Release(ctx); err != nil {
					t.Errorf("release: %v", err)
				}
			}
		}(i)
	}
	wg.Wait()
	if acquired.Load() == 0 {
		t.Fatal("the lock was never acquired")
	}
	mustAcquire(t, store, project, applyID(9999)).Release(ctx)
}

// T111: audit publication is no-replace under concurrency: racing writers
// of one sequence number publish exactly one event, and concurrent
// attempts keep independent, complete trails.
func TestRaceAuditPublication(t *testing.T) {
	ctx := context.Background()
	store, _ := openTestStore(t)
	project := testProject(t, privateTempDir(t))
	var wg sync.WaitGroup
	for a := 0; a < 4; a++ {
		attempt := testAttempt(t, project)
		attempt.ApplyID = applyID(a)
		if err := store.CreateAttempt(ctx, attempt); err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func(id gitmanager.ApplyID) {
			defer wg.Done()
			var published atomic.Int64
			var inner sync.WaitGroup
			for w := 0; w < 4; w++ {
				inner.Add(1)
				go func() {
					defer inner.Done()
					event := auditEvent(project, 1, gitmanager.AuditEventAuthorizationIssued, "")
					event.ApplyID = id
					if _, err := store.AppendAuditEvent(ctx, event); err == nil {
						published.Add(1)
					}
				}()
			}
			inner.Wait()
			trail, err := store.ListAuditEvents(ctx, id)
			if err != nil || published.Load() != 1 || len(trail.Events) != 1 || trail.CrashEvidence {
				t.Errorf("attempt %s: %d published, trail %+v %v", id, published.Load(), trail, err)
			}
		}(attempt.ApplyID)
	}
	wg.Wait()
}
