package sdd

import (
	"github.com/germanmunozs/ai-dev/internal/role"
	"github.com/germanmunozs/ai-dev/internal/workflow"
)

// Outcome is a canonical next-step evaluation outcome.
type Outcome string

const (
	StepRecommended       Outcome = "step_recommended"
	WorkflowTerminal      Outcome = "workflow_terminal"
	ArtifactsInconsistent Outcome = "artifacts_inconsistent"
	FeatureUnknown        Outcome = "feature_unknown"
	StateInvalid          Outcome = "state_invalid"
)

// Report is the outcome of one next-step evaluation request.
type Report struct {
	FeatureID string
	State     workflow.State
	Activity  Activity
	Role      role.Role
	Missing   []string
	Outcome   Outcome
}
