//go:build !((darwin && !ios) || (linux && !android))

package gitmanager

import "context"

// installIndex fails closed where the descriptor-owned index.lock protocol
// cannot be proven.
func (d *OperationDispatcher) installIndex(context.Context, MutationPermit, GitInitializationPlan, DerivedOperation) OperationOutcome {
	return OperationFailed
}

func (d *OperationDispatcher) createDirectory(context.Context, MutationPermit, GitInitializationPlan, DerivedOperation) OperationOutcome {
	return OperationFailed
}

func (d *OperationDispatcher) writeConfig(context.Context, MutationPermit, GitInitializationPlan, DerivedOperation) OperationOutcome {
	return OperationFailed
}
