//go:build (darwin && !ios) || (linux && !android)

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestWorkflowGateConcurrentProcesses(t *testing.T) {
	binary := buildWorkflowNextBinary(t)
	for iteration := 0; iteration < 20; iteration++ {
		repo := makeWorkflowGateRepo(t, map[string][]string{"f": {"spec.md"}})
		record := `{"state":"draft","padding":"` + strings.Repeat("x", 4<<20) + `"}`
		if err := os.WriteFile(filepath.Join(repo, "specs", "f", "workflow-state.json"), []byte(record), 0o644); err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		type result struct {
			out, err string
			code     int
		}
		results := make(chan result, 8)
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				<-start
				cmd := exec.Command(binary, "workflow", "approve", "--approver", string(rune('a'+n)), "f", "specified")
				cmd.Dir = repo
				out, errOut, code := runCommand(cmd)
				results <- result{out, errOut, code}
			}(i)
		}
		close(start)
		wg.Wait()
		close(results)
		applied, conflicts := 0, 0
		for result := range results {
			switch {
			case result.code == 0 && strings.Contains(result.out, "transition_applied"):
				applied++
			case result.code == 1 && strings.Contains(result.out, "state_conflict"):
				conflicts++
			default:
				t.Fatalf("iteration %d unexpected code=%d out=%q err=%q", iteration, result.code, result.out, result.err)
			}
		}
		if applied != 1 || conflicts != 7 {
			t.Fatalf("iteration %d applied=%d conflicts=%d", iteration, applied, conflicts)
		}
		data, err := os.ReadFile(filepath.Join(repo, "specs", "f", "workflow-state.json"))
		if err != nil {
			t.Fatal(err)
		}
		var object map[string]json.RawMessage
		_ = json.Unmarshal(data, &object)
		var history []json.RawMessage
		_ = json.Unmarshal(object["human_gates"], &history)
		if len(history) != 1 {
			t.Fatalf("iteration %d history=%d", iteration, len(history))
		}
	}
}
