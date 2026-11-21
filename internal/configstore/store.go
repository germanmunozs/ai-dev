// Package configstore is ai-dev's sole write path for the Feature 011
// project configuration. It replaces .ai-dev/agents.json atomically and never
// writes through a symbolic link (F011-FR-007).
package configstore

// Store writes the project configuration.
type Store interface {
	// Write atomically replaces the configuration with data and reports
	// whether it did. Warnings describe problems, including ones that did
	// not prevent the write. The result uses only predeclared types, so a
	// consumer can declare this interface without importing configstore.
	Write(data []byte) (written bool, warnings []string)
}

// New returns the platform implementation for the project directory dir,
// without performing I/O.
func New(dir string) Store { return newPlatformStore(dir) }

const unsupportedPlatformWarning = "ai-dev: warning: writing the project configuration is supported only on macOS and Linux"

type unsupportedStore struct{}

func (unsupportedStore) Write([]byte) (bool, []string) {
	return false, []string{unsupportedPlatformWarning}
}
