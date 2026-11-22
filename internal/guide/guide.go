package guide

import (
	"io/fs"

	"github.com/germanmunozs/ai-dev/internal/agent"
	"github.com/germanmunozs/ai-dev/internal/role"
	"github.com/germanmunozs/ai-dev/internal/sdd"
	"github.com/germanmunozs/ai-dev/internal/selection"
)

// Availability is the reported state of the agent a role maps to under the
// role assignment in effect (F007-FR-004, F011 Amendment A2). It is
// information only.
type Availability string

const (
	Available    Availability = "available"
	Unavailable  Availability = "unavailable"
	Inconsistent Availability = "inconsistent"
	// ConfigurationInvalid: the project configuration is invalid, so no
	// agent is named (F011-FR-016).
	ConfigurationInvalid Availability = "configuration_invalid"
)

// Statement is the fixed non-authorization statement every report carries
// (F007-FR-008).
const Statement = "options for the human only; not authorization. Running an agent or a review is not approval; approval is your own Human Gate decision after completing the activity"

// Selector resolves one role under the role assignment in effect. selection.Selector
// satisfies it; guide never constructs one, so it holds no discovery path of
// its own.
type Selector interface {
	Select(suppliedRole string) selection.Report
}

// Report is one guidance report. The embedded sdd.Report is exactly 005's
// next-step report for the same inputs (F007-FR-002).
type Report struct {
	sdd.Report
	Agent        agent.ID
	Availability Availability
	// Source is where the assignment comes from, for step_recommended only.
	Source    selection.Source
	Templates []Template
	Statement string
}

// Evaluate resolves one read-only guidance request. It consults selector
// only for step_recommended, with exactly 005's role.
func Evaluate(root fs.FS, featureID string, selector Selector) Report {
	report := Report{Report: sdd.Evaluate(root, featureID), Statement: Statement}
	if report.Outcome == sdd.StepRecommended {
		report.Agent, report.Availability, report.Source = availabilityOf(report.Role, selector.Select(string(report.Role)))
	}
	report.Templates = []Template{}
	if TemplateSafe(featureID) {
		report.Templates = Templates(report.Outcome, report.Availability, report.Role, report.Agent)
	}
	return report
}

// availabilityOf maps 003's report to F007-FR-004's availability (R-02) and
// its source (F011-FR-017). Any other outcome, including role_unsupported,
// yields no availability, which makes the report invalid.
func availabilityOf(nextRole role.Role, selected selection.Report) (agent.ID, Availability, selection.Source) {
	if selected.RequestedRole != nextRole {
		return "", "", ""
	}
	switch selected.Outcome {
	case selection.RoleSelected:
		return selected.SelectedAgentID, Available, selected.Source
	case selection.AgentUnavailable:
		return selected.MappedAgentID, Unavailable, selected.Source
	case selection.SelectionInconsistent:
		return selected.MappedAgentID, Inconsistent, selected.Source
	case selection.ConfigurationInvalid:
		return "", ConfigurationInvalid, selected.Source
	default:
		return "", "", ""
	}
}

// Valid reports whether r satisfies the data model: a valid 005 report, the
// fixed statement, an agent and availability exactly for step_recommended,
// canonical tokens, and exactly the F007-FR-005 templates (none for an
// identifier that is not template-safe).
func (r Report) Valid() bool {
	if !r.Report.Valid() || r.Statement != Statement {
		return false
	}
	if r.Outcome == sdd.StepRecommended {
		switch r.Availability {
		case Available, Unavailable:
			if !canonicalToken(string(r.Agent)) || (r.Source != selection.SourceBuiltIn && r.Source != selection.SourceProject) {
				return false
			}
		case Inconsistent:
			if !canonicalToken(string(r.Agent)) || r.Source != selection.SourceBuiltIn {
				return false
			}
		case ConfigurationInvalid:
			if r.Agent != "" || r.Source != selection.SourceProject {
				return false
			}
		default:
			return false
		}
	} else if r.Agent != "" || r.Availability != "" || r.Source != "" {
		return false
	}
	want := []Template{}
	if TemplateSafe(r.FeatureID) {
		want = Templates(r.Outcome, r.Availability, r.Role, r.Agent)
	}
	if len(r.Templates) != len(want) {
		return false
	}
	for i := range want {
		if !sameTemplate(r.Templates[i], want[i]) {
			return false
		}
		for _, word := range r.Templates[i].Words {
			if word.Kind == WordToken && !canonicalToken(word.Text) {
				return false
			}
		}
	}
	return true
}

// canonicalToken reports whether value can be rendered bare in a POSIX shell:
// non-empty lowercase ASCII letters, digits and '-'.
func canonicalToken(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}

func sameTemplate(a, b Template) bool {
	return a.Kind == b.Kind && a.Label == b.Label && sameWords(a.Words, b.Words) && sameStrings(a.HumanSlots, b.HumanSlots) && sameStrings(a.OptionalFlags, b.OptionalFlags)
}

func sameWords(a, b []Word) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
