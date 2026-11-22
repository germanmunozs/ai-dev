//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

// raceExecutor completes every operation and counts executions; it is safe
// for concurrent attempts.
type raceExecutor struct {
	executed atomic.Int64
}

func (e *raceExecutor) Preflight(context.Context, GitInitializationPlan) *PlanRejection { return nil }
func (e *raceExecutor) Revalidate(context.Context, GitInitializationPlan, DerivedOperation) *PlanRejection {
	return nil
}
func (e *raceExecutor) Execute(context.Context, MutationPermit, GitInitializationPlan, DerivedOperation) OperationOutcome {
	e.executed.Add(1)
	return OperationCompleted
}
func (e *raceExecutor) Verify(context.Context, GitInitializationPlan) ApplyEvidence {
	return ApplyEvidence{VerifiedComplete: true}
}

func authorizationIDFor(n int) string {
	return fmt.Sprintf("123e4567-e89b-42d3-a456-%012d", 426614176000+n)
}

// T111: run with -race. Concurrent attempts contend for one authorization
// and for several unrelated ones of the same project while the project
// drifts; the claim CAS, the lock, audit publication and the sole terminal
// closer keep each authorization's effects single and its trail ordered.
func TestRaceApplyKernel(t *testing.T) {
	for round := 0; round < 10; round++ {
		f := newApplyFixture(t, basicNewRepo())
		executor := &raceExecutor{}
		deps := f.deps()
		deps.Executor = executor
		apply := func(id string, issue bool) GitApplyResult {
			req := ApplyRequest{PlanFile: f.file, ConfirmPlan: string(f.plan.PlanDigest), AuthorizationID: id}
			if issue {
				req.Issue = &IssueRequest{AuthorizedBy: "operator"}
			}
			return Apply(context.Background(), deps, req)
		}
		var wg sync.WaitGroup
		results := make([]GitApplyResult, 9)
		for i := range results {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				switch {
				case i < 4:
					// Four attempts race to issue and consume one ID.
					results[i] = apply(authorizationIDFor(0), true)
				case i < 8:
					// Four unrelated authorizations of the same plan.
					results[i] = apply(authorizationIDFor(i), true)
				default:
					// Concurrent drift of an unrelated project file.
					mustWrite(t, filepath.Join(f.root, "drift.txt"), "x\n")
					os.Remove(filepath.Join(f.root, "drift.txt"))
				}
			}(i)
		}
		wg.Wait()
		applied := map[AuthorizationID]int{}
		for _, result := range results[:8] {
			if err := result.Validate(); err != nil {
				t.Fatalf("round %d: invalid result %+v", round, result)
			}
			if result.Status == ApplyStatusApplied {
				applied[result.AuthorizationID]++
			}
		}
		closures := map[AuthorizationID]int{}
		for _, event := range f.store.eventsSnapshot() {
			if event.Kind == AuditEventTerminalAuthorizationCAS {
				closures[event.AuthorizationID]++
			}
		}
		for id, count := range closures {
			if count != 1 || applied[id] > 1 {
				t.Fatalf("round %d: authorization %s closed %d times, applied %d times", round, id, count, applied[id])
			}
		}
		if want := int64(len(applied) * len(f.plan.DerivedOperations())); executor.executed.Load() != want {
			t.Fatalf("round %d: %d executions for %d applied authorizations", round, executor.executed.Load(), len(applied))
		}
	}
}
