package gitmanager

import (
	"context"
	"fmt"
)

// DispatcherDependencies are the ports the operation dispatcher uses.
type DispatcherDependencies struct {
	Runner   GitRunner
	Commands GitCommandBuilder
	// Inspect freshly inspects the project at its physical path.
	Inspect func(ctx context.Context, projectPath []byte) (GitInspection, error)
}

// OperationDispatcher is the production OperationExecutor (T076). It runs
// only the typed operations of a regenerated plan, each only under the
// MutationPermit the safety kernel issued for it, by rebuilding its row
// and refusing any argv that differs from the planned one. It has no
// other mutation entrypoint and never rolls back, retries or repairs.
type OperationDispatcher struct {
	deps DispatcherDependencies
}

var _ OperationExecutor = (*OperationDispatcher)(nil)

func NewOperationDispatcher(deps DispatcherDependencies) *OperationDispatcher {
	return &OperationDispatcher{deps: deps}
}

func (d *OperationDispatcher) inspect(ctx context.Context, plan GitInitializationPlan) (GitInspection, error) {
	if d.deps.Inspect == nil {
		return GitInspection{}, fmt.Errorf("no inspector")
	}
	return d.deps.Inspect(ctx, plan.PreconditionSnapshot.ProjectIdentity.ReversiblePathBytes())
}

// Revalidate checks the operation's complete precondition set against a
// fresh inspection immediately before it (F008-FR-164). The caller
// classifies the reason by phase.
func (d *OperationDispatcher) Revalidate(ctx context.Context, plan GitInitializationPlan, operation DerivedOperation) *PlanRejection {
	inspection, err := d.inspect(ctx, plan)
	if err != nil {
		return reject(ReasonConcurrentChange, "the project could not be inspected: %v", err)
	}
	if inspection.ProjectIdentity.Digest() != plan.PreconditionSnapshot.ProjectIdentity.Digest() {
		return reject(ReasonConcurrentChange, "the physical project changed")
	}
	if result, condition := evaluateConditions(operation.Preconditions(), inspection); result != conditionHolds {
		return reject(ReasonConcurrentChange, "precondition %s does not hold", condition.Kind)
	}
	return nil
}

// Execute runs one operation. It fails closed, before invoking anything,
// when the permit does not authorize exactly this operation of this plan
// now, and it reports completion only when the operation's own expected
// postconditions hold on a fresh inspection.
func (d *OperationDispatcher) Execute(ctx context.Context, permit MutationPermit, plan GitInitializationPlan, operation DerivedOperation) OperationOutcome {
	if permit.authorizesPlanned(plan, operation) != nil || !d.planned(plan, operation) {
		return OperationFailed
	}
	switch operation.Kind {
	case OperationKindVerification:
		// The preconditions it verifies were just revalidated.
		return OperationCompleted
	case OperationKindRepositoryInit, OperationKindGitConfigWrite, OperationKindDevelopmentRefCAS:
		invocation, ok := d.invocation(plan, operation)
		if !ok || d.deps.Runner == nil || permit.authorizesPlanned(plan, operation) != nil {
			return OperationFailed
		}
		result, err := d.deps.Runner.Run(ctx, invocation)
		switch {
		case err != nil:
			return OperationIndeterminate
		case result.ExitCode != 0:
			return OperationFailed
		}
		return d.postconditions(ctx, plan, operation)
	}
	// Project-configuration writes are dispatched by the story that
	// implements their filesystem protocol (T093); until then nothing runs.
	return OperationFailed
}

// planned reports whether operation is, byte for byte, an operation of the
// regenerated plan.
func (d *OperationDispatcher) planned(plan GitInitializationPlan, operation DerivedOperation) bool {
	for _, candidate := range plan.DerivedOperations() {
		if candidate.OperationID == operation.OperationID {
			return candidate.Ordinal == operation.Ordinal && candidate.Kind == operation.Kind &&
				equalLists(candidate.ExactArgv(), operation.ExactArgv())
		}
	}
	return false
}

