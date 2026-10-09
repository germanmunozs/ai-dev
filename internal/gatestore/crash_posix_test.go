//go:build (darwin && !ios) || (linux && !android)

package gatestore

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"ai-dev/internal/gate"
	"ai-dev/internal/sdd"
	"ai-dev/internal/workflow"
)

func TestGatestoreCrashMatrixAndRecovery(t *testing.T) {
	for _, step := range []string{"locked", "tmp-written", "renamed", "dir-synced"} {
		t.Run(step, func(t *testing.T) {
			repo := makeStoreRepo(t, `{"state":"draft"}`)
			if err := os.WriteFile(filepath.Join(repo, "specs", "f", "spec.md"), nil, 0o644); err != nil {
				t.Fatal(err)
			}
			cmd, reader := startStoreHelper(t, "commit-pause", repo, "f", "draft", step)
			line, err := reader.ReadString('\n')
			if err != nil || line != "paused:"+step+"\n" {
				t.Fatalf("pause=%q err=%v", line, err)
			}
			if err := cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			if err := cmd.Wait(); err == nil {
				t.Fatal("SIGKILLed helper exited successfully")
			}
			want := workflow.Draft
			if step == "renamed" || step == "dir-synced" {
				want = workflow.Specified
			}
			state, invalid := workflow.ResolveRecord(os.DirFS(repo), "f")
			if invalid || state != want {
				t.Fatalf("state=%s invalid=%v want=%s", state, invalid, want)
			}
			data, err := os.ReadFile(filepath.Join(repo, "specs", "f", workflow.RecordFileName))
			if err != nil {
				t.Fatal(err)
			}
			var object map[string]json.RawMessage
			if err := json.Unmarshal(data, &object); err != nil {
				t.Fatal(err)
			}
			var history []json.RawMessage
			if raw := object["human_gates"]; raw != nil {
				if err := json.Unmarshal(raw, &history); err != nil {
					t.Fatal(err)
				}
			}
			wantHistory := 0
			if want == workflow.Specified {
				wantHistory = 1
			}
			if len(history) != wantHistory {
				t.Fatalf("history=%d want=%d record=%s", len(history), wantHistory, data)
			}
			if missing := sdd.MissingArtifacts(os.DirFS(repo), "f", workflow.Specified); len(missing) != 0 {
				t.Fatalf("leftover tmp affected artifact check: missing=%v", missing)
			}
			leftovers, err := filepath.Glob(filepath.Join(repo, "specs", "f", ".workflow-state.*.tmp"))
			if err != nil {
				t.Fatal(err)
			}
			wantLeftovers := 0
			if step == "tmp-written" {
				wantLeftovers = 1
			}
			if len(leftovers) != wantLeftovers {
				t.Fatalf("leftover tmp count=%d want=%d: %v", len(leftovers), wantLeftovers, leftovers)
			}
			next := workflow.Specified
			if want == workflow.Specified {
				next = workflow.Planned
			}
			status, _ := New(repo).Commit("f", want, testEvidence(want, next, "", false))
			if status != gate.Committed {
				t.Fatalf("recovery status=%v", status)
			}
		})
	}
}

func TestGatestorePostCommitFailuresRemainCommitted(t *testing.T) {
	injected := errors.New("injected")
	for _, tc := range []struct {
		mode     string
		warnings []string
	}{
		{"sync", []string{"ai-dev: warning: committed, but directory flush failed: injected"}},
		{"close", []string{"ai-dev: warning: committed, but directory close failed: injected"}},
		{"both", []string{"ai-dev: warning: committed, but directory flush failed: injected", "ai-dev: warning: committed, but directory close failed: injected"}},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			repo := makeStoreRepo(t, `{"state":"draft"}`)
			store := New(repo).(*posixStore)
			if tc.mode == "sync" || tc.mode == "both" {
				store.ops.syncDir = func(*os.File) error { return injected }
			}
			if tc.mode == "close" || tc.mode == "both" {
				store.ops.closeDir = func(file *os.File) error { _ = file.Close(); return injected }
			}
			status, warnings := store.Commit("f", workflow.Draft, testEvidence(workflow.Draft, workflow.Specified, "", false))
			if status != gate.Committed || !reflect.DeepEqual(warnings, tc.warnings) {
				t.Fatalf("status=%v warnings=%v", status, warnings)
			}
			state, invalid := workflow.ResolveRecord(os.DirFS(repo), "f")
			if invalid || state != workflow.Specified {
				t.Fatalf("state=%s invalid=%v", state, invalid)
			}
			next, _ := New(repo).Commit("f", workflow.Specified, testEvidence(workflow.Specified, workflow.Planned, "", false))
			if next != gate.Committed {
				t.Fatalf("next=%v", next)
			}
		})
	}
}

func TestGatestorePreCommitFaultsDoNotMutate(t *testing.T) {
	injected := errors.New("injected")
	cases := []struct {
		name   string
		record string
		mutate func(*posixStore)
	}{
		{"open root", `{"state":"draft"}`, func(s *posixStore) { s.ops.openRoot = func(string) (*os.Root, error) { return nil, injected } }},
		{"open dir", `{"state":"draft"}`, func(s *posixStore) { s.ops.openDir = func(*os.Root, string) (*os.File, error) { return nil, injected } }},
		{"read", `{"state":"draft"}`, func(s *posixStore) { s.ops.readFile = func(*os.Root, string) ([]byte, error) { return nil, injected } }},
		{"build refusal", `{"state":"draft","human_gates":{}}`, func(*posixStore) {}},
		{"open tmp", `{"state":"draft"}`, func(s *posixStore) {
			s.ops.openTmp = func(*os.Root, string, int, os.FileMode) (*os.File, error) { return nil, injected }
		}},
		{"chmod tmp", `{"state":"draft"}`, func(s *posixStore) { s.ops.chmodTmp = func(*os.File, os.FileMode) error { return injected } }},
		{"write", `{"state":"draft"}`, func(s *posixStore) { s.ops.writeTmp = func(*os.File, []byte) error { return injected } }},
		{"sync", `{"state":"draft"}`, func(s *posixStore) { s.ops.syncTmp = func(*os.File) error { return injected } }},
		{"close", `{"state":"draft"}`, func(s *posixStore) { s.ops.closeTmp = func(f *os.File) error { _ = f.Close(); return injected } }},
		{"rename", `{"state":"draft"}`, func(s *posixStore) { s.ops.rename = func(*os.Root, string, string) error { return injected } }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := makeStoreRepo(t, tc.record)
			record := filepath.Join(repo, "specs", "f", workflow.RecordFileName)
			before, _ := os.ReadFile(record)
			store := New(repo).(*posixStore)
			tc.mutate(store)
			status, _ := store.Commit("f", workflow.Draft, testEvidence(workflow.Draft, workflow.Specified, "", false))
			after, _ := os.ReadFile(record)
			matches, _ := filepath.Glob(filepath.Join(repo, "specs", "f", "*.tmp"))
			if status != gate.Failed || string(before) != string(after) || len(matches) != 0 {
				t.Fatalf("status=%v before=%q after=%q tmp=%v", status, before, after, matches)
			}
		})
	}
}
