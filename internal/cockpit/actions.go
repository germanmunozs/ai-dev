package cockpit

import "fmt"

// ActionID names one action (data-model.md).
type ActionID string

const (
	RunAgent       ActionID = "run-agent"
	Approve        ActionID = "approve"
	Abandon        ActionID = "abandon"
	Setup          ActionID = "setup"
	Init           ActionID = "init"
	CommitValidate ActionID = "commit-validate"
)

// Field is one value the person types; none is ever prefilled
// (F012-FR-009).
type Field struct {
	Label    string
	Required bool
	Hint     string
}

// Action is one existing command the cockpit can run (contracts/cockpit.md).
type Action struct {
	ID    ActionID
	Title string
	// Feature: the action applies to the selected feature.
	Feature bool
	// Changes says what the command changes, shown in the review.
	Changes string
	Fields  []Field
}

// stateHint lists the canonical Workflow States for the approve form. It is
// a hint only: the command validates the value.
const stateHint = "draft, specified, planned, tasked, implemented, approved or abandoned"

var actions = []Action{
	{ID: RunAgent, Title: "Run the agent of the next step", Feature: true,
		Changes: "starts the agent; whatever the agent does, under its own sandbox",
		Fields:  []Field{{Label: "instruction", Required: true, Hint: "passed to the agent as one argument"}}},
	{ID: Approve, Title: "Approve the feature", Feature: true, Changes: "records your decision in workflow-state.json",
		Fields: []Field{
			{Label: "approver", Required: true, Hint: "your name, recorded as declared"},
			{Label: "target state", Required: true, Hint: stateHint},
			{Label: "reason", Hint: "optional"},
		}},
	{ID: Abandon, Title: "Abandon the feature", Feature: true, Changes: "records your decision in workflow-state.json",
		Fields: []Field{
			{Label: "approver", Required: true, Hint: "your name, recorded as declared"},
			{Label: "reason", Required: true},
		}},
	{ID: Setup, Title: "Configure agents", Changes: "the project configuration file .ai-dev/agents.json, only after its own confirmation"},
	{ID: Init, Title: "Initialize repository", Changes: "the repository and its project configuration, only after its own confirmation"},
	{ID: CommitValidate, Title: "Validate a commit message", Changes: "nothing",
		Fields: []Field{{Label: "message", Required: true}}},
}

// Actions lists every action in a stable order.
func Actions() []Action { return actions }

// ActionByID returns the action with id; it panics on an unknown id, which
// is a programming error.
func ActionByID(id ActionID) Action {
	for _, action := range actions {
		if action.ID == id {
			return action
		}
	}
	panic("cockpit: unknown action " + string(id))
}

// Available applies F012-FR-011 to row, the selected feature or nil, and
// gives the reason when the action is unavailable.
func (a Action) Available(row *FeatureRow) (bool, string) {
	if !a.Feature {
		return true, ""
	}
	if row == nil {
		return false, "no feature is selected"
	}
	switch a.ID {
	case RunAgent:
		switch {
		case row.Terminal:
			return false, "the feature is in a terminal state"
		case !row.StepRecommended:
			return false, fmt.Sprintf("no step is recommended (%s)", row.Next)
		case !row.AgentAvailable:
			return false, fmt.Sprintf("the agent of the next step is %s", row.Availability)
		}
	default:
		switch {
		case !row.Recognized:
			return false, fmt.Sprintf("the feature has no recognized state (%s)", row.State)
		case row.Terminal:
			return false, "the feature is in a terminal state"
		}
	}
	return true, ""
}

// Validate returns the refusal for missing required values, or "".
func (a Action) Validate(values []string) string {
	for i, field := range a.Fields {
		if field.Required && (i >= len(values) || values[i] == "") {
			return "the " + field.Label + " is required"
		}
	}
	return ""
}

// Argv is the exact argument vector after "ai-dev" (contracts/cockpit.md):
// each value is one argument, options as --name=value and positional
// values after --, so no value can become an option or split.
func (a Action) Argv(row FeatureRow, values []string) []string {
	value := func(i int) string {
		if i < len(values) {
			return values[i]
		}
		return ""
	}
	switch a.ID {
	case RunAgent:
		return []string{"agent", "run", "--", row.AgentID, value(0)}
	case Approve:
		args := []string{"workflow", "approve", "--approver=" + value(0)}
		if reason := value(2); reason != "" {
			args = append(args, "--reason="+reason)
		}
		return append(args, "--", row.ID, value(1))
	case Abandon:
		return []string{"workflow", "abandon", "--approver=" + value(0), "--reason=" + value(1), "--", row.ID, "abandoned"}
	case Setup:
		return []string{"setup"}
	case Init:
		return []string{"init"}
	case CommitValidate:
		return []string{"git", "commit", "validate", "--message=" + value(0)}
	}
	panic("cockpit: unknown action " + string(a.ID))
}

func (a Action) emptyValues() []string { return make([]string, len(a.Fields)) }
