package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

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
	claudeAt := strings.Index(firstOut.String(), "claude-code")
	codexAt := strings.Index(firstOut.String(), "codex-cli")
	if claudeAt < 0 || codexAt < 0 || claudeAt >= codexAt {
		t.Fatalf("agents not complete and ordered: %q", firstOut.String())
	}
	if !strings.Contains(firstOut.String(), "available") || !strings.Contains(firstOut.String(), "unavailable") {
		t.Fatalf("availability missing: %q", firstOut.String())
	}
	if executor.runs != 0 {
		t.Fatalf("Run calls = %d, want 0", executor.runs)
	}
}

func TestRunAgentsListJSONIsIsolatedAndStable(t *testing.T) {
	executor := &listingExecutor{paths: map[string]string{"claude": "/tools/claude", "codex": "/tools/codex"}}
	var stdout, stderr bytes.Buffer
	if code := RunAgentsList([]string{"--format", "json"}, executor, &stdout, &stderr); code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
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
	if len(report.Agents) != 2 || report.Agents[0].ID != "claude-code" || report.Agents[1].ID != "codex-cli" {
		t.Fatalf("report = %#v", report)
	}
	for _, item := range report.Agents {
		if item.Availability != "available" {
			t.Errorf("availability for %s = %q", item.ID, item.Availability)
		}
	}
}

func TestRunAgentsListSucceedsWhenAllAgentsAreUnavailable(t *testing.T) {
	executor := &listingExecutor{paths: map[string]string{}}
	var stdout, stderr bytes.Buffer
	if code := RunAgentsList(nil, executor, &stdout, &stderr); code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if got := strings.Count(stdout.String(), "unavailable"); got != 2 {
		t.Fatalf("unavailable count = %d in %q, want 2", got, stdout.String())
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
