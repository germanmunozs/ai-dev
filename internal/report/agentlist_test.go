package report

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/agent"
)

func TestWriteAgentListTextPreservesRegistryOrderAndValues(t *testing.T) {
	statuses := listingFixture()
	var output bytes.Buffer
	if err := WriteAgentListText(&output, statuses); err != nil {
		t.Fatalf("WriteAgentListText() error = %v", err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %q, want 2", lines)
	}
	wantFields := [][]string{
		{"claude-code", "Claude", "Code", "available"},
		{"codex-cli", "OpenAI", "Codex", "CLI", "unavailable"},
	}
	for i, line := range lines {
		if got := strings.Fields(line); !equalStrings(got, wantFields[i]) {
			t.Errorf("line %d fields = %q, want %q", i, got, wantFields[i])
		}
	}
}

func TestWriteAgentListJSONWritesOneStableObject(t *testing.T) {
	var output bytes.Buffer
	if err := WriteAgentListJSON(&output, listingFixture()); err != nil {
		t.Fatalf("WriteAgentListJSON() error = %v", err)
	}
	var got struct {
		Agents []struct {
			ID           string `json:"id"`
			DisplayLabel string `json:"display_label"`
			Availability string `json:"availability"`
		} `json:"agents"`
	}
	decoder := json.NewDecoder(&output)
	if err := decoder.Decode(&got); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if decoder.More() {
		t.Fatal("JSON output contains more than one value")
	}
	if len(got.Agents) != 2 {
		t.Fatalf("agents = %#v", got.Agents)
	}
	if got.Agents[0].ID != "claude-code" || got.Agents[0].DisplayLabel != "Claude Code" || got.Agents[0].Availability != "available" {
		t.Errorf("first agent = %#v", got.Agents[0])
	}
	if got.Agents[1].ID != "codex-cli" || got.Agents[1].DisplayLabel != "OpenAI Codex CLI" || got.Agents[1].Availability != "unavailable" {
		t.Errorf("second agent = %#v", got.Agents[1])
	}
}

func listingFixture() []agent.AgentStatus {
	registry := agent.NewRegistry(&reportExecutor{paths: map[string]string{"claude": "/tools/claude"}})
	return registry.List()
}

type reportExecutor struct {
	paths map[string]string
}

func (e *reportExecutor) LookPath(name string) (string, error) {
	if e.paths[name] != "" {
		return e.paths[name], nil
	}
	return "", errors.New("not found")
}

func (e *reportExecutor) Run(_ context.Context, _ string, _ ...string) (string, error) {
	panic("unexpected Run")
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
