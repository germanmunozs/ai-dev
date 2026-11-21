package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/gitmanager"
)

func reconciliationFixture() gitmanager.ReconciliationReport {
	return gitmanager.ReconciliationReport{
		Prior: gitmanager.GitAuthorization{
			AuthorizationID: "123e4567-e89b-42d3-a456-426614174300", State: gitmanager.AuthorizationStateMutationStarted,
			StateVersion: 4, PlanDigest: gitmanager.PlanDigest("sha256:" + strings.Repeat("b", 64)), OwnerApplyID: "apply-1",
		},
		Unresolved:    true,
		TrailReadable: true,
		Trail: gitmanager.AuditTrail{
			Events:        []gitmanager.AuditEventKind{gitmanager.AuditEventAttemptCreated, gitmanager.AuditEventMutationIntent, gitmanager.AuditEventOperationStarted},
			CrashEvidence: true,
		},
		Applied:      []gitmanager.Condition{{Kind: gitmanager.ConditionRepositoryKind, Expected: "normal"}},
		Absent:       []gitmanager.Condition{{Kind: gitmanager.ConditionPathAbsent, Subject: gitmanager.EncodeReversiblePath([]byte(".ai-dev/config.json"))}},
		Unverifiable: []gitmanager.Condition{{Kind: gitmanager.ConditionIndexEmpty}},
	}
}

// An unresolved attempt names the only way to continue; the report still
// grants nothing.
func TestGitReconciliationTextUnresolved(t *testing.T) {
	var out bytes.Buffer
	if err := WriteGitReconciliationText(&out, reconciliationFixture()); err != nil {
		t.Fatal(err)
	}
	want := "authorization:  123e4567-e89b-42d3-a456-426614174300 (mutation_started, version 4)\n" +
		"plan digest:    sha256:" + strings.Repeat("b", 64) + "\n" +
		"apply attempt:  apply-1\n" +
		"unresolved:     true\n" +
		"audit trail:    attempt_created, mutation_intent, operation_started\n" +
		"crash evidence: unpublished temporaries are present\n" +
		"applied:        repository_kind normal\n" +
		"absent:         path_absent .ai-dev/config.json\n" +
		"unverifiable:   index_empty\n" +
		"continue:       plan again and apply with --issue-authorization --reconciles-authorization 123e4567-e89b-42d3-a456-426614174300\n" +
		"authority:      none; this report grants no mutation authority\n"
	if out.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestGitReconciliationTextUnreadableTrail(t *testing.T) {
	r := reconciliationFixture()
	r.TrailReadable, r.Trail, r.Prior.OwnerApplyID = false, gitmanager.AuditTrail{}, ""
	var out bytes.Buffer
	if err := WriteGitReconciliationText(&out, r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "audit trail:    unreadable\n") || strings.Contains(out.String(), "crash evidence:") ||
		strings.Contains(out.String(), "apply attempt:") {
		t.Fatalf("text:\n%s", out.String())
	}
}

// A finding names only the path and the category; a path that is not
// UTF-8 keeps its reversible encoding, and the screening limitation is
// stated only when there are findings.
func TestGitPlanRejectionText(t *testing.T) {
	rejection := &gitmanager.PlanRejection{Reason: gitmanager.ReasonSensitiveContentSuspected, Detail: "the planned commit contains a credential",
		Findings: []gitmanager.SecurityFinding{
			{Path: gitmanager.NewObservedPath([]byte("notes.txt")), Category: "url_credentials"},
			{Path: gitmanager.NewObservedPath([]byte{'k', 0xff}), Category: "private_key"},
		}}
	var out bytes.Buffer
	if err := WriteGitPlanRejection(&out, "text", rejection, nil); err != nil {
		t.Fatal(err)
	}
	want := "refused: " + string(gitmanager.ReasonSensitiveContentSuspected) + ": the planned commit contains a credential\n" +
		"finding:        notes.txt url_credentials\n" +
		"finding:        base64url:" + gitmanager.EncodeReversiblePath([]byte{'k', 0xff}) + " private_key\n" +
		"screening:      " + gitmanager.SecretScreeningLimitation + "\n"
	if out.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out.String(), want)
	}

	out.Reset()
	if err := WriteGitPlanRejection(&out, "text", &gitmanager.PlanRejection{Reason: gitmanager.ReasonPlanInvalid, Detail: "x"}, nil); err != nil ||
		out.String() != "refused: "+string(gitmanager.ReasonPlanInvalid)+": x\n" {
		t.Fatalf("got %q %v", out.String(), err)
	}
}

func TestDisplayRefKeepsReversibleEncoding(t *testing.T) {
	if got := displayRef(gitmanager.NewObservedRef([]byte("refs/heads/main"))); got != "refs/heads/main" {
		t.Errorf("got %q", got)
	}
	if got, want := displayRef(gitmanager.NewObservedRef([]byte{'r', 0xfe})), "base64url:"+gitmanager.EncodeReversiblePath([]byte{'r', 0xfe}); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
