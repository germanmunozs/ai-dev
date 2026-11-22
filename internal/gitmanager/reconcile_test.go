package gitmanager

import (
	"bytes"
	"context"
	"testing"
	"time"
)

const reconciliationAuthorizationID AuthorizationID = "123e4567-e89b-42d3-a456-426614174031"

// reconciliationInput is a new supervised authorization over a fresh plan.
func reconciliationInput(t *testing.T, f *kernelFixture) GitAuthorizationInput {
	in := validAuthorizationInput(t)
	in.SchemaVersion = GitAuthorizationSchema
	in.AuthorizationID = reconciliationAuthorizationID
	in.PlanDigest = testOtherSHA256
	in.ProjectIdentity = f.project
	in.IssuedAt = time.Unix(200, 0).UTC()
	return in
}

// leaveIndeterminate consumes the fixture authorization with an operation
// whose outcome cannot be established.
func leaveIndeterminate(t *testing.T, f *kernelFixture) {
	session := f.claim(testApplyID)
	mustFinish(t, session, mustBegin(t, session, kernelOperations[0]), operationIndeterminate)
	if result := session.close(context.Background(), terminalClosure{}); result.Status != ApplyStatusIndeterminate {
		t.Fatalf("setup result %+v", result)
	}
}

func encodedAuthorization(t *testing.T, v GitAuthorization) []byte {
	data, err := EncodeAuthorizationRecord(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestReconciliationLinksANewAuthorization(t *testing.T) {
	ctx := context.Background()
	f := newKernelFixture(t)
	leaveIndeterminate(t, f)
	prior := f.durable()
	priorBytes := encodedAuthorization(t, prior)
	journal := f.journal("123e4567-e89b-42d3-a456-426614174032")
	calls := len(f.store.callsSnapshot())
	issued, err := issueReconciliation(ctx, f.store, journal, prior.AuthorizationID, reconciliationInput(t, f))
	if err != nil {
		t.Fatal(err)
	}
	if issued.PreviousAuthorizationID != prior.AuthorizationID || issued.PreviousApplyID != testApplyID || issued.PreviousPlanDigest != prior.PlanDigest ||
		issued.State != AuthorizationStateIssued {
		t.Fatalf("linked authorization %+v", issued)
	}
	for _, call := range f.store.callsSnapshot()[calls:] {
		if call == "TransitionAuthorization" {
			t.Fatal("reconciliation transitioned an authorization")
		}
	}
	if after := f.durable(); !bytes.Equal(encodedAuthorization(t, after), priorBytes) {
		t.Fatal("reconciliation changed the prior authorization")
	}
	if events := f.events("123e4567-e89b-42d3-a456-426614174032"); len(events) != 1 || events[0] != AuditEventAuthorizationIssued {
		t.Fatalf("reconciliation events %v", events)
	}
	loaded, err := f.store.LoadAuthorization(ctx, f.project.Digest(), reconciliationAuthorizationID)
	if err != nil || !bytes.Equal(encodedAuthorization(t, loaded), encodedAuthorization(t, issued)) {
		t.Fatalf("issued record %v", err)
	}
	// The new authorization is independent authority.
	request := f.request(testOtherApplyID)
	request.authorization, request.planDigest = reconciliationAuthorizationID, testOtherSHA256
	if _, refusal := f.kernel.claim(ctx, request); refusal != nil {
		t.Fatalf("reconciliation authorization refused: %+v", *refusal)
	}
}

func TestReconciliationRequiresAnUnresolvedPrior(t *testing.T) {
	ctx := context.Background()
	t.Run("issued prior", func(t *testing.T) {
		f := newKernelFixture(t)
		if _, err := issueReconciliation(ctx, f.store, f.journal(testOtherApplyID), f.authorization.AuthorizationID, reconciliationInput(t, f)); err == nil {
			t.Fatal("reconciled an unused authorization")
		}
	})
	t.Run("conclusive prior", func(t *testing.T) {
		f := newKernelFixture(t)
		session := f.claim(testApplyID)
		for _, op := range kernelOperations {
			mustFinish(t, session, mustBegin(t, session, op), operationCompleted)
		}
		session.close(ctx, terminalClosure{VerifiedComplete: true})
		if _, err := issueReconciliation(ctx, f.store, f.journal(testOtherApplyID), f.authorization.AuthorizationID, reconciliationInput(t, f)); err == nil {
			t.Fatal("reconciled a conclusive authorization")
		}
	})
	t.Run("other project", func(t *testing.T) {
		f := newKernelFixture(t)
		leaveIndeterminate(t, f)
		in := reconciliationInput(t, f)
		other, err := NewPhysicalProjectIdentity("/tmp/other", []byte("/tmp/other"), "1", "3")
		if err != nil {
			t.Fatal(err)
		}
		in.ProjectIdentity = other
		if _, err := issueReconciliation(ctx, f.store, f.journal(testOtherApplyID), f.authorization.AuthorizationID, in); err == nil {
			t.Fatal("reconciled across project identities")
		}
	})
	t.Run("reused authorization ID", func(t *testing.T) {
		f := newKernelFixture(t)
		leaveIndeterminate(t, f)
		in := reconciliationInput(t, f)
		in.AuthorizationID = f.authorization.AuthorizationID
		if _, err := issueReconciliation(ctx, f.store, f.journal(testOtherApplyID), f.authorization.AuthorizationID, in); err == nil {
			t.Fatal("reconciliation reused the prior authorization ID")
		}
	})
	t.Run("consumed claim with intent", func(t *testing.T) {
		f := newKernelFixture(t)
		session := f.claim(testApplyID)
		f.store.faults.failCall("TransitionAuthorization", 1, errInjected, false)
		if _, err := session.begin(ctx, kernelOperations[0]); err == nil {
			t.Fatal("setup: start CAS did not fail")
		}
		session.close(ctx, terminalClosure{})
		if _, err := issueReconciliation(ctx, f.store, f.journal(testOtherApplyID), f.authorization.AuthorizationID, reconciliationInput(t, f)); err != nil {
			t.Fatalf("claimed-with-intent prior not reconcilable: %v", err)
		}
	})
}

// occurrences counts the calls of method the fake store has recorded, so a
// fault can name the next one.
func occurrences(f *kernelFixture, method string) int {
	n := 0
	for _, call := range f.store.callsSnapshot() {
		if call == method {
			n++
		}
	}
	return n
}

// T126: every outcome of a linked reconciliation maps to its closed reason.
func TestReconcileAuthorizationReasons(t *testing.T) {
	const absentID AuthorizationID = "123e4567-e89b-42d3-a456-426614174099"
	for name, test := range map[string]struct {
		setup  func(t *testing.T, f *kernelFixture) AuthorizationID
		reason Reason
	}{
		"invalid prior ID":       {func(t *testing.T, f *kernelFixture) AuthorizationID { return "not-an-id" }, ReasonAuthorizationMismatch},
		"prior is the new ID":    {func(t *testing.T, f *kernelFixture) AuthorizationID { return reconciliationAuthorizationID }, ReasonAuthorizationMismatch},
		"missing prior":          {func(t *testing.T, f *kernelFixture) AuthorizationID { return absentID }, ReasonAuthorizationMissing},
		"prior not reconcilable": {func(t *testing.T, f *kernelFixture) AuthorizationID { return f.authorization.AuthorizationID }, ReasonAuthorizationMismatch},
		"new ID exists": {func(t *testing.T, f *kernelFixture) AuthorizationID {
			leaveIndeterminate(t, f)
			if rejection := reconcileAuthorization(context.Background(), f.store, f.journal(testOtherApplyID), f.authorization.AuthorizationID, reconciliationInput(t, f)); rejection != nil {
				t.Fatalf("first reconciliation %+v", *rejection)
			}
			return f.authorization.AuthorizationID
		}, ReasonAuthorizationIDExists},
		"prior unreadable": {func(t *testing.T, f *kernelFixture) AuthorizationID {
			leaveIndeterminate(t, f)
			f.store.faults.failCall("LoadAuthorization", occurrences(f, "LoadAuthorization"), errInjected, false)
			return f.authorization.AuthorizationID
		}, ReasonAuditUnavailable},
		"issuance fails": {func(t *testing.T, f *kernelFixture) AuthorizationID {
			leaveIndeterminate(t, f)
			f.store.faults.failCall("IssueAuthorization", occurrences(f, "IssueAuthorization"), errInjected, false)
			return f.authorization.AuthorizationID
		}, ReasonAuditUnavailable},
		"issuance audit fails": {func(t *testing.T, f *kernelFixture) AuthorizationID {
			leaveIndeterminate(t, f)
			f.store.faults.failCall("AppendAuditEvent", occurrences(f, "AppendAuditEvent"), errInjected, false)
			return f.authorization.AuthorizationID
		}, ReasonAuditUnavailable},
		"prior reread fails": {func(t *testing.T, f *kernelFixture) AuthorizationID {
			leaveIndeterminate(t, f)
			f.store.faults.failCall("LoadAuthorization", occurrences(f, "LoadAuthorization")+1, errInjected, false)
			return f.authorization.AuthorizationID
		}, ReasonAuditUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			f := newKernelFixture(t)
			prior := test.setup(t, f)
			journal := f.journal("123e4567-e89b-42d3-a456-426614174033")
			rejection := reconcileAuthorization(context.Background(), f.store, journal, prior, reconciliationInput(t, f))
			if rejection == nil || rejection.Reason != test.reason {
				t.Fatalf("rejection %+v, want %s", rejection, test.reason)
			}
		})
	}
	f := newKernelFixture(t)
	leaveIndeterminate(t, f)
	if rejection := reconcileAuthorization(context.Background(), f.store, f.journal(testOtherApplyID), f.authorization.AuthorizationID, reconciliationInput(t, f)); rejection != nil {
		t.Fatalf("reconciliation %+v", *rejection)
	}
}
