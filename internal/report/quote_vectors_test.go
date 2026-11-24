package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The shared vectors pin the quoting rules, so the Neovim plugin's quoting
// can be proven equal to this one (F014-FR-023, SC-007).
func TestShellInertQuoteSharedVectors(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "quote-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct{ Name, Input, Want string }
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors) == 0 {
		t.Fatal("no quote vectors")
	}
	for _, v := range vectors {
		if got := ShellInertQuote(v.Input); got != v.Want {
			t.Errorf("%s: ShellInertQuote(%q) = %q, want %q", v.Name, v.Input, got, v.Want)
		}
	}
}
