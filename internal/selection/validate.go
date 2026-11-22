package selection

import (
	"strings"

	"github.com/germanmunozs/ai-dev/internal/agent"
	"github.com/germanmunozs/ai-dev/internal/role"
)

// Valid reports whether r is a Report the approved selection model could
// actually have produced: an exhaustive check of field presence, role
// recognition, and mapping correctness against the fixed production policy
// for a built_in report, or catalog membership for a project report.
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
	if r.Source != SourceBuiltIn && r.Source != SourceProject {
		return false
	}

	recognized, isRecognized := role.Recognized(r.SuppliedRole)

	switch r.Outcome {
	case RoleUnsupported:
		return !isRecognized && r.RequestedRole == "" && r.SelectedAgentID == "" && r.MappedAgentID == ""

	case ConfigurationInvalid:
		return isRecognized && r.RequestedRole == recognized && r.Source == SourceProject && r.SelectedAgentID == "" && r.MappedAgentID == ""
	}

	if r.Source == SourceProject {
		// A project report cannot be checked against the file, which the
		// caller has just read; it must still name a catalog agent, and a
		// project never yields selection_inconsistent (F011-FR-018).
		if !isRecognized || r.RequestedRole != recognized {
			return false
		}
		switch r.Outcome {
		case RoleSelected:
			return agent.Known(r.SelectedAgentID) && r.MappedAgentID == ""
		case AgentUnavailable:
			return agent.Known(r.MappedAgentID) && r.SelectedAgentID == ""
		default:
			return false
		}
	}

	switch r.Outcome {
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
