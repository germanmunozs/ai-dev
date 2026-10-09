package gate

import (
	"time"

	"ai-dev/internal/workflow"
)

// Evidence is the durable record of one applied Human Gate decision.
type Evidence struct {
	FeatureID string
	Decision  Decision
	Approver  string
	FromState workflow.State
	ToState   workflow.State
	DecidedAt time.Time
	Reason    string
	ReasonSet bool
}
