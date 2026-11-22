package gitmanager

import (
	"context"
	"reflect"
	"testing"
)

// fakeResource is one owned resource whose release outcome is scripted.
type fakeResource struct {
	err      error
	released int
	order    *[]ResourceRole
	role     ResourceRole
}

func (r *fakeResource) release(context.Context) error {
	r.released++
	if r.order != nil {
		*r.order = append(*r.order, r.role)
	}
	return r.err
}

func owned(role ResourceRole, ordinal uint64, resource *fakeResource) ownedRelease {
	resource.role = role
	return ownedRelease{Role: role, Ordinal: ordinal, Release: resource.release}
}

func releaseJournal(t *testing.T) (*kernelFixture, *attemptJournal) {
	f := newKernelFixture(t)
	return f, f.journal(testApplyID)
}

func TestReleaseSoleFailure(t *testing.T) {
	f, journal := releaseJournal(t)
	failing := &fakeResource{err: errInjected}
	report := releaseOwned(context.Background(), journal, []ownedRelease{owned(ResourceRoleIndexLock, 1, failing)})
	if report.AuditFailed || len(report.Failures) != 1 {
		t.Fatalf("report %+v", report)
	}
	failure := report.Failures[0]
	if failure.Code != ReasonResourceReleaseFailed || failure.Phase != SecondaryFailurePhaseResourceRelease || failure.ResourceRole != ResourceRoleIndexLock ||
		failure.FailureClass != releaseFailureRelease || !failure.OwnershipVerified || failure.ReleaseOrdinal != 1 || failure.Validate() != nil {
		t.Fatalf("failure %+v", failure)
	}
	// The failure cites the durable result record of its release.
	events := f.store.eventsSnapshot()
	if len(events) != 2 || events[1].Kind != AuditEventResourceReleaseResult || failure.AuditRecordDigest != recordDigest(events[1].Record) {
		t.Fatalf("events %+v, digest %q", events, failure.AuditRecordDigest)
	}
	if failing.released != 1 {
		t.Fatalf("released %d times", failing.released)
	}
}

// A replaced or missing resource is not released and reports unverified
// ownership.
func TestReleaseReplacedResource(t *testing.T) {
	_, journal := releaseJournal(t)
	replaced := &fakeResource{err: ErrOwnedResourceReplaced}
	report := releaseOwned(context.Background(), journal, []ownedRelease{owned(ResourceRoleConfigTemporary, 2, replaced)})
	if len(report.Failures) != 1 || report.Failures[0].OwnershipVerified || report.Failures[0].FailureClass != releaseFailureOwnership {
		t.Fatalf("report %+v", report)
	}
}

// Releases run in planned order with the apply lock last, and failures are
// ordered by planned ordinal and role whatever order they arrive in.
func TestReleaseOrderIsDeterministic(t *testing.T) {
	var expected []SecondaryFailure
	for _, permutation := range [][]int{{0, 1, 2, 3}, {3, 2, 1, 0}, {2, 0, 3, 1}} {
		_, journal := releaseJournal(t)
		var order []ResourceRole
		resources := []ownedRelease{
			owned(ResourceRoleApplyLock, 1, &fakeResource{err: errInjected, order: &order}),
			owned(ResourceRolePreparedIndex, 3, &fakeResource{err: errInjected, order: &order}),
			owned(ResourceRoleIndexLock, 2, &fakeResource{order: &order}),
			owned(ResourceRoleConfigTemporary, 2, &fakeResource{err: ErrOwnedResourceReplaced, order: &order}),
		}
		permuted := make([]ownedRelease, len(resources))
		for i, j := range permutation {
			permuted[i] = resources[j]
		}
		report := releaseOwned(context.Background(), journal, permuted)
		wantOrder := []ResourceRole{ResourceRoleConfigTemporary, ResourceRoleIndexLock, ResourceRolePreparedIndex, ResourceRoleApplyLock}
		if !reflect.DeepEqual(order, wantOrder) {
			t.Fatalf("release order %v, want %v", order, wantOrder)
		}
		roles := make([]ResourceRole, len(report.Failures))
		for i, failure := range report.Failures {
			roles[i] = failure.ResourceRole
			failure.AuditRecordDigest = ""
			report.Failures[i] = failure
		}
		if !reflect.DeepEqual(roles, []ResourceRole{ResourceRoleConfigTemporary, ResourceRolePreparedIndex, ResourceRoleApplyLock}) {
			t.Fatalf("failure order %v", roles)
		}
		if report.Failures[2].ReleaseOrdinal <= report.Failures[1].ReleaseOrdinal {
			t.Fatalf("apply lock ordinal %d is not last", report.Failures[2].ReleaseOrdinal)
		}
		if expected == nil {
			expected = report.Failures
		} else if !reflect.DeepEqual(expected, report.Failures) {
			t.Fatalf("permutation %v produced %+v, want %+v", permutation, report.Failures, expected)
		}
	}
}

