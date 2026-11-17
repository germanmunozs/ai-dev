package gitstore

import "ai-dev/internal/gitmanager"

// The fail-closed implementations are compiled on every platform so they
// can be tested on any host; unsupported_other.go selects them.

func unsupportedError() error {
	return &PlatformError{Reason: gitmanager.ReasonPlatformUnsupported,
		Detail: "durable Git state is supported only on macOS and Linux"}
}

func unsupportedObserveFilesystem(string) (gitmanager.FilesystemObservation, error) {
	return gitmanager.FilesystemObservation{}, unsupportedError()
}

func unsupportedProbePrimitives(string) (gitmanager.FilesystemPrimitives, error) {
	return gitmanager.FilesystemPrimitives{}, unsupportedError()
}

func unsupportedOpen(gitmanager.ObservedPath) (*Store, error) { return nil, unsupportedError() }
