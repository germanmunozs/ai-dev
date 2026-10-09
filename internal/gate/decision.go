// Package gate implements Feature 006's Human Gate domain and control flow.
// It records explicit decisions but performs no filesystem writes itself.
package gate

// Decision identifies the explicit kind of Human Gate decision.
type Decision string

const (
	Approval    Decision = "approval"
	Abandonment Decision = "abandonment"
)

// Request contains only values explicitly supplied by the caller.
type Request struct {
	FeatureID      string
	Decision       Decision
	SuppliedTarget string
	Approver       string
	Reason         string
	ReasonSet      bool
}
