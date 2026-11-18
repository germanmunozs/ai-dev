package report

import (
	"bytes"
	"testing"

	"ai-dev/internal/gitmanager"
)

// An inspection that does not validate is never rendered, in either form.
func TestGitInspectionRenderingValidatesFirst(t *testing.T) {
	var out bytes.Buffer
	if err := WriteGitInspectionJSON(&out, gitmanager.GitInspection{}); err == nil || out.Len() != 0 {
		t.Fatalf("invalid inspection rendered as JSON: %q %v", out.String(), err)
	}
	if err := WriteGitInspectionText(&out, gitmanager.GitInspection{}); err == nil || out.Len() != 0 {
		t.Fatalf("invalid inspection rendered as text: %q %v", out.String(), err)
	}
}