// invocation rebuilds the typed row of a Git operation from the plan and
// returns it only when its argv is exactly the planned argv.
func (d *OperationDispatcher) invocation(plan GitInitializationPlan, operation DerivedOperation) (GitInvocation, bool) {
	intent, snapshot := plan.ImmutableIntent, plan.PreconditionSnapshot
	root := snapshot.ProjectIdentity.ReversiblePathBytes()
	var command GitCommand
	switch operation.Kind {
	case OperationKindRepositoryInit:
		if checkInitArgv(intent, root, operation.ExactArgv()) != nil {
			return GitInvocation{}, false
		}
		command = GitCommand{Kind: GitCommandInit, BranchName: []byte(initialBranchOf(intent.Strategy, intent.BranchParameters())), PhysicalRoot: root}
	case OperationKindGitConfigWrite:
		keys, effects := operation.AffectedConfigKeys(), operation.ExpectedEffects()
		if len(keys) != 1 || len(effects) != 1 || effects[0].Kind != ConditionConfigValue || effects[0].Subject != keys[0] {
			return GitInvocation{}, false
		}
		command = GitCommand{Kind: GitCommandConfigWrite, ConfigKey: keys[0], ConfigValue: effects[0].Expected}
	case OperationKindDevelopmentRefCAS:
		if intent.DevelopmentBase.IsZero() {
			return GitInvocation{}, false
		}
		ref := "refs/heads/" + intent.BranchParameters()[BranchDevelopment]
		command = GitCommand{Kind: GitCommandUpdateRef, RefName: []byte(ref), ObjectOID: intent.DevelopmentBase.BaseOID}
	default:
		return GitInvocation{}, false
	}
	invocation, rejection := buildCommand(d.deps.Commands, intent, snapshot, command)
	if rejection != nil || !equalLists(invocation.Args, operation.ExactArgv()) {
		return GitInvocation{}, false
	}
	return invocation, true
}

// postconditions inspects the operation's complete expected postconditions
// after Git reported success (F008-FR-166): exit status alone proves
// nothing.
func (d *OperationDispatcher) postconditions(ctx context.Context, plan GitInitializationPlan, operation DerivedOperation) OperationOutcome {
	inspection, err := d.inspect(ctx, plan)
	if err != nil {
		return OperationIndeterminate
	}
	switch result, _ := evaluateConditions(operation.ExpectedPostconditions(), inspection); result {
	case conditionHolds:
		return OperationCompleted
	case conditionFails:
		return OperationFailed
	}
	return OperationIndeterminate
}

// Verify freshly inspects every planned postcondition and the effects of
// every mutating operation, for the terminal classification
// (F008-FR-166, 182, 189).
func (d *OperationDispatcher) Verify(ctx context.Context, plan GitInitializationPlan) ApplyEvidence {
	inspection, err := d.inspect(ctx, plan)
	if err != nil {
		return ApplyEvidence{EffectsUnverifiable: true}
	}
	var evidence ApplyEvidence
	complete, _ := evaluateConditions(plan.ExpectedPostconditions(), inspection)
	evidence.VerifiedComplete = complete == conditionHolds
	for _, operation := range plan.DerivedOperations() {
		for _, effect := range operation.ExpectedEffects() {
			switch evaluateCondition(effect, inspection) {
			case conditionHolds:
				evidence.VerifiedEffects = append(evidence.VerifiedEffects, effect)
			case conditionUnverifiable:
				evidence.EffectsUnverifiable = true
			}
		}
	}
	// A complete result is not a failure, even when every operation was a
	// verification without effects.
	evidence.ZeroEffectsProven = !evidence.EffectsUnverifiable && len(evidence.VerifiedEffects) == 0 && !evidence.VerifiedComplete
	return evidence
}
