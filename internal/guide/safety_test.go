package guide

import "testing"

func TestTemplateSafe(t *testing.T) {
	unsafe := map[string]string{
		"NUL":                 "a\x00b",
		"newline":             "a\nb",
		"carriage return":     "a\rb",
		"tab":                 "a\tb",
		"escape":              "a\x1bb",
		"DEL":                 "a\x7fb",
		"C1 control":          "a\u0085b",
		"right-to-left":       "a\u202eb",
		"zero-width space":    "a\u200bb",
		"zero-width joiner":   "a\u200db",
		"byte order mark":     "\ufeffa",
		"line separator":      "a\u2028b",
		"paragraph separator": "a\u2029b",
	}
	for name, id := range unsafe {
		t.Run("unsafe "+name, func(t *testing.T) {
			if TemplateSafe(id) {
				t.Fatalf("TemplateSafe(%q) = true, want false", id)
			}
		})
	}

	safe := []string{"a;b", "$(x)", "-f", "it's", "a b", "ñandú", "007-workflow-orchestration", "`x`", `a"b`, `a\b`, "*", "~x"}
	for _, id := range safe {
		t.Run("safe "+id, func(t *testing.T) {
			if !TemplateSafe(id) {
				t.Fatalf("TemplateSafe(%q) = false, want true", id)
			}
		})
	}
}
