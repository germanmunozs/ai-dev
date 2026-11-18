//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// reconciliationPlan is a planned basic initialization, its plan file and
// a store holding an authorization issued for it.
type reconciliationPlan struct {
	plan          GitInitializationPlan
	file          []byte
	inspection    GitInspection
	store         *fakeGitStore
	authorization GitAuthorization
}

func newReconciliationPlan(t *testing.T, pf *planFixture, store *fakeGitStore, req StrategyRequest, id AuthorizationID) reconciliationPlan {
	t.Helper()
	plan := mustPlan(t, pf, req).Plan
	file, err := EncodePlanFile(plan)
	if err != nil {
		t.Fatal(err)
	}
	authorization, rejection := newAuthorization(IssueRequest{AuthorizationID: string(id), AuthorizedBy: "operator-label"}, plan, time.Unix(1000, 0))
	if rejection != nil {
		t.Fatalf("authorization: %s", rejection.Detail)
	}
	if err := store.IssueAuthorization(context.Background(), authorization); err != nil {
		t.Fatal(err)
	}
	return reconciliationPlan{plan: plan, file: file, inspection: pf.inspect(), store: store, authorization: authorization}
}

func (r reconciliationPlan) deps() ReconciliationDependencies {
	return ReconciliationDependencies{Store: r.store, Inspect: func(context.Context, []byte, [][]byte) (GitInspection, error) { return r.inspection, nil }}
}

// T126/AC-021: the read-only reconciliation inspection refuses every
// unusable input with its reason, classifies every prior postcondition and
// encodes a report that grants nothing and carries no operator text.
func TestInspectReconciliationReasons(t *testing.T) {
	ctx := context.Background()
	pf, store := newPlanFixture(t), newFakeGitStore()
	r := newReconciliationPlan(t, pf, store, basicNewRepo(), issuedID)
	other := newReconciliationPlan(t, pf, store, newRepo("github-flow", map[string]string{"initial-branch": "main"}), "123e4567-e89b-42d3-a456-426614174034")
	request := ReconciliationRequest{PlanFile: r.file, AuthorizationID: issuedID}
	inspectFails := func(err error) ReconciliationDependencies {
		deps := r.deps()
		deps.Inspect = func(context.Context, []byte, [][]byte) (GitInspection, error) { return GitInspection{}, err }
		return deps
	}
	for name, test := range map[string]struct {
		deps   ReconciliationDependencies
		req    ReconciliationRequest
		fault  string
		reason Reason
	}{
		"no store":          {ReconciliationDependencies{}, request, "", ReasonAuditUnavailable},
		"invalid plan file": {r.deps(), ReconciliationRequest{PlanFile: []byte("{}"), AuthorizationID: issuedID}, "", ReasonPlanInvalid},
		"invalid ID":        {r.deps(), ReconciliationRequest{PlanFile: r.file, AuthorizationID: "x"}, "", ReasonAuthorizationMissing},
		"inspection refused": {inspectFails(&InspectError{Reason: ReasonFilesystemUnsupported, Detail: "tmpfs"}), request, "",
			ReasonFilesystemUnsupported},
		"inspection error":         {inspectFails(errors.New("boom")), request, "", ReasonProjectIdentityUnavailable},
		"missing authorization":    {r.deps(), ReconciliationRequest{PlanFile: r.file, AuthorizationID: "123e4567-e89b-42d3-a456-426614174099"}, "", ReasonAuthorizationMissing},
		"unreadable authorization": {r.deps(), request, "LoadAuthorization", ReasonAuditUnavailable},
		"another plan's authorization": {r.deps(), ReconciliationRequest{PlanFile: r.file, AuthorizationID: string(other.authorization.AuthorizationID)}, "",
			ReasonAuthorizationMismatch},
	} {
		t.Run(name, func(t *testing.T) {
			r.store.faults = faultPlan{}
			if test.fault != "" {
				r.store.faults.failCall(test.fault, 0, errInjected, false)
			}
			if _, rejection := InspectReconciliation(ctx, test.deps, test.req); rejection == nil || rejection.Reason != test.reason {
				t.Fatalf("rejection %+v, want %s", rejection, test.reason)
			}
		})
	}
	r.store.faults = faultPlan{}
	report, rejection := InspectReconciliation(ctx, r.deps(), request)
	if rejection != nil {
		t.Fatalf("inspection %+v", *rejection)
	}
	conditions := r.plan.ExpectedPostconditions()
	if report.Unresolved || report.TrailReadable || len(report.Applied)+len(report.Absent)+len(report.Unverifiable) != len(conditions) || len(conditions) == 0 {
		t.Fatalf("report %+v over %d conditions", report, len(conditions))
	}
	// Nothing is applied yet: the planned repository does not exist.
	if len(report.Absent) == 0 {
		t.Fatalf("no postcondition is absent in %+v", report)
	}
	encoded, err := EncodeReconciliationReport(report)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := EncodeReconciliationReport(report)
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil || !bytes.Equal(encoded, again) {
		t.Fatalf("report %s: %v", encoded, err)
	}
	if decoded["schema_version"] != GitReconciliationReportSchema || decoded["grants_mutation_authority"] != false ||
		decoded["unresolved"] != false || strings.Contains(string(encoded), "operator-label") {
		t.Fatalf("encoded report %s", encoded)
	}
	if _, ok := decoded["prior_authorization"].(map[string]any)["owner_apply_id"]; ok {
		t.Fatal("an unowned authorization reports an owner")
	}
}
