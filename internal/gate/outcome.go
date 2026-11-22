package gate

import "github.com/germanmunozs/ai-dev/internal/workflow"

// Outcome is a canonical Human Gate outcome.
type Outcome string

const (
	TransitionApplied Outcome = "transition_applied"
	FeatureUnknown    Outcome = Outcome(workflow.FeatureUnknown)
	StateInvalid      Outcome = Outcome(workflow.StateInvalid)
	TargetUnsupported Outcome = Outcome(workflow.TargetUnsupported)
	TransitionInvalid Outcome = Outcome(workflow.TransitionInvalid)
	ArtifactsMissing  Outcome = "artifacts_missing"
	StateConflict     Outcome = "state_conflict"
	PersistenceFailed Outcome = "persistence_failed"
)

// Report is the result of one Human Gate request.
type Report struct {
	FeatureID      string
	Decision       Decision
	Approver       string
	FromState      workflow.State
	SuppliedTarget string
	Target         workflow.State
	Reason         string
	ReasonSet      bool
	Missing        []string
	Outcome        Outcome
}
