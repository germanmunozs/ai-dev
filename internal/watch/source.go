package watch

import "context"

// Source supplies the results of the existing commands, computed by their
// own functions and rendered as their own JSON (F015-FR-006, research R-03).
// internal/app implements it from a read-only fs.FS and the executor, so
// this package depends on nothing that writes state or runs processes.
type Source interface {
	// Features lists the features Feature 004 recognizes, in name order.
	Features() ([]string, error)
	// State is "workflow state --format json <id>".
	State(id string) StateResult
	// Next is the bytes of "workflow next --format json <id>".
	Next(id string) []byte
	// Setup is the bytes of "setup show --format json".
	Setup() []byte
	// Environment is "doctor --format json".
	Environment(ctx context.Context) EnvironmentResult
}

// StateResult is one state report: its JSON bytes, and the state and
// outcome the bytes hold. State is meaningful only when Recognized.
type StateResult struct {
	JSON       []byte
	State      string
	Recognized bool
	Outcome    string
}

// EnvironmentResult is the doctor report: its JSON bytes and each check's
// status.
type EnvironmentResult struct {
	JSON   []byte
	Checks []CheckStatus
}

// CheckStatus is one doctor check: pass or fail, with its remediation.
type CheckStatus struct {
	ID, Status, Remediation string
}
