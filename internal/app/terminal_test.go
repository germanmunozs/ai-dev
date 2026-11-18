package app

import (
	"os"
	"strings"
	"testing"
)

func TestIsTerminalRejectsNonTerminals(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	file, err := os.CreateTemp(t.TempDir(), "f")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	for name, reader := range map[string]any{"/dev/null": devNull, "regular file": file, "string reader": strings.NewReader("x")} {
		if isTerminal(reader.(interface{ Read([]byte) (int, error) })) {
			t.Errorf("%s detected as a terminal", name)
		}
	}
}
