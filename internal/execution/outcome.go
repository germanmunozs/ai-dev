package execution

// Outcome is the stable semantic result of a recognized execution request.
type Outcome string

const (
	AgentSucceeded  Outcome = "agent_succeeded"
	AgentFailed     Outcome = "agent_failed"
	ExecutionFailed Outcome = "execution_failed"
)

// Report is the final vendor-neutral execution result.
type Report struct {
	RequestedAgentID string
	Outcome          Outcome
	Explanation      string
}
