//go:build (darwin && !ios) || (linux && !android)

package gatestore

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"syscall"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/gate"
	"github.com/germanmunozs/ai-dev/internal/workflow"
)

func makeStoreRepo(t *testing.T, record string) string {
	t.Helper()
	repo := t.TempDir()
	dir := filepath.Join(repo, "specs", "f")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if record != "" {
		if err := os.WriteFile(filepath.Join(dir, workflow.RecordFileName), []byte(record), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	return repo
}

func TestPOSIXStoreCommitAndAppend(t *testing.T) {
	repo := makeStoreRepo(t, "")
	store := New(repo)
	first := testEvidence(workflow.Draft, workflow.Specified, "", false)
	status, warnings := store.Commit("f", workflow.Draft, first)
	if status != gate.Committed || len(warnings) != 0 {
		t.Fatalf("status=%v warnings=%v", status, warnings)
	}
	report := workflow.EvaluateState(os.DirFS(repo), "f")
	if report.Outcome != workflow.StateReported || report.State != workflow.Specified {
		t.Fatalf("report=%+v", report)
	}
	firstData, err := os.ReadFile(filepath.Join(repo, "specs", "f", workflow.RecordFileName))
	if err != nil {
		t.Fatal(err)
	}
	var firstObject map[string]json.RawMessage
	if err := json.Unmarshal(firstData, &firstObject); err != nil {
		t.Fatal(err)
	}
	var firstHistory []json.RawMessage
	if err := json.Unmarshal(firstObject["human_gates"], &firstHistory); err != nil {
		t.Fatal(err)
	}
	second := testEvidence(workflow.Specified, workflow.Planned, "plan ok", true)
	status, warnings = store.Commit("f", workflow.Specified, second)
	if status != gate.Committed || len(warnings) != 0 {
		t.Fatalf("status=%v warnings=%v", status, warnings)
	}
	data, err := os.ReadFile(filepath.Join(repo, "specs", "f", workflow.RecordFileName))
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	json.Unmarshal(data, &object)
	var history []json.RawMessage
	json.Unmarshal(object["human_gates"], &history)
	if len(history) != 2 || len(firstHistory) != 1 || !bytes.Equal(compactJSON(history[0]), compactJSON(firstHistory[0])) {
		t.Fatalf("history=%s", object["human_gates"])
	}
	assertNoStoreTmp(t, repo)
}

func TestPOSIXStorePreservesRecordMode(t *testing.T) {
	repo := makeStoreRepo(t, `{"state":"draft"}`)
	// syscall.Umask changes process-wide state; this test must not run in parallel.
	oldUmask := syscall.Umask(0o077)
	t.Cleanup(func() { syscall.Umask(oldUmask) })
	status, _ := New(repo).Commit("f", workflow.Draft, testEvidence(workflow.Draft, workflow.Specified, "", false))
	if status != gate.Committed {
		t.Fatalf("status=%v", status)
	}
	info, err := os.Stat(filepath.Join(repo, "specs", "f", workflow.RecordFileName))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
}

func TestPOSIXStoreDoesNotChangeUnknownMemberOrder(t *testing.T) {
	const record = `{"before":{"v":1},"state":"draft","after":[2,3]}`
	repo := makeStoreRepo(t, record)
	status, _ := New(repo).Commit("f", workflow.Draft, testEvidence(workflow.Draft, workflow.Specified, "", false))
	if status != gate.Committed {
		t.Fatalf("status=%v", status)
	}
	data, err := os.ReadFile(filepath.Join(repo, "specs", "f", workflow.RecordFileName))
	if err != nil {
		t.Fatal(err)
	}
	beforeIndex := bytes.Index(data, []byte(`"before"`))
	stateIndex := bytes.Index(data, []byte(`"state"`))
	afterIndex := bytes.Index(data, []byte(`"after"`))
	historyIndex := bytes.Index(data, []byte(`"human_gates"`))
	if beforeIndex < 0 || stateIndex <= beforeIndex || afterIndex <= stateIndex || historyIndex <= afterIndex {
		t.Fatalf("top-level member order changed: %s", data)
	}
}

func TestPOSIXStoreNewRecordModeIgnoresRestrictiveUmask(t *testing.T) {
	repo := makeStoreRepo(t, "")
	// syscall.Umask changes process-wide state; this test must not run in parallel.
	oldUmask := syscall.Umask(0o077)
	t.Cleanup(func() { syscall.Umask(oldUmask) })
	status, _ := New(repo).Commit("f", workflow.Draft, testEvidence(workflow.Draft, workflow.Specified, "", false))
	if status != gate.Committed {
		t.Fatalf("status=%v", status)
	}
	info, err := os.Stat(filepath.Join(repo, "specs", "f", workflow.RecordFileName))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
}

func TestPOSIXStoreRefusalsDoNotMutate(t *testing.T) {
	cases := []struct {
		name, record string
		expected     workflow.State
		want         gate.CommitStatus
	}{
		{"stale", `{"state":"specified"}`, workflow.Draft, gate.Conflict},
		{"invalid", `{"state":"bad"}`, workflow.Draft, gate.Conflict},
		{"malformed history", `{"state":"draft","human_gates":{}}`, workflow.Draft, gate.Failed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := makeStoreRepo(t, tc.record)
			path := filepath.Join(repo, "specs", "f", workflow.RecordFileName)
			before, _ := os.ReadFile(path)
			status, _ := New(repo).Commit("f", tc.expected, testEvidence(tc.expected, workflow.Specified, "", false))
			if status != tc.want {
				t.Fatalf("status=%v", status)
			}
			after, _ := os.ReadFile(path)
			if string(after) != string(before) {
				t.Fatalf("record changed: %s", after)
			}
			matches, _ := filepath.Glob(filepath.Join(repo, "specs", "f", "*.tmp"))
			if len(matches) != 0 {
				t.Fatalf("tmp=%v", matches)
			}
		})
	}
}

func TestPOSIXStoreIgnoresOldLockFileAndPreservesUnknown(t *testing.T) {
	repo := makeStoreRepo(t, `{"note":{"x":1},"state":"draft"}`)
	if err := os.WriteFile(filepath.Join(repo, "specs", "f", ".workflow-state.lock"), []byte("user"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, _ := New(repo).Commit("f", workflow.Draft, testEvidence(workflow.Draft, workflow.Specified, "", false))
	if status != gate.Committed {
		t.Fatalf("status=%v", status)
	}
	data, _ := os.ReadFile(filepath.Join(repo, "specs", "f", workflow.RecordFileName))
	var object map[string]json.RawMessage
	json.Unmarshal(data, &object)
	if _, ok := object["note"]; !ok {
		t.Fatal("unknown member lost")
	}
	lock, _ := os.ReadFile(filepath.Join(repo, "specs", "f", ".workflow-state.lock"))
	if string(lock) != "user" {
		t.Fatal("old lock touched")
	}
	assertNoStoreTmp(t, repo)
}

func TestPOSIXStoreRejectsSymlinkedRecord(t *testing.T) {
	repo := makeStoreRepo(t, "")
	dir := filepath.Join(repo, "specs", "f")
	if err := os.WriteFile(filepath.Join(dir, "target.json"), []byte(`{"state":"draft"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target.json", filepath.Join(dir, workflow.RecordFileName)); err != nil {
		t.Skip(err)
	}
	status, _ := New(repo).Commit("f", workflow.Draft, testEvidence(workflow.Draft, workflow.Specified, "", false))
	if status != gate.Conflict {
		t.Fatalf("status=%v", status)
	}
	target, err := os.ReadFile(filepath.Join(dir, "target.json"))
	if err != nil || string(target) != `{"state":"draft"}` {
		t.Fatalf("target changed: %q err=%v", target, err)
	}
	assertNoStoreTmp(t, repo)
}

func TestPOSIXStoreReadOnlyDirectoryFailsWithoutMutation(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permissions are not enforced for this user/platform")
	}
	repo := makeStoreRepo(t, `{"state":"draft"}`)
	dir := filepath.Join(repo, "specs", "f")
	record := filepath.Join(dir, workflow.RecordFileName)
	before, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	status, _ := New(repo).Commit("f", workflow.Draft, testEvidence(workflow.Draft, workflow.Specified, "", false))
	after, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if status != gate.Failed || !bytes.Equal(before, after) {
		t.Fatalf("status=%v before=%q after=%q", status, before, after)
	}
	assertNoStoreTmp(t, repo)
}

func TestPOSIXStoreChangesOnlyWorkflowRecord(t *testing.T) {
	repo := makeStoreRepo(t, `{"note":{"x":1},"state":"draft"}`)
	dir := filepath.Join(repo, "specs", "f")
	marker := filepath.Join(dir, "spec.md")
	other := filepath.Join(repo, "specs", "other")
	if err := os.WriteFile(marker, []byte("spec"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "marker"), []byte("other"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshotStoreTree(t, repo)
	status, _ := New(repo).Commit("f", workflow.Draft, testEvidence(workflow.Draft, workflow.Specified, "", false))
	if status != gate.Committed {
		t.Fatalf("status=%v", status)
	}
	after := snapshotStoreTree(t, repo)
	recordRelative := filepath.Join("specs", "f", workflow.RecordFileName)
	if reflect.DeepEqual(before[recordRelative], after[recordRelative]) {
		t.Fatal("workflow-state.json did not change")
	}
	delete(before, recordRelative)
	delete(after, recordRelative)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("a path other than workflow-state.json changed\nbefore=%v\nafter=%v", before, after)
	}
	data, err := os.ReadFile(filepath.Join(dir, workflow.RecordFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"note": {`)) || !bytes.Contains(data, []byte(`"x": 1`)) {
		t.Fatalf("unknown member not preserved: %s", data)
	}
	assertNoStoreTmp(t, repo)
}

type storeSnapshotEntry struct {
	mode    os.FileMode
	content string
}

func snapshotStoreTree(t *testing.T, root string) map[string]storeSnapshotEntry {
	t.Helper()
	snapshot := map[string]storeSnapshotEntry{}
	err := filepath.Walk(root, func(filename string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if filename == root {
			return nil
		}
		relative, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		entry := storeSnapshotEntry{mode: info.Mode() & (os.ModeType | os.ModePerm)}
		if info.Mode().IsRegular() {
			content, err := os.ReadFile(filename)
			if err != nil {
				return err
			}
			entry.content = string(content)
		}
		snapshot[relative] = entry
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func assertNoStoreTmp(t *testing.T, repo string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(repo, "specs", "f", "*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("tmp=%v", matches)
	}
}
