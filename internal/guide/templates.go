package guide

import (
	"ai-dev/internal/agent"
	"ai-dev/internal/role"
	"ai-dev/internal/sdd"
)

// TemplateKind identifies one command template of F007-FR-005.
type TemplateKind string

const (
	KindSelect     TemplateKind = "select"
	KindRun        TemplateKind = "run"
	KindTransition TemplateKind = "transition"
	KindApprove    TemplateKind = "approve"
	KindState      TemplateKind = "state"
)

// WordKind says how one template word is rendered.
type WordKind uint8

const (
	// WordFixed is a literal command word such as ai-dev, workflow or --.
	WordFixed WordKind = iota
	// WordToken is a canonical role or agent identity.
	WordToken
	// WordFeature is the feature identifier, rendered as one quoted word.
	WordFeature
	// WordHuman is a slot only the human may fill; Text is its name.
	WordHuman
)

// Human slot names (F007-FR-005). Guidance never fills them.
const (
	SlotInstruction = "instruction"
	SlotApprover    = "approver"
	SlotTargetState = "target-state"
)

// Word is one word of a command template.
type Word struct {
	Kind WordKind
	Text string
}

// Template is one command a human may complete and run. It is never run by
// guidance.
type Template struct {
	Kind          TemplateKind
	Label         string
	Words         []Word
	HumanSlots    []string
	OptionalFlags []string
}

// Templates returns the fixed F007-FR-005 template set, in order, for one
// outcome. For step_recommended the set depends on availability; with no
// availability it is empty, because guidance has no agent to report.
func Templates(outcome sdd.Outcome, availability Availability, nextRole role.Role, agentID agent.ID) []Template {
	switch outcome {
	case sdd.StepRecommended:
		templates := []Template{}
		switch availability {
		case Available:
			templates = append(templates, selectTemplate(nextRole), runTemplate(agentID))
		case Unavailable:
			templates = append(templates, selectTemplate(nextRole))
		case Inconsistent:
		default:
			return []Template{}
		}
		return append(templates, transitionTemplate(), approveTemplate())
	case sdd.WorkflowTerminal:
		return []Template{stateTemplate()}
	default:
		return []Template{}
	}
}

func fixed(texts ...string) []Word {
	words := make([]Word, 0, len(texts))
	for _, text := range texts {
		words = append(words, Word{Kind: WordFixed, Text: text})
	}
	return words
}

func selectTemplate(nextRole role.Role) Template {
	return Template{
		Kind:       KindSelect,
		Label:      "select the agent for this role (003)",
		Words:      append(fixed("ai-dev", "agent", "select", "--"), Word{Kind: WordToken, Text: string(nextRole)}),
		HumanSlots: []string{},
	}
}

func runTemplate(agentID agent.ID) Template {
	return Template{
		Kind:       KindRun,
		Label:      "run the agent (002)",
		Words:      append(fixed("ai-dev", "agent", "run", "--"), Word{Kind: WordToken, Text: string(agentID)}, Word{Kind: WordHuman, Text: SlotInstruction}),
		HumanSlots: []string{SlotInstruction},
	}
}

func transitionTemplate() Template {
	return Template{
		Kind:       KindTransition,
		Label:      "after completing the activity, check the transition (004)",
		Words:      append(fixed("ai-dev", "workflow", "transition", "--"), Word{Kind: WordFeature}, Word{Kind: WordHuman, Text: SlotTargetState}),
		HumanSlots: []string{SlotTargetState},
	}
}

func approveTemplate() Template {
	words := append(fixed("ai-dev", "workflow", "approve", "--approver"), Word{Kind: WordHuman, Text: SlotApprover})
	words = append(words, Word{Kind: WordFixed, Text: "--"}, Word{Kind: WordFeature}, Word{Kind: WordHuman, Text: SlotTargetState})
	return Template{
		Kind:          KindApprove,
		Label:         "then record your decision (006)",
		Words:         words,
		HumanSlots:    []string{SlotApprover, SlotTargetState},
		OptionalFlags: []string{"--reason"},
	}
}

func stateTemplate() Template {
	return Template{
		Kind:       KindState,
		Label:      "report the state (004)",
		Words:      append(fixed("ai-dev", "workflow", "state", "--"), Word{Kind: WordFeature}),
		HumanSlots: []string{},
	}
}
