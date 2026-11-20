package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/conventionalcommit"
)

// T101: the rendering keeps validity and breaking recognition separate
// and refuses an inconsistent result instead of writing it.
func TestConventionalCommitRendering(t *testing.T) {
	var text, json bytes.Buffer
	valid := conventionalcommit.Validate([]byte("feat(api)!: drop\n\nWhy.\nMore.\n\nBREAKING CHANGE: gone\n"))
	if err := WriteConventionalCommitText(&text, valid); err != nil {
		t.Fatal(err)
	}
	if err := WriteConventionalCommitJSON(&json, valid); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text.String(), "scope:          api\n") || !strings.Contains(text.String(), "body lines:     2\n") ||
		!strings.Contains(text.String(), "breaking:       true\n") {
		t.Fatalf("text %q", text.String())
	}
	if !strings.Contains(json.String(), `"body_lines":["Why.","More."]`) || !strings.Contains(json.String(), `"diagnostics":[]`) {
		t.Fatalf("json %q", json.String())
	}
	var out bytes.Buffer
	for _, bad := range []conventionalcommit.Validation{{Valid: true, Diagnostics: []string{"x"}}, {Breaking: true, Diagnostics: []string{"x"}}, {}} {
		if err := WriteConventionalCommitJSON(&out, bad); err == nil || out.Len() != 0 {
			t.Fatalf("inconsistent result rendered: %+v", bad)
		}
	}
}
