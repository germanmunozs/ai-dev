package gate

import (
	"io/fs"
	"time"

	"github.com/germanmunozs/ai-dev/internal/sdd"
	"github.com/germanmunozs/ai-dev/internal/workflow"
)

// Evaluate performs Human Gate precedence steps 3 through 9.
func Evaluate(root fs.FS, req Request, store Store, now func() time.Time) (Report, []string) {
	base := Report{FeatureID: req.FeatureID, Decision: req.Decision, Approver: req.Approver}
	transition := workflow.EvaluateTransition(root, req.FeatureID, req.SuppliedTarget)
	switch transition.Outcome {
	case workflow.TargetUnsupported:
		base.SuppliedTarget = req.SuppliedTarget
		base.Outcome = TargetUnsupported
		return base, nil
	case workflow.FeatureUnknown:
		base.Outcome = FeatureUnknown
		return base, nil
	case workflow.StateInvalid:
		base.Outcome = StateInvalid
		return base, nil
	case workflow.TransitionInvalid:
		base.FromState = transition.CurrentState
		base.SuppliedTarget = transition.SuppliedTarget
		base.Target = transition.Target
		base.Outcome = TransitionInvalid
		return base, nil
	case workflow.TransitionValid:
	default:
		base.FromState = transition.CurrentState
		base.SuppliedTarget = req.SuppliedTarget
		base.Target = transition.Target
		base.Outcome = PersistenceFailed
		return base, nil
	}

	base.FromState = transition.CurrentState
	base.SuppliedTarget = transition.SuppliedTarget
	base.Target = transition.Target
	if req.Decision == Approval {
		if missing := sdd.MissingArtifacts(root, req.FeatureID, transition.Target); len(missing) > 0 {
			base.Missing = missing
			base.Outcome = ArtifactsMissing
			return base, nil
		}
	}

	entry := Evidence{
		FeatureID: req.FeatureID, Decision: req.Decision, Approver: req.Approver,
		FromState: transition.CurrentState, ToState: transition.Target,
		DecidedAt: now().UTC(), Reason: req.Reason, ReasonSet: req.ReasonSet,
	}
	status, warnings := store.Commit(req.FeatureID, transition.CurrentState, entry)
	switch status {
	case Committed:
		base.Outcome = TransitionApplied
		base.Reason, base.ReasonSet = req.Reason, req.ReasonSet
	case Conflict:
		base.Outcome = StateConflict
	case Failed:
		base.Outcome = PersistenceFailed
	default:
		base.Outcome = PersistenceFailed
	}
	return base, warnings
}