// A release whose start cannot be audited is not performed; one whose
// result cannot be audited is reported against the durable start record.
func TestReleaseAuditFailures(t *testing.T) {
	t.Run("start record", func(t *testing.T) {
		f, journal := releaseJournal(t)
		f.store.faults.failCall("AppendAuditEvent", 0, errInjected, false)
		resource := &fakeResource{}
		report := releaseOwned(context.Background(), journal, []ownedRelease{owned(ResourceRolePreparedIndex, 1, resource)})
		if !report.AuditFailed || resource.released != 0 || len(report.Failures) != 1 {
			t.Fatalf("report %+v, released %d", report, resource.released)
		}
		if failure := report.Failures[0]; failure.FailureClass != releaseFailureAudit || failure.OwnershipVerified || failure.AuditRecordDigest != "" {
			t.Fatalf("failure %+v", failure)
		}
	})
	t.Run("result record", func(t *testing.T) {
		f, journal := releaseJournal(t)
		f.store.faults.failCall("AppendAuditEvent", 1, errInjected, false)
		resource := &fakeResource{}
		report := releaseOwned(context.Background(), journal, []ownedRelease{owned(ResourceRolePreparedIndex, 1, resource)})
		events := f.store.eventsSnapshot()
		if !report.AuditFailed || resource.released != 1 || len(report.Failures) != 1 || len(events) != 1 {
			t.Fatalf("report %+v, events %d", report, len(events))
		}
		if failure := report.Failures[0]; failure.FailureClass != releaseFailureAudit || !failure.OwnershipVerified ||
			failure.AuditRecordDigest != recordDigest(events[0].Record) {
			t.Fatalf("failure %+v", failure)
		}
	})
	t.Run("later releases after a broken trail", func(t *testing.T) {
		f, journal := releaseJournal(t)
		f.store.faults.failCall("AppendAuditEvent", 1, errInjected, false)
		first, second := &fakeResource{}, &fakeResource{}
		report := releaseOwned(context.Background(), journal, []ownedRelease{
			owned(ResourceRoleIndexLock, 1, first), owned(ResourceRoleApplyLock, 2, second),
		})
		// The open release cannot be closed in the trail, so the next start
		// is unpublishable and that resource stays held until process exit.
		if second.released != 0 || len(report.Failures) != 2 || report.Failures[1].ResourceRole != ResourceRoleApplyLock {
			t.Fatalf("report %+v, apply lock released %d", report, second.released)
		}
	})
}

func TestReleaseSuccessLeavesNoFailures(t *testing.T) {
	f, journal := releaseJournal(t)
	resource := &fakeResource{}
	report := releaseOwned(context.Background(), journal, []ownedRelease{owned(ResourceRoleApplyLock, 1, resource)})
	if report.AuditFailed || len(report.Failures) != 0 || resource.released != 1 {
		t.Fatalf("report %+v", report)
	}
	want := []AuditEventKind{AuditEventResourceReleaseStarted, AuditEventResourceReleaseResult}
	if events := f.events(testApplyID); !reflect.DeepEqual(events, want) {
		t.Fatalf("events %v", events)
	}
}
