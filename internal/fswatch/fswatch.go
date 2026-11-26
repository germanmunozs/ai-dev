// Package fswatch observes paths through the operating system's change
// notifications, for ai-dev watch (Feature 015, research R-01): kqueue on
// macOS, inotify on Linux, and an adapter that is always unavailable
// elsewhere. It only reads: every descriptor it opens is for events, and it
// never follows a symbolic link. It reports that something changed, never
// what; the caller reads the files again and compares results.
package fswatch

import "fmt"

// Kind classifies a notification.
type Kind int

const (
	// Changed means the path, or an entry of the directory, may have changed.
	Changed Kind = iota + 1
	// Gone means the observed path itself was deleted or renamed.
	Gone
	// Overflow means the kernel dropped notifications; everything must be
	// read again.
	Overflow
)

// Change is one notification. Path is the absolute path the watcher was
// asked to observe; it is empty for Overflow.
type Change struct {
	Path string
	Kind Kind
}

// Watcher observes a set of paths.
type Watcher interface {
	// Sync makes the observed set exactly dirs and files. A path that does
	// not exist, or is a symbolic link, is skipped. The Linux adapter
	// observes directories only, so it ignores files.
	Sync(dirs, files []string) error
	// Events delivers notifications until Close.
	Events() <-chan Change
	// Close releases every descriptor; Events is closed afterwards.
	Close() error
}

// Unavailable reports that watching cannot be established or was lost, with
// the cause and how to fix it (F015-FR-016).
type Unavailable struct {
	Reason string // platform_unsupported, watch_limit, open_file_limit, instance_limit, watch_failed
	Detail string
	Remedy string
}

func (u Unavailable) Error() string {
	if u.Remedy == "" {
		return fmt.Sprintf("%s: %s", u.Reason, u.Detail)
	}
	return fmt.Sprintf("%s: %s (%s)", u.Reason, u.Detail, u.Remedy)
}
