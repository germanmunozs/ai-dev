package checks

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func TestValidateManifestFilesPassesForNonEmptyListedFiles(t *testing.T) {
	root := fstest.MapFS{
		"manifest.json": &fstest.MapFile{Data: []byte(`{"files":{"one.txt":"old-hash","dir/two.txt":"other-hash"}}`)},
		"one.txt":       &fstest.MapFile{Data: []byte("customized")},
		"dir/two.txt":   &fstest.MapFile{Data: []byte("x")},
	}
	issues := ValidateManifestFiles(root, "manifest.json")
	if len(issues) != 0 {
		t.Fatalf("issues = %v, want none", issues)
	}
}

type panicFS struct{}

func (panicFS) Open(string) (fs.File, error) { panic("filesystem panic") }

func TestValidateManifestFilesFailures(t *testing.T) {
	tests := []struct {
		name string
		root fs.FS
		want []string
	}{
		{"missing manifest", fstest.MapFS{}, []string{"manifest.json"}},
		{"malformed manifest", fstest.MapFS{"manifest.json": &fstest.MapFile{Data: []byte("{")}}, []string{"malformed"}},
		{"missing files map", fstest.MapFS{"manifest.json": &fstest.MapFile{Data: []byte(`{}`)}}, []string{"files map"}},
		{"empty files map", fstest.MapFS{"manifest.json": &fstest.MapFile{Data: []byte(`{"files":{}}`)}}, []string{"empty files map"}},
		{"missing and empty files aggregated", fstest.MapFS{
			"manifest.json": &fstest.MapFile{Data: []byte(`{"files":{"missing.txt":"x","empty.txt":"y"}}`)},
			"empty.txt":     &fstest.MapFile{Data: []byte(" \n")},
		}, []string{"missing.txt", "empty.txt"}},
		{"unsafe paths", fstest.MapFS{"manifest.json": &fstest.MapFile{Data: []byte(`{"files":{"../secret":"x","/absolute":"y"}}`)}}, []string{"../secret", "/absolute"}},
		{"panic", panicFS{}, []string{"unexpected filesystem failure"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			joined := strings.Join(ValidateManifestFiles(tt.root, "manifest.json"), " ")
			for _, want := range tt.want {
				if !strings.Contains(joined, want) {
					t.Errorf("issues %q missing %q", joined, want)
				}
			}
		})
	}
}
