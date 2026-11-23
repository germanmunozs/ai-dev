package agent

import (
	"context"
	"errors"
	"reflect"
	"regexp"
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

// wantCatalog is contracts/catalog.md: claude-code and codex-cli first, as
// before Feature 013, then the other agents by identifier (F013-FR-005).
var wantCatalog = []Agent{
	{ID: ClaudeCode, DisplayLabel: "Claude Code", binary: "claude"},
	{ID: CodexCLI, DisplayLabel: "OpenAI Codex CLI", binary: "codex"},
	{ID: FactoryDroid, DisplayLabel: "Factory Droid", binary: "droid"},
	{ID: GeminiCLI, DisplayLabel: "Gemini CLI", binary: "gemini"},
	{ID: KiroCLI, DisplayLabel: "Kiro CLI", binary: "kiro-cli"},
	{ID: MistralVibe, DisplayLabel: "Mistral Vibe", binary: "vibe"},
	{ID: OpenCode, DisplayLabel: "OpenCode", binary: "opencode"},
	{ID: QwenCode, DisplayLabel: "Qwen Code", binary: "qwen"},
}

func TestRegistryListIsFixedOrderedAndUsesOnlyLocalDiscovery(t *testing.T) {
	executor := &recordingExecutor{paths: map[string]string{"claude": "/tools/claude", "vibe": "/tools/vibe"}}
	registry := NewRegistry(executor)

	first := registry.List()
	second := registry.List()
	var want []AgentStatus
	var wantLookups []string
	for _, a := range wantCatalog {
		availability := Unavailable
		if a.binary == "claude" || a.binary == "vibe" {
			availability = Available
		}
		want = append(want, AgentStatus{Agent: a, Availability: availability})
		wantLookups = append(wantLookups, a.binary)
	}
	wantLookups = append(wantLookups, wantLookups...)
	if !reflect.DeepEqual(first, want) {
		t.Fatalf("first List() = %#v, want %#v", first, want)
	}
	if !reflect.DeepEqual(second, want) {
		t.Fatalf("second List() = %#v, want %#v", second, want)
	}
	if got := executor.lookups; !reflect.DeepEqual(got, wantLookups) {
		t.Fatalf("LookPath calls = %v, want %v", got, wantLookups)
	}
	if executor.runs != 0 {
		t.Fatalf("Run calls = %d, want 0", executor.runs)
	}
}

func TestRegistryListStillReportsEveryAgentWhenNoneAreAvailable(t *testing.T) {
	registry := NewRegistry(&recordingExecutor{paths: map[string]string{}})
	statuses := registry.List()
	if len(statuses) != len(wantCatalog) {
		t.Fatalf("len(List()) = %d, want %d", len(statuses), len(wantCatalog))
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
		{GeminiCLI, Agent{ID: GeminiCLI, DisplayLabel: "Gemini CLI", binary: "gemini"}, true},
		{ID("gemini"), Agent{}, false},
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
	if !reflect.DeepEqual(catalog, wantCatalog) {
		t.Fatalf("Catalog() = %#v, want %#v", catalog, wantCatalog)
	}
	catalog[0].ID = "changed"
	if Catalog()[0].ID != ClaudeCode {
		t.Fatal("mutating the Catalog() result changed the catalog")
	}
	for _, test := range []struct {
		id   ID
		want bool
	}{{ClaudeCode, true}, {CodexCLI, true}, {QwenCode, true}, {"Claude-Code", false}, {"", false}, {"gemini", false}} {
		if got := Known(test.id); got != test.want {
			t.Errorf("Known(%q) = %t, want %t", test.id, got, test.want)
		}
	}
}

// Identifiers are lowercase words joined by single hyphens, distinct, and
// after the two original agents strictly ascending (F013-FR-002, FR-005).
func TestCatalogIdentifiersAreWellFormedDistinctAndOrdered(t *testing.T) {
	valid := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	catalog := Catalog()
	seen := map[ID]bool{}
	for i, a := range catalog {
		if !valid.MatchString(string(a.ID)) {
			t.Errorf("identifier %q is not lowercase words joined by single hyphens", a.ID)
		}
		if seen[a.ID] {
			t.Errorf("identifier %q appears twice", a.ID)
		}
		seen[a.ID] = true
		if a.DisplayLabel == "" || a.Executable() == "" {
			t.Errorf("%s has an empty label or executable", a.ID)
		}
		if i > 2 && catalog[i-1].ID >= a.ID {
			t.Errorf("%s follows %s; after the first two, identifiers must ascend", a.ID, catalog[i-1].ID)
		}
	}
	if catalog[0].ID != ClaudeCode || catalog[1].ID != CodexCLI {
		t.Errorf("the catalog must start with claude-code and codex-cli, got %s and %s", catalog[0].ID, catalog[1].ID)
	}
}

func TestExecutableIsTheLookedUpName(t *testing.T) {
	for _, a := range wantCatalog {
		if got := a.Executable(); got != a.binary {
			t.Errorf("%s Executable() = %q, want %q", a.ID, got, a.binary)
		}
	}
}
