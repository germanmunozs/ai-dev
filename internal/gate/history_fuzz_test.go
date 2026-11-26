package gate

import (
	"testing"
)

func FuzzReadHistory(f *testing.F) {
	for _, seed := range []string{
		`{"state":"planned"}`,
		`{"state":"approved","human_gates":[{"feature_id":"f","decision":"approval","approver":"a","from_state":"implemented","to_state":"approved","decided_at":"2026-11-25T14:32:07Z","reason":"ok"}]}`,
		`{"human_gates":[],"human_gates":[]}`,
		`{"Human_Gates":[]}`,
		`{"human_gates":null}`,
		`{"human_gates":["x"]}`,
		`{"human_gates":[{"decision":"approve"}]}`,
		`[]`,
		`{"human_gates":[`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		root := historyFS(string(data))
		entries, ok := ReadHistory(root, "f")
		if !ok {
			return
		}
		for _, entry := range entries {
			if entry.Decision == "" || entry.FromState == "" || entry.ToState == "" || entry.DecidedAt.IsZero() {
				t.Fatalf("accepted an incomplete entry %+v from %q", entry, data)
			}
		}
	})
}
