package checks

import (
	"context"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"ai-dev/internal/doctor"
)

func TestManifestIntegrationCheckPassesCustomizedFiles(t *testing.T) {
	root := fstest.MapFS{
		".specify/integration.json":                  &fstest.MapFile{Data: []byte(`{"installed_integrations":["claude","codex"]}`)},
		".specify/integrations/claude.manifest.json": &fstest.MapFile{Data: []byte(`{"files":{".claude/skill.md":"original-sha"}}`)},
		".specify/integrations/codex.manifest.json":  &fstest.MapFile{Data: []byte(`{"files":{".agents/skill.md":"original-sha"}}`)},
		".claude/skill.md":                           &fstest.MapFile{Data: []byte("custom Claude content")},
		".agents/skill.md":                           &fstest.MapFile{Data: []byte("custom Codex content")},
	}
	tests := []struct{ integration, id, name string }{
		{"claude", "claude-integration", "Claude Spec Kit Integration"},
		{"codex", "codex-integration", "Codex Spec Kit Integration"},
	}
	for _, tt := range tests {
		t.Run(tt.integration, func(t *testing.T) {
			got := NewManifestIntegrationCheck(tt.integration, tt.id, tt.name, root).Run(context.Background())
			if got.Status != doctor.Pass || got.ID != tt.id || got.Name != tt.name || got.Area != "Repository" {
				t.Fatalf("unexpected result: %#v", got)
			}
		})
	}
}

func TestManifestIntegrationCheckFailuresAreIndependentAndContained(t *testing.T) {
	tests := []struct {
		name string
		root fs.FS
		want string
	}{
		{"missing record", fstest.MapFS{}, "integration.json"},
		{"malformed record", fstest.MapFS{
			".specify/integration.json": &fstest.MapFile{Data: []byte("{")},
		}, "malformed"},
		{"membership missing", fstest.MapFS{
			".specify/integration.json":                  &fstest.MapFile{Data: []byte(`{"installed_integrations":["codex"]}`)},
			".specify/integrations/claude.manifest.json": &fstest.MapFile{Data: []byte(`{"files":{}}`)},
		}, "installed_integrations"},
		{"manifest missing", fstest.MapFS{
			".specify/integration.json": &fstest.MapFile{Data: []byte(`{"installed_integrations":["claude"]}`)},
		}, "claude.manifest.json"},
		{"manifest malformed", fstest.MapFS{
			".specify/integration.json":                  &fstest.MapFile{Data: []byte(`{"installed_integrations":["claude"]}`)},
			".specify/integrations/claude.manifest.json": &fstest.MapFile{Data: []byte("{")},
		}, "malformed"},
		{"required file missing", fstest.MapFS{
			".specify/integration.json":                  &fstest.MapFile{Data: []byte(`{"installed_integrations":["claude"]}`)},
			".specify/integrations/claude.manifest.json": &fstest.MapFile{Data: []byte(`{"files":{".claude/missing.md":"x"}}`)},
		}, ".claude/missing.md"},
		{"required file empty", fstest.MapFS{
			".specify/integration.json":                  &fstest.MapFile{Data: []byte(`{"installed_integrations":["claude"]}`)},
			".specify/integrations/claude.manifest.json": &fstest.MapFile{Data: []byte(`{"files":{".claude/empty.md":"x"}}`)},
			".claude/empty.md":                           &fstest.MapFile{},
		}, ".claude/empty.md"},
		{"panic", panicFS{}, "unexpected"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewManifestIntegrationCheck("claude", "claude-integration", "Claude Spec Kit Integration", tt.root).Run(context.Background())
			if got.Status != doctor.Fail || got.Explanation == "" || got.Remediation == "" || !strings.Contains(got.Explanation, tt.want) {
				t.Fatalf("unexpected result: %#v", got)
			}
		})
	}
}

func TestClaudeAndCodexIntegrationResultsAreIndependent(t *testing.T) {
	root := fstest.MapFS{
		".specify/integration.json":                  &fstest.MapFile{Data: []byte(`{"installed_integrations":["claude","codex"]}`)},
		".specify/integrations/claude.manifest.json": &fstest.MapFile{Data: []byte(`{"files":{".claude/skill.md":"x"}}`)},
		".specify/integrations/codex.manifest.json":  &fstest.MapFile{Data: []byte(`{"files":{".agents/missing.md":"x"}}`)},
		".claude/skill.md":                           &fstest.MapFile{Data: []byte("installed")},
	}
	claude := NewManifestIntegrationCheck("claude", "claude-integration", "Claude", root).Run(context.Background())
	codex := NewManifestIntegrationCheck("codex", "codex-integration", "Codex", root).Run(context.Background())
	if claude.Status != doctor.Pass || codex.Status != doctor.Fail {
		t.Fatalf("claude/codex = %#v / %#v", claude, codex)
	}
}
