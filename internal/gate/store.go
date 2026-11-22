package gate

import "github.com/germanmunozs/ai-dev/internal/workflow"

// CommitStatus describes the result of the store's all-or-nothing commit.
type CommitStatus int

const (
	Committed CommitStatus = iota + 1
	Conflict
	Failed
)

// Store is the sole persistence port used by the Human Gate domain.
type Store interface {
	Commit(featureID string, expected workflow.State, entry Evidence) (CommitStatus, []string)
}
