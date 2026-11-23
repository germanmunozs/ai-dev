package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// catalogIDs is the catalog order of contracts/catalog.md (Feature 013).
var catalogIDs = []string{"claude-code", "codex-cli", "factory-droid", "gemini-cli", "kiro-cli", "mistral-vibe", "opencode", "qwen-code"}

// catalogExecutables are the names one availability scan looks up, in
// catalog order.
var catalogExecutables = []string{"claude", "codex", "droid", "gemini", "kiro-cli", "vibe", "opencode", "qwen"}

type listingExecutor struct {
	paths   map[string]string
	lookups []string
	runs    int
}

func (e *listingExecutor) LookPath(name string) (string, error) {
	e.lookups = append(e.lookups, name)
	if path := e.paths[name]; path != "" {
		return path, nil
	}
	return "", errors.New("not found")
}

func (e *listingExecutor) Run(context.Context, string, ...string) (string, error) {
	e.runs++
	return "", errors.New("unexpected process execution")
}

func TestRunAgentsListTextIsCompleteOrderedAndDeterministic(t *testing.T) {
	executor := &listingExecutor{paths: map[string]string{"claude": "/tools/claude"}}
	var firstOut, firstErr bytes.Buffer
	if code := RunAgentsList(nil, executor, &firstOut, &firstErr); code != 0 {
		t.Fatalf("first code = %d, stderr = %q", code, firstErr.String())
	}
	var secondOut, secondErr bytes.Buffer
	if code := RunAgentsList([]string{"--format", "text"}, executor, &secondOut, &secondErr); code != 0 {
		t.Fatalf("second code = %d, stderr = %q", code, secondErr.String())
	}
	if firstOut.String() != secondOut.String() {
		t.Fatalf("repeated output changed:\nfirst=%q\nsecond=%q", firstOut.String(), secondOut.String())
	}
	if firstErr.Len() != 0 || secondErr.Len() != 0 {
		t.Fatalf("unexpected stderr: %q / %q", firstErr.String(), secondErr.String())
	}
	last := -1
	for _, id := range catalogIDs {
		at := strings.Index(firstOut.String(), id+"\t")
		if at <= last {
			t.Fatalf("agent %s missing or out of catalog order: %q", id, firstOut.String())
		}
		last = at
	}
	if !strings.Contains(firstOut.String(), "available") || !strings.Contains(firstOut.String(), "unavailable") {
		t.Fatalf("availability missing: %q", firstOut.String())
	}
	if executor.runs != 0 {
		t.Fatalf("Run calls = %d, want 0", executor.runs)
	}
}

// The JSON listing names every catalog agent in catalog order, and only the
// executables found are available (F013 US1, 002 FR-002, FR-003).
func TestRunAgentsListJSONIsIsolatedAndStable(t *testing.T) {
	executor := &listingExecutor{paths: map[string]string{"gemini": "/tools/gemini", "vibe": "/tools/vibe"}}
	var stdout, stderr bytes.Buffer
	if code := RunAgentsList([]string{"--format", "json"}, executor, &stdout, &stderr); code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	var again bytes.Buffer
	if code := RunAgentsList([]string{"--format", "json"}, executor, &again, &stderr); code != 0 || again.String() != stdout.String() {
		t.Fatalf("a second listing differs (code %d):\n%s\n%s", code, stdout.String(), again.String())
	}
	var report struct {
		Agents []struct {
			ID           string `json:"id"`
			Availability string `json:"availability"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("stdout is not one JSON report: %q: %v", stdout.String(), err)
	}
	if len(report.Agents) != len(catalogIDs) {
		t.Fatalf("report = %#v, want %d agents", report, len(catalogIDs))
	}
	for i, item := range report.Agents {
		want := "unavailable"
		if item.ID == "gemini-cli" || item.ID == "mistral-vibe" {
			want = "available"
		}
		if item.ID != catalogIDs[i] || item.Availability != want {
			t.Errorf("agent %d = %s %s, want %s %s", i, item.ID, item.Availability, catalogIDs[i], want)
		}
	}
	if executor.runs != 0 {
		t.Fatalf("Run calls = %d, want 0", executor.runs)
	}
}

func TestRunAgentsListSucceedsWhenAllAgentsAreUnavailable(t *testing.T) {
	executor := &listingExecutor{paths: map[string]string{}}
	var stdout, stderr bytes.Buffer
	if code := RunAgentsList(nil, executor, &stdout, &stderr); code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if got := strings.Count(stdout.String(), "unavailable"); got != len(catalogIDs) {
		t.Fatalf("unavailable count = %d in %q, want %d", got, stdout.String(), len(catalogIDs))
	}
	if executor.runs != 0 {
		t.Fatalf("Run calls = %d, want 0", executor.runs)
	}
}

func TestRunAgentsListUsageFailuresDoNotDiscover(t *testing.T) {
	tests := [][]string{
		{"--unknown"},
		{"--format", "yaml"},
		{"extra"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			executor := &listingExecutor{paths: map[string]string{"claude": "/tools/claude"}}
			var stdout, stderr bytes.Buffer
			if code := RunAgentsList(args, executor, &stdout, &stderr); code != 2 {
				t.Fatalf("code = %d, want 2", code)
			}
			if stdout.Len() != 0 || !strings.Contains(stderr.String(), "Usage:") {
				t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			if len(executor.lookups) != 0 || executor.runs != 0 {
				t.Fatalf("usage failure performed discovery/execution: lookups=%v runs=%d", executor.lookups, executor.runs)
			}
		})
	}
}
