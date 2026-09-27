package checks

import (
	"context"
	"testing"
	"testing/fstest"

	"ai-dev/internal/doctor"
)

func TestSpecKitCheckUsesOnlyCoreManifest(t *testing.T) {
	root := fstest.MapFS{
		".specify/integrations/speckit.manifest.json": &fstest.MapFile{Data: []byte(`{"files":{".specify/templates/plan.md":"ignored"}}`)},
		".specify/templates/plan.md":                  &fstest.MapFile{Data: []byte("template")},
		".specify/integration.json":                   &fstest.MapFile{Data: []byte(`not relevant`)},
	}
	got := NewSpecKitCheck(root).Run(context.Background())
	if got.Status != doctor.Pass || got.ID != "speckit-initialized" {
		t.Fatalf("unexpected result: %#v", got)
	}
}

func TestSpecKitCheckReportsIncompleteCoreScaffold(t *testing.T) {
	tests := []struct {
		name string
		root fstest.MapFS
	}{
		{"absent scaffold", fstest.MapFS{}},
		{"incomplete scaffold", fstest.MapFS{
			".specify/integrations/speckit.manifest.json": &fstest.MapFile{Data: []byte(`{"files":{".specify/templates/missing.md":"x"}}`)},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewSpecKitCheck(tt.root).Run(context.Background())
			if got.Status != doctor.Fail || got.Explanation == "" || got.Remediation == "" {
				t.Fatalf("unexpected result: %#v", got)
			}
		})
	}
}
