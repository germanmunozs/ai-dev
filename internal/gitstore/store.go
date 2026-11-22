// Package gitstore contains the durable Git authorization, audit, and
// locking adapters, and the filesystem observation they depend on. Every
// file it writes lives beneath the private Git state root, never inside a
// project. Only (darwin && !ios) || (linux && !android) is supported; every
// other platform fails closed.
package gitstore

import (
	"fmt"

	"github.com/germanmunozs/ai-dev/internal/gitmanager"
)

var _ gitmanager.GitStore = (*Store)(nil)

// PlatformError reports an operation that cannot run on this platform or
// with this state, together with the closed reason the caller returns.
type PlatformError struct {
	Reason gitmanager.Reason
	Detail string
}

func (e *PlatformError) Error() string {
	return fmt.Sprintf("git store: %s (%s)", e.Detail, e.Reason)
}
