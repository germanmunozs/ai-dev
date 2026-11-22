package agent

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type recordingExecutor struct {
	paths   map[string]string
	lookups []string
	runs    int
}

func (e *recordingExecutor) LookPath(name string) (string, error) {
	e.lookups = append(e.lookups, name)
	if path := e.paths[name]; path != "" {
		return path, nil
	}
	return "", errors.New("not found")
}

func (e *recordingExecutor) Run(context.Context, string, ...string) (string, error) {
	e.runs++
	return "", errors.New("Run must not be used for availability")
}

func TestRegistryListIsFixedOrderedAndUsesOnlyLocalDiscovery(t *testing.T) {
	executor := &recordingExecutor{paths: map[string]string{"claude": "/tools/claude"}}
	registry := NewRegistry(executor)

	first := registry.List()
	second := registry.List()
	want := []AgentStatus{
		{Agent: Agent{ID: ClaudeCode, DisplayLabel: "Claude Code", binary: "claude"}, Availability: Available},
		{Agent: Agent{ID: CodexCLI, DisplayLabel: "OpenAI Codex CLI", binary: "codex"}, Availability: Unavailable},
	}
	if !reflect.DeepEqual(first, want) {
		t.Fatalf("first List() = %#v, want %#v", first, want)
	}
	if !reflect.DeepEqual(second, want) {
		t.Fatalf("second List() = %#v, want %#v", second, want)
	}
	if got, wantLookups := executor.lookups, []string{"claude", "codex", "claude", "codex"}; !reflect.DeepEqual(got, wantLookups) {
		t.Fatalf("LookPath calls = %v, want %v", got, wantLookups)
	}
	if executor.runs != 0 {
		t.Fatalf("Run calls = %d, want 0", executor.runs)
	}
}

func TestRegistryListStillReportsEveryAgentWhenNoneAreAvailable(t *testing.T) {
	registry := NewRegistry(&recordingExecutor{paths: map[string]string{}})
	statuses := registry.List()
	if len(statuses) != 2 {
		t.Fatalf("len(List()) = %d, want 2", len(statuses))
	}
	for _, status := range statuses {
		if status.Availability != Unavailable {
			t.Errorf("%s availability = %v, want unavailable", status.Agent.ID, status.Availability)
		}
	}
}

func TestRegistryResolveUsesOnlyCanonicalIDs(t *testing.T) {
	registry := NewRegistry(&recordingExecutor{paths: map[string]string{}})
	tests := []struct {
		id        ID
		want      Agent
		wantFound bool
	}{
		{ClaudeCode, Agent{ID: ClaudeCode, DisplayLabel: "Claude Code", binary: "claude"}, true},
		{CodexCLI, Agent{ID: CodexCLI, DisplayLabel: "OpenAI Codex CLI", binary: "codex"}, true},
		{ID("Claude Code"), Agent{}, false},
		{ID("unknown"), Agent{}, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.id), func(t *testing.T) {
			got, found := registry.Resolve(tt.id)
			if found != tt.wantFound || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Resolve(%q) = %#v, %v; want %#v, %v", tt.id, got, found, tt.want, tt.wantFound)
			}
		})
	}
}

// Catalog and Known are read-only views of the fixed set (F011 R-06): they
// probe nothing and cannot change the catalog.
func TestCatalogIsAReadOnlyCopyInCatalogOrder(t *testing.T) {
	catalog := Catalog()
	if len(catalog) != 2 || catalog[0].ID != ClaudeCode || catalog[1].ID != CodexCLI {
		t.Fatalf("Catalog() = %#v", catalog)
	}
	catalog[0].ID = "changed"
	if Catalog()[0].ID != ClaudeCode {
		t.Fatal("mutating the Catalog() result changed the catalog")
	}
	for _, test := range []struct {
		id   ID
		want bool
	}{{ClaudeCode, true}, {CodexCLI, true}, {"Claude-Code", false}, {"", false}, {"gemini", false}} {
		if got := Known(test.id); got != test.want {
			t.Errorf("Known(%q) = %t, want %t", test.id, got, test.want)
		}
	}
}
