//go:build !((darwin && !ios) || (linux && !android))

package gitstore

import (
	"context"

	"github.com/germanmunozs/ai-dev/internal/gitmanager"
)

// Store is unavailable on this platform; Open never returns one, and every
// method fails closed.
type Store struct{}

func (*Store) Close() error { return unsupportedError() }
func (*Store) CreateAttempt(context.Context, gitmanager.GitApplyAttempt) error {
	return unsupportedError()
}
func (*Store) RecordAttemptResult(context.Context, gitmanager.ApplyID, []byte) (gitmanager.ContentDigest, error) {
	return "", unsupportedError()
}
func (*Store) ListAuditEvents(context.Context, gitmanager.ApplyID) (gitmanager.AuditTrail, error) {
	return gitmanager.AuditTrail{}, unsupportedError()
}
func (*Store) IssueAuthorization(context.Context, gitmanager.GitAuthorization) error {
	return unsupportedError()
}
func (*Store) LoadAuthorization(context.Context, gitmanager.ProjectIdentityDigest, gitmanager.AuthorizationID) (gitmanager.GitAuthorization, error) {
	return gitmanager.GitAuthorization{}, unsupportedError()
}
func (*Store) TransitionAuthorization(context.Context, gitmanager.AuthorizationTransitionRequest) (gitmanager.GitAuthorization, error) {
	return gitmanager.GitAuthorization{}, unsupportedError()
}
func (*Store) AppendAuditEvent(context.Context, gitmanager.AuditEvent) (gitmanager.ContentDigest, error) {
	return "", unsupportedError()
}
func (*Store) AcquireApplyLock(context.Context, gitmanager.PhysicalProjectIdentity, gitmanager.ApplyID) (gitmanager.ApplyLock, error) {
	return nil, unsupportedError()
}

// ObserveFilesystem fails closed with platform_unsupported.
func ObserveFilesystem(path string) (gitmanager.FilesystemObservation, error) {
	return unsupportedObserveFilesystem(path)
}

// ProbePrimitives fails closed with platform_unsupported.
func ProbePrimitives(dir string) (gitmanager.FilesystemPrimitives, error) {
	return unsupportedProbePrimitives(dir)
}

// Open fails closed with platform_unsupported.
func Open(stateRoot gitmanager.ObservedPath) (*Store, error) { return unsupportedOpen(stateRoot) }
