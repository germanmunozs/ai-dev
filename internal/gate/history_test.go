package gate

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"github.com/germanmunozs/ai-dev/internal/workflow"
)

const historyRecordPath = "specs/f/workflow-state.json"

func historyFS(record string) fstest.MapFS {
	return fstest.MapFS{
		"specs/f":         {Mode: os.ModeDir | 0o755},
		historyRecordPath: {Data: []byte(record)},
	}
}

func TestReadHistoryMissingRecordIsEmpty(t *testing.T) {
	root := fstest.MapFS{"specs/f": {Mode: os.ModeDir | 0o755}}
	entries, ok := ReadHistory(root, "f")
	if !ok || entries == nil || len(entries) != 0 {
		t.Fatalf("ReadHistory = %v, %v; want empty, true", entries, ok)
	}
}

func TestReadHistoryWithoutHumanGatesIsEmpty(t *testing.T) {
	entries, ok := ReadHistory(historyFS(`{"state":"planned"}`), "f")
	if !ok || len(entries) != 0 {
		t.Fatalf("ReadHistory = %v, %v; want empty, true", entries, ok)
	}
}

func TestReadHistoryDecodesEntries(t *testing.T) {
	record := `{"state":"abandoned","human_gates":[` +
		`{"feature_id":"f","decision":"approval","approver":"ana@example.com","from_state":"implemented","to_state":"approved","decided_at":"2026-11-25T14:32:07Z"},` +
		`{"feature_id":"f","decision":"abandonment","approver":"bo","from_state":"draft","to_state":"abandoned","decided_at":"2026-11-26T08:00:00+01:00","reason":"Ya no \"aplica\"\n"}]}`
	entries, ok := ReadHistory(historyFS(record), "f")
	if !ok || len(entries) != 2 {
		t.Fatalf("ReadHistory = %v, %v; want two entries", entries, ok)
	}
	want := []Evidence{
		{FeatureID: "f", Decision: Approval, Approver: "ana@example.com", FromState: workflow.Implemented, ToState: workflow.Approved,
			DecidedAt: time.Date(2026, 11, 25, 14, 32, 7, 0, time.UTC)},
		{FeatureID: "f", Decision: Abandonment, Approver: "bo", FromState: workflow.Draft, ToState: workflow.Abandoned,
			DecidedAt: time.Date(2026, 11, 26, 7, 0, 0, 0, time.UTC), Reason: "Ya no \"aplica\"\n", ReasonSet: true},
	}
	for i := range want {
		got := entries[i]
		if got.FeatureID != want[i].FeatureID || got.Decision != want[i].Decision || got.Approver != want[i].Approver ||
			got.FromState != want[i].FromState || got.ToState != want[i].ToState || !got.DecidedAt.Equal(want[i].DecidedAt) ||
			got.Reason != want[i].Reason || got.ReasonSet != want[i].ReasonSet {
			t.Errorf("entry %d = %+v; want %+v", i, got, want[i])
		}
	}
}

func TestReadHistoryRefusesInvalidRecords(t *testing.T) {
	entry := `{"feature_id":"f","decision":"approval","approver":"a","from_state":"implemented","to_state":"approved","decided_at":"2026-11-25T14:32:07Z"}`
	cases := map[string]string{
		"duplicated human_gates":  `{"human_gates":[],"human_gates":[]}`,
		"case variant beside":     `{"human_gates":[],"Human_Gates":[]}`,
		"case variant instead":    `{"Human_Gates":[` + entry + `]}`,
		"not an array":            `{"human_gates":{}}`,
		"null":                    `{"human_gates":null}`,
		"entry not an object":     `{"human_gates":["x"]}`,
		"missing feature_id":      `{"human_gates":[{"decision":"approval","approver":"a","from_state":"implemented","to_state":"approved","decided_at":"2026-11-25T14:32:07Z"}]}`,
		"missing decision":        `{"human_gates":[{"feature_id":"f","approver":"a","from_state":"implemented","to_state":"approved","decided_at":"2026-11-25T14:32:07Z"}]}`,
		"missing approver":        `{"human_gates":[{"feature_id":"f","decision":"approval","from_state":"implemented","to_state":"approved","decided_at":"2026-11-25T14:32:07Z"}]}`,
		"missing from_state":      `{"human_gates":[{"feature_id":"f","decision":"approval","approver":"a","to_state":"approved","decided_at":"2026-11-25T14:32:07Z"}]}`,
		"missing to_state":        `{"human_gates":[{"feature_id":"f","decision":"approval","approver":"a","from_state":"implemented","decided_at":"2026-11-25T14:32:07Z"}]}`,
		"missing decided_at":      `{"human_gates":[{"feature_id":"f","decision":"approval","approver":"a","from_state":"implemented","to_state":"approved"}]}`,
		"unknown decision":        `{"human_gates":[{"feature_id":"f","decision":"approve","approver":"a","from_state":"implemented","to_state":"approved","decided_at":"2026-11-25T14:32:07Z"}]}`,
		"unrecognized state":      `{"human_gates":[{"feature_id":"f","decision":"approval","approver":"a","from_state":"Implemented","to_state":"approved","decided_at":"2026-11-25T14:32:07Z"}]}`,
		"decided_at not RFC 3339": `{"human_gates":[{"feature_id":"f","decision":"approval","approver":"a","from_state":"implemented","to_state":"approved","decided_at":"yesterday"}]}`,
		"reason not a string":     `{"human_gates":[{"feature_id":"f","decision":"approval","approver":"a","from_state":"implemented","to_state":"approved","decided_at":"2026-11-25T14:32:07Z","reason":1}]}`,
		"not an object":           `[]`,
		"invalid JSON":            `{"human_gates":[`,
	}
	for name, record := range cases {
		t.Run(name, func(t *testing.T) {
			if entries, ok := ReadHistory(historyFS(record), "f"); ok {
				t.Fatalf("ReadHistory accepted %s: %v", record, entries)
			}
		})
	}
}

func TestReadHistoryRefusesInvalidFeatureIDs(t *testing.T) {
	for _, id := range []string{"..", "a/b", "", "."} {
		if _, ok := ReadHistory(historyFS(`{}`), id); ok {
			t.Errorf("ReadHistory accepted feature id %q", id)
		}
	}
}

func TestReadHistoryRefusesNonRegularRecords(t *testing.T) {
	dir := t.TempDir()
	mk := func(feature string) string {
		path := filepath.Join(dir, "specs", feature)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		return filepath.Join(path, workflow.RecordFileName)
	}
	target := filepath.Join(dir, "target.json")
	if err := os.WriteFile(target, []byte(`{"human_gates":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, mk("link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(mk("directory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(mk("fifo"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := os.DirFS(dir)
	for _, feature := range []string{"link", "directory", "fifo"} {
		if entries, ok := ReadHistory(root, feature); ok {
			t.Errorf("ReadHistory accepted a %s record: %v", feature, entries)
		}
	}
}
