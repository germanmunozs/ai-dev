//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"testing"
	"time"
)

func approvalTuple(file AuthorizedFile) string {
	return file.PathUTF8 + "|" + file.Mode + "|" + string(file.ContentSHA256) + "|blob:" + string(file.BlobOID)
}

// The approval set must equal the allowlist exactly, in the matching
// normal or ignored form (F008-FR-159–161).
func TestCheckApprovals(t *testing.T) {
	in := validIntentInput(t)
	ignored := validAuthorizedFile("build/out.txt")
	ignored.Ignored, ignored.ApprovalKind = true, ApprovalKindIgnored
	in.AuthorizedFiles = append(in.AuthorizedFiles, ignored)
	intent, err := NewImmutableIntent(in)
	if err != nil {
		t.Fatal(err)
	}
	config := validAuthorizedFile(ProjectConfigPath)
	good := IssueRequest{AuthorizedBy: "op", ConfirmInitialCommit: true, Approvals: []string{approvalTuple(config)}, IgnoredApprovals: []string{approvalTuple(ignored)}}
	if rejection := checkApprovals(intent, good); rejection != nil {
		t.Fatalf("exact approvals refused: %+v", rejection)
	}
	wrongMode := config
	wrongMode.Mode = "100755"
	for name, test := range map[string]struct {
		req    IssueRequest
		reason Reason
	}{
		"no confirmation":       {IssueRequest{Approvals: good.Approvals, IgnoredApprovals: good.IgnoredApprovals}, ReasonAuthorizationInputIncomplete},
		"missing approval":      {IssueRequest{ConfirmInitialCommit: true, Approvals: good.Approvals}, ReasonAuthorizationInputIncomplete},
		"ignored as normal":     {IssueRequest{ConfirmInitialCommit: true, Approvals: []string{approvalTuple(config), approvalTuple(ignored)}}, ReasonAuthorizationMismatch},
		"normal as ignored":     {IssueRequest{ConfirmInitialCommit: true, IgnoredApprovals: []string{approvalTuple(config), approvalTuple(ignored)}}, ReasonAuthorizationMismatch},
		"duplicate":             {IssueRequest{ConfirmInitialCommit: true, Approvals: []string{approvalTuple(config), approvalTuple(config)}, IgnoredApprovals: good.IgnoredApprovals}, ReasonAuthorizationMismatch},
		"extra file":            {IssueRequest{ConfirmInitialCommit: true, Approvals: []string{approvalTuple(config), approvalTuple(validAuthorizedFile("extra.txt"))}, IgnoredApprovals: good.IgnoredApprovals}, ReasonAuthorizationMismatch},
		"mismatched mode":       {IssueRequest{ConfirmInitialCommit: true, Approvals: []string{approvalTuple(wrongMode)}, IgnoredApprovals: good.IgnoredApprovals}, ReasonAuthorizationMismatch},
		"malformed tuple":       {IssueRequest{ConfirmInitialCommit: true, Approvals: []string{"a|b"}, IgnoredApprovals: good.IgnoredApprovals}, ReasonAuthorizationMismatch},
		"path with extra field": {IssueRequest{ConfirmInitialCommit: true, Approvals: []string{"a|b|" + approvalTuple(config)}, IgnoredApprovals: good.IgnoredApprovals}, ReasonAuthorizationMismatch},
	} {
		if rejection := checkApprovals(intent, test.req); rejection == nil || rejection.Reason != test.reason {
			t.Errorf("%s: %+v, want %s", name, rejection, test.reason)
		}
	}
}

// Every scope member binds independently, beyond the plan digest the
// kernel also compares.
func TestCheckScope(t *testing.T) {
	f := newPlanFixture(t)
	plan := mustPlan(t, f, gitFlowRequest("avh")).Plan
	authorization, rejection := newAuthorization(IssueRequest{AuthorizationID: issuedID, AuthorizedBy: "op"}, plan, time.Unix(500, 0))
	if rejection != nil {
		t.Fatal(rejection)
	}
	if rejection := checkScope(authorization, plan); rejection != nil {
		t.Fatalf("own scope refused: %+v", rejection)
	}
	base := GitAuthorizationInput{
		SchemaVersion: GitAuthorizationSchema, AuthorizationID: issuedID, PlanDigest: plan.PlanDigest,
		ProjectIdentity: authorization.ProjectIdentity, AuthorizedOperationIDs: authorization.AuthorizedOperationIDs(),
		Strategy: authorization.Strategy, GitFlowDialect: authorization.GitFlowDialect, AuthorizedRefs: authorization.AuthorizedRefs(),
		AuthorizedConfigKeys: authorization.AuthorizedConfigKeys(), DeclaredOperator: "op", IssuedAt: time.Unix(500, 0).UTC(),
		State: AuthorizationStateIssued, StateVersion: 1,
	}
	for name, mutate := range map[string]func(*GitAuthorizationInput){
		"operations":  func(in *GitAuthorizationInput) { in.AuthorizedOperationIDs = in.AuthorizedOperationIDs[1:] },
		"config keys": func(in *GitAuthorizationInput) { in.AuthorizedConfigKeys = in.AuthorizedConfigKeys[1:] },
		"refs":        func(in *GitAuthorizationInput) { in.AuthorizedRefs = nil },
		"dialect":     func(in *GitAuthorizationInput) { in.GitFlowDialect = GitFlowDialectNext },
		"digest":      func(in *GitAuthorizationInput) { in.PlanDigest = testOtherSHA256 },
	} {
		in := base
		mutate(&in)
		other, err := NewGitAuthorization(in)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if rejection := checkScope(other, plan); rejection == nil || rejection.Reason != ReasonAuthorizationMismatch {
			t.Errorf("%s: %+v", name, rejection)
		}
	}
}
