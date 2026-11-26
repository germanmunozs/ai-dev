//go:build !darwin && !linux

package fswatch

// New reports that watching is unavailable on this platform (F015-FR-016).
func New() (Watcher, error) {
	return nil, Unavailable{Reason: "platform_unsupported", Detail: "watching is supported on macOS and Linux"}
}
