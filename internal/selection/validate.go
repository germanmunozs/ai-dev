package selection

import (
	"strings"

	"ai-dev/internal/role"
)

// Valid reports whether r is a Report the approved selection model could
// actually have produced: an exhaustive check of field presence, role
// recognition, and mapping correctness against the fixed production policy.
//
// This is a read-only predicate over one already-built Report. It exposes no
// way to view, edit, override, persist, or otherwise reach the underlying
// mappedAgent switch from outside this package — only a yes/no fact about
// the given Report crosses the package boundary. It exists so callers such
// as internal/report can defensively reject a malformed or inconsistent
// Report before rendering it, without duplicating the fixed role-to-agent
// mapping in a second, independently-maintained place.
func (r Report) Valid() bool {
	// SuppliedRole must always be present and carry actual content — a
	// whitespace-only value is not a legitimate supplied role under any
	// outcome (this is a presence check only: the raw, untrimmed value is
	// still what role.Recognized compares against below, so no valid role
	// value is trimmed or normalized).
	if strings.TrimSpace(r.SuppliedRole) == "" {
		return false
	}
	if r.SelectedAgentID != "" && r.MappedAgentID != "" {
		return false
	}

	recognized, isRecognized := role.Recognized(r.SuppliedRole)

	switch r.Outcome {
	case RoleUnsupported:
		return !isRecognized && r.RequestedRole == "" && r.SelectedAgentID == "" && r.MappedAgentID == ""

	case RoleSelected:
		if !isRecognized || r.RequestedRole != recognized {
			return false
		}
		target, ok := mappedAgent(recognized)
		return ok && r.SelectedAgentID == target && r.MappedAgentID == ""

	case AgentUnavailable, SelectionInconsistent:
		if !isRecognized || r.RequestedRole != recognized {
			return false
		}
		target, ok := mappedAgent(recognized)
		return ok && r.MappedAgentID == target && r.SelectedAgentID == ""

	default:
		return false
	}
}
