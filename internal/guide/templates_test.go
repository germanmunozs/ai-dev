package guide

import (
	"reflect"
	"testing"

	"ai-dev/internal/agent"
	"ai-dev/internal/role"
	"ai-dev/internal/sdd"
)

func templateKinds(templates []Template) []TemplateKind {
	kinds := make([]TemplateKind, 0, len(templates))
	for _, template := range templates {
		kinds = append(kinds, template.Kind)
	}
	return kinds
}

func TestTemplatesSetAndOrder(t *testing.T) {
	cases := []struct {
		name         string
		outcome      sdd.Outcome
		availability Availability
		want         []TemplateKind
	}{
		{"step available", sdd.StepRecommended, Available, []TemplateKind{KindSelect, KindRun, KindTransition, KindApprove}},
		{"step unavailable", sdd.StepRecommended, Unavailable, []TemplateKind{KindSelect, KindTransition, KindApprove}},
		{"step inconsistent", sdd.StepRecommended, Inconsistent, []TemplateKind{KindTransition, KindApprove}},
		{"step without availability", sdd.StepRecommended, "", []TemplateKind{}},
		{"terminal", sdd.WorkflowTerminal, "", []TemplateKind{KindState}},
		{"artifacts inconsistent", sdd.ArtifactsInconsistent, "", []TemplateKind{}},
		{"feature unknown", sdd.FeatureUnknown, "", []TemplateKind{}},
		{"state invalid", sdd.StateInvalid, "", []TemplateKind{}},
		{"unknown outcome", sdd.Outcome("bogus"), "", []TemplateKind{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := templateKinds(Templates(tc.outcome, tc.availability, role.Planning, agent.ClaudeCode))
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("kinds = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTemplatesWordsAndSlots(t *testing.T) {
	templates := Templates(sdd.StepRecommended, Available, role.Implementation, agent.CodexCLI)
	templates = append(templates, Templates(sdd.WorkflowTerminal, "", "", "")...)
	fixed := func(text string) Word { return Word{Kind: WordFixed, Text: text} }
	want := map[TemplateKind]struct {
		label string
		words []Word
		slots []string
		flags []string
	}{
		KindSelect: {"select the agent for this role (003)",
			[]Word{fixed("ai-dev"), fixed("agent"), fixed("select"), fixed("--"), {Kind: WordToken, Text: "implementation"}},
			[]string{}, nil},
		KindRun: {"run the agent (002)",
			[]Word{fixed("ai-dev"), fixed("agent"), fixed("run"), fixed("--"), {Kind: WordToken, Text: "codex-cli"}, {Kind: WordHuman, Text: SlotInstruction}},
			[]string{SlotInstruction}, nil},
		KindTransition: {"after completing the activity, check the transition (004)",
			[]Word{fixed("ai-dev"), fixed("workflow"), fixed("transition"), fixed("--"), {Kind: WordFeature}, {Kind: WordHuman, Text: SlotTargetState}},
			[]string{SlotTargetState}, nil},
		KindApprove: {"then record your decision (006)",
			[]Word{fixed("ai-dev"), fixed("workflow"), fixed("approve"), fixed("--approver"), {Kind: WordHuman, Text: SlotApprover}, fixed("--"), {Kind: WordFeature}, {Kind: WordHuman, Text: SlotTargetState}},
			[]string{SlotApprover, SlotTargetState}, []string{"--reason"}},
		KindState: {"report the state (004)",
			[]Word{fixed("ai-dev"), fixed("workflow"), fixed("state"), fixed("--"), {Kind: WordFeature}},
			[]string{}, nil},
	}
	if len(templates) != len(want) {
		t.Fatalf("templates = %d, want %d", len(templates), len(want))
	}
	for _, template := range templates {
		expected, ok := want[template.Kind]
		if !ok {
			t.Fatalf("unexpected template kind %q", template.Kind)
		}
		if template.Label != expected.label || !reflect.DeepEqual(template.Words, expected.words) || !reflect.DeepEqual(template.HumanSlots, expected.slots) || !reflect.DeepEqual(template.OptionalFlags, expected.flags) {
			t.Fatalf("template %q = %+v, want label %q words %+v slots %v flags %v", template.Kind, template, expected.label, expected.words, expected.slots, expected.flags)
		}
		for _, word := range template.Words {
			if word.Text == "--reason" {
				t.Fatalf("template %q renders a --reason word (F007-FR-005)", template.Kind)
			}
		}
	}
}

func TestTemplatesAreFreshCopies(t *testing.T) {
	first := Templates(sdd.StepRecommended, Available, role.Planning, agent.ClaudeCode)
	first[0].Words[0].Text = "mutated"
	first[3].HumanSlots[0] = "mutated"
	second := Templates(sdd.StepRecommended, Available, role.Planning, agent.ClaudeCode)
	if second[0].Words[0].Text != "ai-dev" || second[3].HumanSlots[0] != SlotApprover {
		t.Fatalf("Templates shares state across calls: %+v", second)
	}
}
