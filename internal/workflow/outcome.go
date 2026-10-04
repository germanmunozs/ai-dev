package workflow

// Outcome is one of the six canonical workflow outcome values shared by the
// state-reporting and transition-evaluation capabilities (FR-013).
type Outcome string

const (
	StateReported     Outcome = "state_reported"
	FeatureUnknown    Outcome = "feature_unknown"
	StateInvalid      Outcome = "state_invalid"
	TransitionValid   Outcome = "transition_valid"
	TransitionInvalid Outcome = "transition_invalid"
	TargetUnsupported Outcome = "target_unsupported"
)

// StateReport is the result of one state-reporting request (Workflow State
// Report).
type StateReport struct {
	FeatureID string
	State     State
	Terminal  bool
	Outcome   Outcome
}

// TransitionReport is the result of one transition-evaluation request
// (Transition Evaluation Result).
type TransitionReport struct {
	FeatureID      string
	CurrentState   State
	SuppliedTarget string
	Target         State
	Outcome        Outcome
}
