package report

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
)

// SetupSchemaVersion identifies the machine-readable setup report.
const SetupSchemaVersion = "ai-dev.setup/v1"

// SetupOutcome is one outcome of ai-dev setup (F011-FR-009 to FR-012).
type SetupOutcome string

const (
	NotConfigured          SetupOutcome = "not_configured"
	Configured             SetupOutcome = "configured"
	ConfigurationInvalid   SetupOutcome = "configuration_invalid"
	ConfigurationWritten   SetupOutcome = "configuration_written"
	ConfigurationUnchanged SetupOutcome = "configuration_unchanged"
	ConfigurationRejected  SetupOutcome = "configuration_rejected"
	ConfigurationExists    SetupOutcome = "configuration_exists"
	SetupCancelled         SetupOutcome = "setup_cancelled"
	PersistenceFailed      SetupOutcome = "persistence_failed"
)

// SetupAgent is one catalog agent in a setup report. It has no field for an
// executable path, so none can be reported (F011-FR-014).
type SetupAgent struct {
	ID           string
	Label        string
	Availability string
	Chosen       bool
}

// SetupRole is one role of the assignment in effect.
type SetupRole struct {
	Role  string
	Agent string
}

// SetupReason is one reason a configuration is invalid or rejected. Value is
// untrusted and is quoted when rendered as text.
type SetupReason struct {
	Code  string
	Value string
}

// SetupReport is the result of one setup command (contracts/setup-cli.md).
type SetupReport struct {
	Command string // show, apply or setup
	Outcome SetupOutcome
	Source  string // built_in, project, or empty
	Agents  []SetupAgent
	Roles   []SetupRole
	Reasons []SetupReason
}

var setupOutcomes = map[string][]SetupOutcome{
	"show":  {NotConfigured, Configured, ConfigurationInvalid},
	"apply": {ConfigurationWritten, ConfigurationUnchanged, ConfigurationRejected, ConfigurationExists, PersistenceFailed},
	"setup": {ConfigurationWritten, ConfigurationUnchanged, SetupCancelled, PersistenceFailed},
}

// Valid rejects a report the setup commands could not have produced.
func (r SetupReport) Valid() bool {
	if !slices.Contains(setupOutcomes[r.Command], r.Outcome) || len(r.Agents) == 0 {
		return false
	}
	chosen := false
	for _, a := range r.Agents {
		if a.ID == "" || a.Label == "" || (a.Availability != "available" && a.Availability != "unavailable") {
			return false
		}
		chosen = chosen || a.Chosen
	}
	switch r.Outcome {
	case NotConfigured:
		return r.Source == "built_in" && len(r.Roles) == 5 && !chosen && len(r.Reasons) == 0
	case Configured, ConfigurationWritten, ConfigurationUnchanged:
		return r.Source == "project" && len(r.Roles) == 5 && chosen && len(r.Reasons) == 0
	case ConfigurationInvalid, ConfigurationRejected:
		return r.Source == "" && len(r.Roles) == 0 && !chosen && len(r.Reasons) > 0
	default: // configuration_exists, setup_cancelled, persistence_failed
		return r.Source == "" && len(r.Roles) == 0 && !chosen && len(r.Reasons) == 0
	}
}

// SetupReasonLine renders one reason as an indented line, with its untrusted
// value in the 004 shell-inert quoted form, so it cannot split the report or
// fake an outcome.
func SetupReasonLine(reason SetupReason) string {
	line := "  reason " + reason.Code
	if reason.Value != "" {
		line += " " + shellInertQuote(reason.Value)
	}
	return line + "\n"
}

func (a SetupAgent) warning() string {
	if a.Chosen && a.Availability == "unavailable" {
		return "chosen_agent_unavailable"
	}
	return ""
}

// WriteSetupText writes the line-oriented setup report in one write.
func WriteSetupText(w io.Writer, r SetupReport) error {
	if !r.Valid() {
		return fmt.Errorf("invalid setup report: %+v", r)
	}
	var out strings.Builder
	fmt.Fprintf(&out, "ai-dev: outcome=%s", r.Outcome)
	if r.Source != "" {
		fmt.Fprintf(&out, " source=%s", r.Source)
	}
	out.WriteString("\n")
	for _, a := range r.Agents {
		chosen := "no"
		if a.Chosen {
			chosen = "yes"
		}
		fmt.Fprintf(&out, "  agent %s availability=%s chosen=%s", a.ID, a.Availability, chosen)
		if warning := a.warning(); warning != "" {
			fmt.Fprintf(&out, " warning=%s", warning)
		}
		out.WriteString("\n")
	}
	for _, role := range r.Roles {
		fmt.Fprintf(&out, "  role %s agent=%s\n", role.Role, role.Agent)
	}
	for _, reason := range r.Reasons {
		out.WriteString(SetupReasonLine(reason))
	}
	_, err := io.WriteString(w, out.String())
	return err
}

// WriteSetupJSON writes exactly one compact ai-dev.setup/v1 object.
func WriteSetupJSON(w io.Writer, r SetupReport) error {
	if !r.Valid() {
		return fmt.Errorf("invalid setup report: %+v", r)
	}
	type wireAgent struct {
		ID           string `json:"id"`
		Label        string `json:"label"`
		Availability string `json:"availability"`
		Chosen       bool   `json:"chosen"`
	}
	type wireRole struct {
		Role  string `json:"role"`
		Agent string `json:"agent"`
	}
	type wireReason struct {
		Code  string `json:"code"`
		Value string `json:"value,omitempty"`
	}
	wire := struct {
		SchemaVersion string       `json:"schema_version"`
		Command       string       `json:"command"`
		Outcome       SetupOutcome `json:"outcome"`
		Source        string       `json:"source,omitempty"`
		Agents        []wireAgent  `json:"agents"`
		Roles         []wireRole   `json:"roles"`
		Warnings      []string     `json:"warnings"`
		Reasons       []wireReason `json:"reasons"`
	}{SchemaVersion: SetupSchemaVersion, Command: r.Command, Outcome: r.Outcome, Source: r.Source,
		Agents: []wireAgent{}, Roles: []wireRole{}, Warnings: []string{}, Reasons: []wireReason{}}
	for _, a := range r.Agents {
		wire.Agents = append(wire.Agents, wireAgent(a))
		if warning := a.warning(); warning != "" {
			wire.Warnings = append(wire.Warnings, warning+":"+a.ID)
		}
	}
	for _, role := range r.Roles {
		wire.Roles = append(wire.Roles, wireRole(role))
	}
	for _, reason := range r.Reasons {
		wire.Reasons = append(wire.Reasons, wireReason(reason))
	}
	return json.NewEncoder(w).Encode(wire)
}
