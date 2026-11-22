package workflow

// Allowed reports whether the fixed Transition Rule Set (FR-004) contains a
// directed entry from `from` to `to`. This is the complete FR-004 table,
// expressed as code, not data — there is no map, slice, or struct literal
// that could be mutated, and no exported way to reconfigure it.
func Allowed(from, to State) bool {
	switch from {
	case Draft:
		return to == Specified || to == Abandoned
	case Specified:
		return to == Planned || to == Abandoned
	case Planned:
		return to == Tasked || to == Abandoned
	case Tasked:
		return to == Implemented || to == Abandoned
	case Implemented:
		return to == Approved || to == Abandoned
	default: // Approved, Abandoned: terminal, no outgoing entries (FR-003)
		return false
	}
}
