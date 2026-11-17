//go:build !((darwin && !ios) || (linux && !android))

package gitstore

import "ai-dev/internal/gitmanager"

// Store is unavailable on this platform.
type Store struct{}

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
