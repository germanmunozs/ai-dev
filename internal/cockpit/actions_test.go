package cockpit

import (
	"reflect"
	"strings"
	"testing"
)

func recommendedRow() FeatureRow {
	return FeatureRow{
		Name: Sanitize("012-x"), ID: "012-x", State: Sanitize("draft"), Recognized: true,
		StepRecommended: true, AgentAvailable: true, AgentID: "claude-code",
	}
}

// hostileValues are values that must reach the command as one unmodified
// argument each (F012-FR-009, US3 scenario 5).
var hostileValues = []string{"plain", "-x", "--help", "a b", "k=v", `"quoted" 'single'`, "line\nbreak", "$(touch pwned)", ""}

// TestActionArgumentVectors proves the argument vectors of
// contracts/cockpit.md for ordinary and hostile values (SC-003).
func TestActionArgumentVectors(t *testing.T) {
	row := recommendedRow()
	for _, v := range hostileValues {
		cases := []struct {
			id     ActionID
			values []string
			want   []string
		}{
			{RunAgent, []string{v}, []string{"agent", "run", "--", "claude-code", v}},
			{Approve, []string{v, "specified", v}, approveWant(v, "specified", v)},
			{Abandon, []string{v, v}, []string{"workflow", "abandon", "--approver=" + v, "--reason=" + v, "--", "012-x", "abandoned"}},
			{CommitValidate, []string{v}, []string{"git", "commit", "validate", "--message=" + v}},
		}
		for _, c := range cases {
			got := ActionByID(c.id).Argv(row, c.values)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("%s with %q: %q, want %q", c.id, v, got, c.want)
			}
		}
	}
	if got := ActionByID(Setup).Argv(FeatureRow{}, nil); !reflect.DeepEqual(got, []string{"setup"}) {
		t.Errorf("setup: %q", got)
	}
	if got := ActionByID(Init).Argv(FeatureRow{}, nil); !reflect.DeepEqual(got, []string{"init"}) {
		t.Errorf("init: %q", got)
	}
	hostileID := FeatureRow{ID: "-rf \x1b]52;c;x\x07", Recognized: true}
	if got := ActionByID(Approve).Argv(hostileID, []string{"a", "planned", ""}); got[len(got)-3] != "--" || got[len(got)-2] != hostileID.ID {
		t.Errorf("feature identifier not passed raw after --: %q", got)
	}
}

func approveWant(approver, target, reason string) []string {
	args := []string{"workflow", "approve", "--approver=" + approver}
	if reason != "" {
		args = append(args, "--reason="+reason)
	}
	return append(args, "--", "012-x", target)
}

// TestActionAvailability proves the rules of F012-FR-011, each with a reason
// when unavailable.
func TestActionAvailability(t *testing.T) {
	recommended := recommendedRow()
	unavailableAgent := recommended
	unavailableAgent.AgentAvailable = false
	unavailableAgent.Availability = Sanitize("unavailable")
	noStep := recommended
	noStep.StepRecommended = false
	noStep.Next = Sanitize("artifacts_inconsistent")
	terminal := recommended
	terminal.Terminal, terminal.StepRecommended = true, false
	unrecognized := FeatureRow{ID: "x", State: Sanitize("state_invalid")}

	cases := []struct {
		id   ActionID
		row  *FeatureRow
		want bool
	}{
		{RunAgent, &recommended, true},
		{RunAgent, &unavailableAgent, false},
		{RunAgent, &noStep, false},
		{RunAgent, &terminal, false},
		{RunAgent, nil, false},
		{Approve, &recommended, true},
		{Approve, &terminal, false},
		{Approve, &unrecognized, false},
		{Approve, nil, false},
		{Abandon, &noStep, true},
		{Abandon, &terminal, false},
		{Abandon, &unrecognized, false},
		{Setup, nil, true},
		{Init, nil, true},
		{CommitValidate, nil, true},
		{Setup, &terminal, true},
	}
	for _, c := range cases {
		ok, reason := ActionByID(c.id).Available(c.row)
		if ok != c.want {
			t.Errorf("%s on %+v: available=%t, want %t", c.id, c.row, ok, c.want)
		}
		if !ok && reason == "" {
			t.Errorf("%s: unavailable without a reason", c.id)
		}
		if ok && reason != "" {
			t.Errorf("%s: available with reason %q", c.id, reason)
		}
	}
	if _, reason := ActionByID(RunAgent).Available(&unavailableAgent); !strings.Contains(reason, "unavailable") {
		t.Errorf("reason does not name the availability: %q", reason)
	}
}

// TestRequiredFieldsRefuseEmptyValues: approve needs approver and target,
// abandon needs approver and reason, and nothing is prefilled.
func TestRequiredFieldsRefuseEmptyValues(t *testing.T) {
	cases := []struct {
		id      ActionID
		values  []string
		missing string
	}{
		{RunAgent, []string{""}, "instruction"},
		{Approve, []string{"", "planned", ""}, "approver"},
		{Approve, []string{"me", "", ""}, "target state"},
		{Abandon, []string{"me", ""}, "reason"},
		{CommitValidate, []string{""}, "message"},
	}
	for _, c := range cases {
		err := ActionByID(c.id).Validate(c.values)
		if err == "" || !strings.Contains(err, c.missing) {
			t.Errorf("%s %q: %q, want a refusal naming %q", c.id, c.values, err, c.missing)
		}
	}
	if err := ActionByID(Approve).Validate([]string{"me", "planned", ""}); err != "" {
		t.Errorf("approve with an empty optional reason refused: %s", err)
	}
	for _, action := range Actions() {
		if len(action.Fields) != len(action.emptyValues()) {
			t.Fatalf("%s: field count", action.ID)
		}
		for _, field := range action.Fields {
			if field.Label == "" {
				t.Errorf("%s: unlabeled field", action.ID)
			}
		}
	}
}

func TestEveryActionIsListedOnce(t *testing.T) {
	seen := map[ActionID]bool{}
	for _, action := range Actions() {
		if seen[action.ID] || action.Title == "" {
			t.Errorf("action %q duplicated or untitled", action.ID)
		}
		seen[action.ID] = true
	}
	for _, id := range []ActionID{RunAgent, Approve, Abandon, Setup, Init, CommitValidate} {
		if !seen[id] {
			t.Errorf("action %s missing", id)
		}
	}
}
