package report

import (
	"encoding/json"
	"fmt"
	"io"

	"ai-dev/internal/selection"
)

// WriteRoleSelectionText writes one human-readable line reporting the given
// Role Selection Report's outcome.
func WriteRoleSelectionText(w io.Writer, r selection.Report) error {
	if err := validateRoleSelectionReport(r); err != nil {
		return err
	}
	switch r.Outcome {
	case selection.RoleSelected:
		_, err := fmt.Fprintf(w, "ai-dev: outcome=%s role=%s agent=%s\n", r.Outcome, r.RequestedRole, r.SelectedAgentID)
		return err
	case selection.RoleUnsupported:
		_, err := fmt.Fprintf(w, "ai-dev: outcome=%s role=%q\n", r.Outcome, r.SuppliedRole)
		return err
	default:
		_, err := fmt.Fprintf(w, "ai-dev: outcome=%s role=%s agent=%s\n", r.Outcome, r.RequestedRole, r.MappedAgentID)
		return err
	}
}

// WriteRoleSelectionJSON writes exactly one machine-readable Role Selection
// Report object.
func WriteRoleSelectionJSON(w io.Writer, r selection.Report) error {
	if err := validateRoleSelectionReport(r); err != nil {
		return err
	}
	wire := struct {
		SuppliedRole    string            `json:"supplied_role"`
		RequestedRole   string            `json:"requested_role,omitempty"`
		SelectedAgentID string            `json:"selected_agent_id,omitempty"`
		MappedAgentID   string            `json:"mapped_agent_id,omitempty"`
		Outcome         selection.Outcome `json:"outcome"`
	}{
		SuppliedRole:    r.SuppliedRole,
		RequestedRole:   string(r.RequestedRole),
		SelectedAgentID: string(r.SelectedAgentID),
		MappedAgentID:   string(r.MappedAgentID),
		Outcome:         r.Outcome,
	}
	return json.NewEncoder(w).Encode(wire)
}

// validateRoleSelectionReport rejects any Report the approved selection
// model could not actually have produced — noncanonical or inconsistent
// roles, an outcome/identity combination the fixed production policy does
// not allow, or field presence that does not match the outcome's fixed
// contract — so an internal defect fails loudly at render time rather than
// emitting a malformed or misleading report. The check itself lives in
// selection.Report.Valid, so the fixed role-to-agent mapping is consulted
// through one shared, immutable helper rather than re-encoded here.
func validateRoleSelectionReport(r selection.Report) error {
	if !r.Valid() {
		return fmt.Errorf("invalid role selection report: %+v", r)
	}
	return nil
}
