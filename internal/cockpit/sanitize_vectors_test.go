package cockpit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The shared vectors pin the sanitizer's rules, so the Neovim plugin's
// sanitizer can be proven equal to this one (F014-FR-023, SC-007).
func TestSanitizeSharedVectors(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "sanitize-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct{ Name, Input, Want string }
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors) == 0 {
		t.Fatal("no sanitize vectors")
	}
	for _, v := range vectors {
		if got := Sanitize(v.Input).String(); got != v.Want {
			t.Errorf("%s: Sanitize(%q) = %q, want %q", v.Name, v.Input, got, v.Want)
		}
	}
}
