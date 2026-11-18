//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"context"
	"testing"
	"time"
)

func developmentBaseRequest() StrategyRequest {
	req := gitFlowRequest("avh")
	req.InitializeRepository, req.DevelopmentBaseOID = false, "sha1:"+testCommitOID
	return req
}

// T092/AC-031: the development CAS is its own authorized operation: its ID
// is in the authorization scope and only its permit runs it
// (F008-FR-046, 192).
func TestDevelopmentCASAuthorization(t *testing.T) {
	f := newPlanFixture(t)
	f.existingRepository()
	plan := mustPlan(t, f, developmentBaseRequest()).Plan
	operations := plan.DerivedOperations()
	cas := operations[len(operations)-1]
	authorization, rejection := newAuthorization(IssueRequest{AuthorizationID: issuedID, AuthorizedBy: "op"}, plan, time.Unix(1000, 0))
	if rejection != nil || !containsValue(authorization.AuthorizedOperationIDs(), cas.OperationID) {
		t.Fatalf("scope %v %+v", authorization.AuthorizedOperationIDs(), rejection)
	}
	argv := cas.ExactArgv()
	if argv[len(argv)-1] != "" || argv[len(argv)-2] != testCommitOID {
		t.Fatalf("CAS is not target-absent at the base: %q", argv)
	}
}

// T092/AC-031: production drift after planning is plan_stale before any
// mutation; a development ref that appears after the immediate
// revalidation of the CAS is concurrent_change (c-01).
func TestDevelopmentBaseDriftAndRace(t *testing.T) {
	planned := newPlanFixture(t)
	planned.existingRepository()
	f := &applyFixture{planFixture: planned, store: newFakeGitStore(), now: time.Unix(1000, 0).UTC(),
		plan: mustPlan(t, planned, developmentBaseRequest()).Plan}
	f.executor = &fakeExecutor{t: t, authorized: issuedID, evidence: ApplyEvidence{VerifiedComplete: true}}
	data, err := EncodePlanFile(f.plan)
	if err != nil {
		t.Fatal(err)
	}
	f.file = data
	f.git.inner.results[GitReadShowRefs] = GitRunResult{Stdout: []byte(testOtherSHA1OID + " refs/heads/main\n")}
	expectResult(t, f.issue(operator()), ApplyStatusRefused, ReasonPlanStale, 1)
	if len(f.executor.executed) != 0 {
		t.Fatalf("executed %v", f.executor.executed)
	}

	d := &dispatcherFixture{planFixture: newPlanFixture(t), runner: &scriptedRunner{}}
	d.planFixture.existingRepository()
	d.plan = mustPlan(t, d.planFixture, developmentBaseRequest()).Plan
	d.d = NewOperationDispatcher(DispatcherDependencies{Runner: d.runner, Commands: d.planFixture.deps.Commands,
		Inspect: func(context.Context, []byte, [][]byte) (GitInspection, error) { return d.planFixture.inspect(), nil }})
	operations := d.plan.DerivedOperations()
	cas := operations[len(operations)-1]
	d.git.inner.results[GitReadShowRefs] = GitRunResult{Stdout: []byte(testCommitOID + " refs/heads/develop\n" + testCommitOID + " refs/heads/main\n")}
	if rejection := d.d.Revalidate(context.Background(), d.plan, cas); rejection == nil || rejection.Reason != ReasonConcurrentChange {
		t.Fatalf("revalidation with develop present: %+v", rejection)
	}
}
