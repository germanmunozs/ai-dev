package gitmanager

import (
	"context"
	"errors"
	"sort"
)

// Failure classes of a SecondaryFailure produced by release.
const (
	// releaseFailureOwnership: the resource was missing or replaced, so it
	// was not released.
	releaseFailureOwnership = "ownership_unverified"
	// releaseFailureRelease: ownership was verified but the release failed.
	releaseFailureRelease = "release_failed"
	// releaseFailureAudit: a release record could not be published. When
	// its start record failed the resource was not released.
	releaseFailureAudit = "release_audit_failed"
)

// ownedRelease is one resource created and exclusively owned by the current
// attempt, with its planned release ordinal. Release must reverify
// ownership and report ErrOwnedResourceReplaced instead of touching a
// resource it no longer owns.
type ownedRelease struct {
	Role      ResourceRole
	Operation OperationID
	Ordinal   uint64
	Release   func(context.Context) error
}

// releaseReport is the outcome of the planned release protocol.
type releaseReport struct {
	Failures    []SecondaryFailure
	AuditFailed bool
}

// releaseOwned runs the planned release protocol (F008-FR-169, contract §7)
// over the attempt's owned resources in planned order, the apply lock last.
// Each release is preceded by a durable resource_release_started record and
// followed by its resource_release_result; a release whose start record
// cannot be published is not performed. Every failed release becomes one
// SecondaryFailure citing the latest durable release record, if any. It is
// never rollback: it touches only the listed resources.
func releaseOwned(ctx context.Context, journal *attemptJournal, resources []ownedRelease) releaseReport {
	// The apply lock is planned last: its ordinal follows every other one.
	ordered := append([]ownedRelease(nil), resources...)
	var last uint64
	for _, resource := range ordered {
		if resource.Role != ResourceRoleApplyLock && resource.Ordinal > last {
			last = resource.Ordinal
		}
	}
	for i := range ordered {
		if ordered[i].Role == ResourceRoleApplyLock {
			ordered[i].Ordinal = last + 1
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if a.Ordinal != b.Ordinal {
			return a.Ordinal < b.Ordinal
		}
		return a.Role < b.Role
	})
	var report releaseReport
	for _, resource := range ordered {
		details := map[string]any{"resource_role": string(resource.Role), "release_ordinal": canonicalInteger(resource.Ordinal)}
		failure := SecondaryFailure{
			Code: ReasonResourceReleaseFailed, Phase: SecondaryFailurePhaseResourceRelease, ResourceRole: resource.Role,
			OperationID: resource.Operation, ReleaseOrdinal: resource.Ordinal,
		}
		started, err := journal.append(ctx, AuditEventResourceReleaseStarted, "", details)
		if err != nil {
			report.AuditFailed = true
			failure.FailureClass = releaseFailureAudit
			report.Failures = append(report.Failures, failure)
			continue
		}
		releaseErr := resource.Release(ctx)
		outcome := "released"
		switch {
		case errors.Is(releaseErr, ErrOwnedResourceReplaced):
			outcome, failure.FailureClass = releaseFailureOwnership, releaseFailureOwnership
		case releaseErr != nil:
			outcome, failure.FailureClass, failure.OwnershipVerified = releaseFailureRelease, releaseFailureRelease, true
		default:
			failure.OwnershipVerified = true
		}
		details["outcome"] = outcome
		failure.AuditRecordDigest = started
		if result, err := journal.append(ctx, AuditEventResourceReleaseResult, "", details); err != nil {
			report.AuditFailed = true
			if releaseErr == nil {
				failure.FailureClass = releaseFailureAudit
			}
		} else {
			failure.AuditRecordDigest = result
			if releaseErr == nil {
				continue
			}
		}
		report.Failures = append(report.Failures, failure)
	}
	SortSecondaryFailures(report.Failures)
	return report
}
