package cockpit

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func filesRow(files ...string) FeatureRow {
	row := recommendedRow()
	row.Files = files
	return row
}

// The four "open" actions follow the existing ones, in this order (A3,
// owner decision I3-A, contracts/open.md).
func TestOpenActionsFollowTheExistingOnes(t *testing.T) {
	var ids []ActionID
	for _, action := range Actions() {
		ids = append(ids, action.ID)
	}
	want := []ActionID{RunAgent, Approve, Abandon, Setup, Init, CommitValidate, OpenSpec, OpenPlan, OpenTasks, OpenFile}
	if !slices.Equal(ids, want) {
		t.Fatalf("actions %v, want %v", ids, want)
	}
	for _, id := range want[6:] {
		action := ActionByID(id)
		if !action.Feature || action.Changes != "nothing in the project; opens the file in Neovim" {
			t.Errorf("%s: %+v", id, action)
		}
	}
}

func TestOpenActionsAvailability(t *testing.T) {
	cases := []struct {
		id     ActionID
		row    *FeatureRow
		ok     bool
		reason string
	}{
		{OpenSpec, ptr(filesRow("spec.md")), true, ""},
		{OpenSpec, ptr(filesRow("plan.md")), false, "no spec.md in this feature"},
		{OpenPlan, ptr(filesRow("spec.md", "plan.md")), true, ""},
		{OpenPlan, ptr(filesRow("spec.md")), false, "no plan.md in this feature"},
		{OpenTasks, ptr(filesRow("tasks.md")), true, ""},
		{OpenTasks, ptr(filesRow()), false, "no tasks.md in this feature"},
		{OpenFile, ptr(filesRow("notes.txt")), true, ""},
		{OpenFile, ptr(filesRow()), false, "no files in this feature"},
		{OpenSpec, nil, false, "no feature is selected"},
		{OpenFile, nil, false, "no feature is selected"},
	}
	for _, c := range cases {
		ok, reason := ActionByID(c.id).Available(c.row)
		if ok != c.ok || reason != c.reason {
			t.Errorf("%s on %+v: %t %q, want %t %q", c.id, c.row, ok, reason, c.ok, c.reason)
		}
	}
	// Opening a file is harmless: a terminal or unrecognized state does
	// not hide it.
	row := filesRow("spec.md")
	row.Terminal, row.Recognized, row.StepRecommended = true, false, false
	if ok, _ := ActionByID(OpenSpec).Available(&row); !ok {
		t.Error("open spec unavailable for a terminal feature")
	}
}

func ptr(row FeatureRow) *FeatureRow { return &row }

func TestOpenActionsArgumentVectors(t *testing.T) {
	row := filesRow("spec.md", "plan.md", "tasks.md", "-x b.md")
	row.ID = "-014 a b"
	for id, file := range map[ActionID]string{OpenSpec: "spec.md", OpenPlan: "plan.md", OpenTasks: "tasks.md"} {
		action := ActionByID(id)
		if len(action.Fields) != 0 {
			t.Errorf("%s takes input", id)
		}
		if got, want := action.Argv(row, nil), []string{"open", "--", "specs/-014 a b/" + file}; !slices.Equal(got, want) {
			t.Errorf("%s: %q, want %q", id, got, want)
		}
	}
	open := ActionByID(OpenFile)
	if len(open.Fields) != 1 || open.Fields[0].Label != "file" || !open.Fields[0].Required {
		t.Fatalf("open file fields %+v", open.Fields)
	}
	if got, want := open.Argv(row, []string{"-x b.md"}), []string{"open", "--", "specs/-014 a b/-x b.md"}; !slices.Equal(got, want) {
		t.Errorf("open file: %q, want %q", got, want)
	}
}

// The file must be one of the feature's regular files, exactly
// (contracts/open.md).
func TestOpenFileValidation(t *testing.T) {
	row := filesRow("Notes.md", "spec.md")
	open := ActionByID(OpenFile)
	for _, value := range []string{"Notes.md", "spec.md"} {
		if refusal := open.ValidateFor(row, []string{value}); refusal != "" {
			t.Errorf("%q refused: %s", value, refusal)
		}
	}
	if refusal := open.ValidateFor(row, []string{""}); refusal != "the file is required" {
		t.Errorf("empty value: %q", refusal)
	}
	for _, value := range []string{"notes.md", "../x", "sub/y", "/etc/hosts", "spec.md ", "missing.md"} {
		if refusal := open.ValidateFor(row, []string{value}); refusal != "not a file of this feature" {
			t.Errorf("%q: %q", value, refusal)
		}
	}
	// Other actions validate as before.
	if refusal := ActionByID(Approve).ValidateFor(row, []string{"me", "planned", ""}); refusal != "" {
		t.Errorf("approve: %q", refusal)
	}
}

func TestOpenFileHint(t *testing.T) {
	var files []string
	for i := 1; i <= 25; i++ {
		files = append(files, fmt.Sprintf("f%02d.md", i))
	}
	row := filesRow(files...)
	hint := ActionByID(OpenFile).hintFor(row, 0)
	if !strings.HasPrefix(hint, "f01.md, f02.md") || !strings.Contains(hint, "f20.md … and 5 more") || strings.Contains(hint, "f21.md") {
		t.Fatalf("hint %q", hint)
	}
	if refusal := ActionByID(OpenFile).ValidateFor(row, []string{"f25.md"}); refusal != "" {
		t.Fatalf("an unlisted file of the feature refused: %s", refusal)
	}
	hostile := filesRow("b\x1b[31m.md", "a.md")
	hint = ActionByID(OpenFile).hintFor(hostile, 0)
	if hint != `a.md, b\x1b[31m.md` {
		t.Fatalf("hostile hint %q", hint)
	}
	if got := ActionByID(Approve).hintFor(row, 1); got != stateHint {
		t.Fatalf("approve hint %q", got)
	}
}
