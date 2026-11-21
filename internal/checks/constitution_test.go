package checks

import (
	"context"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/germanmunozs/ai-dev/internal/doctor"
)

func TestConstitutionCheckPass(t *testing.T) {
	root := fstest.MapFS{
		".specify/memory/constitution.md": &fstest.MapFile{Data: []byte("# Constitution\n\n**Version**: 1.0.0 | **Ratified**: 2026-09-27\n")},
	}
	got := NewConstitutionCheck(root).Run(context.Background())
	if got.Status != doctor.Pass || got.ID != "constitution" || got.Explanation != "" || got.Remediation != "" {
		t.Fatalf("unexpected result: %#v", got)
	}
}

func TestConstitutionCheckFailuresAreSpecificAndContained(t *testing.T) {
	tests := []struct {
		name string
		root fs.FS
		want string
	}{
		{"missing", fstest.MapFS{}, "missing"},
		{"empty", fstest.MapFS{".specify/memory/constitution.md": &fstest.MapFile{}}, "empty"},
		{"placeholder", fstest.MapFS{".specify/memory/constitution.md": &fstest.MapFile{Data: []byte("[PROJECT_NAME]\n**Version**: 1\n**Ratified**: today")}}, "placeholder"},
		{"versionless", fstest.MapFS{".specify/memory/constitution.md": &fstest.MapFile{Data: []byte("**Ratified**: today")}}, "Version"},
		{"ratificationless", fstest.MapFS{".specify/memory/constitution.md": &fstest.MapFile{Data: []byte("**Version**: 1")}}, "Ratified"},
		{"panic", panicFS{}, "unexpected"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewConstitutionCheck(tt.root).Run(context.Background())
			if got.Status != doctor.Fail || got.Explanation == "" || got.Remediation == "" || !strings.Contains(got.Explanation, tt.want) {
				t.Fatalf("unexpected result: %#v", got)
			}
		})
	}
}
