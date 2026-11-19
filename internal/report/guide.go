package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"ai-dev/internal/guide"
	"ai-dev/internal/sdd"
)

// WorkflowGuideSchemaVersion identifies the machine-readable guidance report.
const WorkflowGuideSchemaVersion = "ai-dev.workflow-guide/v1"

// RenderGuideCommand renders one template for pasting into a POSIX shell:
// fixed words and canonical tokens bare, the feature identifier as one
// single-quoted word (R-04), and each human slot as an unquoted (name), which
// a shell rejects as a syntax error before running anything (R-03).
func RenderGuideCommand(featureID string, template guide.Template) string {
	words := make([]string, 0, len(template.Words))
	for _, word := range template.Words {
		switch word.Kind {
		case guide.WordFeature:
			words = append(words, shellQuote(featureID))
		case guide.WordHuman:
			words = append(words, "("+word.Text+")")
		default:
			words = append(words, word.Text)
		}
	}
	return strings.Join(words, " ")
}

// WriteGuideText writes one human-readable guidance report in a single write.
func WriteGuideText(w io.Writer, report guide.Report) error {
	if !report.Valid() {
		return fmt.Errorf("invalid workflow guide report: %+v", report)
	}
	var out strings.Builder
	switch report.Outcome {
	case sdd.StepRecommended:
		fmt.Fprintf(&out, "ai-dev: outcome=%s feature=%q state=%s activity=%s role=%s agent=%s availability=%s note=%q\n", report.Outcome, report.FeatureID, report.State, report.Activity, report.Role, report.Agent, report.Availability, report.Statement)
	case sdd.WorkflowTerminal:
		fmt.Fprintf(&out, "ai-dev: outcome=%s feature=%q state=%s note=%q\n", report.Outcome, report.FeatureID, report.State, report.Statement)
	case sdd.ArtifactsInconsistent:
		fmt.Fprintf(&out, "ai-dev: outcome=%s feature=%q state=%s missing=%s note=%q\n", report.Outcome, report.FeatureID, report.State, strings.Join(report.Missing, ","), report.Statement)
	case sdd.FeatureUnknown, sdd.StateInvalid:
		fmt.Fprintf(&out, "ai-dev: outcome=%s feature=%q note=%q\n", report.Outcome, report.FeatureID, report.Statement)
	default:
		return fmt.Errorf("workflow guide outcome %q cannot be rendered", report.Outcome)
	}
	humanSlots, reason := false, false
	for _, template := range report.Templates {
		fmt.Fprintf(&out, "  %s: %s\n", template.Label, RenderGuideCommand(report.FeatureID, template))
		humanSlots = humanSlots || len(template.HumanSlots) > 0
		reason = reason || template.Kind == guide.KindApprove
	}
	if humanSlots {
		out.WriteString("  replace each (slot) before running")
		if reason {
			out.WriteString("; --reason (reason) is optional for approval (006 FR-010)")
		}
		out.WriteString("\n")
	}
	_, err := io.WriteString(w, out.String())
	return err
}

type guideTemplateWire struct {
	Kind          guide.TemplateKind `json:"kind"`
	Label         string             `json:"label"`
	Command       string             `json:"command"`
	HumanSlots    []string           `json:"human_slots"`
	OptionalFlags []string           `json:"optional_flags,omitempty"`
}

// WriteGuideJSON writes exactly one machine-readable guidance report. Fields
// shared with 005 keep its names and per-outcome presence.
func WriteGuideJSON(w io.Writer, report guide.Report) error {
	if !report.Valid() {
		return fmt.Errorf("invalid workflow guide report: %+v", report)
	}
	wire := struct {
		SchemaVersion     string              `json:"schema_version"`
		FeatureID         string              `json:"feature_id"`
		Outcome           sdd.Outcome         `json:"outcome"`
		CurrentState      string              `json:"current_state,omitempty"`
		Activity          string              `json:"activity,omitempty"`
		Role              string              `json:"role,omitempty"`
		MissingArtifacts  []string            `json:"missing_artifacts,omitempty"`
		Agent             string              `json:"agent,omitempty"`
		AgentAvailability string              `json:"agent_availability,omitempty"`
		Templates         []guideTemplateWire `json:"templates"`
		Statement         string              `json:"statement"`
	}{
		SchemaVersion:     WorkflowGuideSchemaVersion,
		FeatureID:         report.FeatureID,
		Outcome:           report.Outcome,
		CurrentState:      string(report.State),
		Activity:          string(report.Activity),
		Role:              string(report.Role),
		MissingArtifacts:  report.Missing,
		Agent:             string(report.Agent),
		AgentAvailability: string(report.Availability),
		Templates:         make([]guideTemplateWire, 0, len(report.Templates)),
		Statement:         report.Statement,
	}
	for _, template := range report.Templates {
		slots := append([]string{}, template.HumanSlots...)
		wire.Templates = append(wire.Templates, guideTemplateWire{
			Kind:          template.Kind,
			Label:         template.Label,
			Command:       RenderGuideCommand(report.FeatureID, template),
			HumanSlots:    slots,
			OptionalFlags: template.OptionalFlags,
		})
	}
	return json.NewEncoder(w).Encode(wire)
}
