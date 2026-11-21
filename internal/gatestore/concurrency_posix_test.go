//go:build (darwin && !ios) || (linux && !android)

package gatestore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/gate"
	"github.com/germanmunozs/ai-dev/internal/workflow"
)

func TestGatestoreConcurrentCommits(t *testing.T) {
	repo := makeStoreRepo(t, `{"state":"draft"}`)
	start := make(chan struct{})
	results := make(chan gate.CommitStatus, 16)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			<-start
			entry := testEvidence(workflow.Draft, workflow.Specified, "", false)
			entry.Approver = string(rune('a' + n))
			status, _ := New(repo).Commit("f", workflow.Draft, entry)
			results <- status
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	committed, conflicts := 0, 0
	for status := range results {
		if status == gate.Committed {
			committed++
		} else if status == gate.Conflict {
			conflicts++
		} else {
			t.Fatalf("status=%v", status)
		}
	}
	if committed != 1 || conflicts != 15 {
		t.Fatalf("committed=%d conflicts=%d", committed, conflicts)
	}
	data, _ := os.ReadFile(filepath.Join(repo, "specs", "f", workflow.RecordFileName))
	var object map[string]json.RawMessage
	json.Unmarshal(data, &object)
	var history []json.RawMessage
	json.Unmarshal(object["human_gates"], &history)
	if len(history) != 1 {
		t.Fatalf("history=%d", len(history))
	}
	if report := workflow.EvaluateState(os.DirFS(repo), "f"); report.State != workflow.Specified {
		t.Fatalf("report=%+v", report)
	}
}

func TestGatestoreRealStateChangeConflicts(t *testing.T) {
	repo := makeStoreRepo(t, `{"state":"draft"}`)
	if status, _ := New(repo).Commit("f", workflow.Draft, testEvidence(workflow.Draft, workflow.Specified, "", false)); status != gate.Committed {
		t.Fatal(status)
	}
	record := filepath.Join(repo, "specs", "f", workflow.RecordFileName)
	before, _ := os.ReadFile(record)
	status, _ := New(repo).Commit("f", workflow.Draft, testEvidence(workflow.Draft, workflow.Specified, "", false))
	after, _ := os.ReadFile(record)
	if status != gate.Conflict || string(before) != string(after) {
		t.Fatalf("status=%v", status)
	}
}
