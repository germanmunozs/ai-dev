// Package workflow defines the vendor-neutral workflow-state model: the
// canonical Workflow State vocabulary, the fixed Transition Rule Set, and
// read-only Workflow State Record resolution. It has no dependency on any
// coding-agent, role, selection, execution, or process package.
package workflow

// State is a canonical Workflow State value, independent of any coding agent
// identity or development role.
type State string

const (
	Draft       State = "draft"
	Specified   State = "specified"
	Planned     State = "planned"
	Tasked      State = "tasked"
	Implemented State = "implemented"
	Approved    State = "approved"
	Abandoned   State = "abandoned"
)

// Recognized reports whether value is exactly, case-sensitively, one of the
// seven canonical Workflow State values, with no trimming, normalization,
// aliasing, case folding, or abbreviation.
func Recognized(value string) (State, bool) {
	switch State(value) {
	case Draft, Specified, Planned, Tasked, Implemented, Approved, Abandoned:
		return State(value), true
	default:
		return "", false
	}
}

// Terminal reports whether s is a terminal Workflow State. Only Approved and
// Abandoned are terminal; Implemented is not.
func Terminal(s State) bool {
	return s == Approved || s == Abandoned
}
