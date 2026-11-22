package conventionalcommit

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"
)

// T098: validation is total, deterministic and consistent for any bytes:
// it never panics, repeats its verdict, never marks an invalid message
// breaking, refuses CR, NUL and invalid UTF-8, and an accepted message
// reassembles from its parts (F008-FR-102–107, 147).
func FuzzConventionalCommitValidator(f *testing.F) {
	for _, seed := range []string{
		"feat: add\n", "FIX(core)!: x", "fix: x\n\nbody\n\nRefs #123\n", "feat: x\n\nBREAKING CHANGE: y\n",
		"feat: x\n\nbreaking-change: y\n", "feat: x\r\n", "\xff", "", "feat(): x", "feat: x\n\nRefs: \n",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, message []byte) {
		v := Validate(message)
		again := Validate(message)
		if v.Valid != again.Valid || v.Breaking != again.Breaking || v.Body != again.Body || len(v.Footers) != len(again.Footers) {
			t.Fatal("validation is not deterministic")
		}
		if v.Valid == (len(v.Diagnostics) > 0) || (!v.Valid && v.Breaking) {
			t.Fatalf("inconsistent result %+v", v)
		}
		if !v.Valid {
			return
		}
		if !utf8.Valid(message) || bytes.ContainsAny(message, "\x00\r") {
			t.Fatalf("accepted %q", message)
		}
		header := v.Type
		if v.Scope != "" {
			header += "(" + v.Scope + ")"
		}
		if strings.HasPrefix(strings.TrimPrefix(string(message), header), "!") {
			header += "!"
		}
		header += ": " + v.Description
		if !strings.HasPrefix(string(message), header) {
			t.Fatalf("header %q does not reassemble %q", header, message)
		}
		for _, footer := range v.Footers {
			if !strings.Contains(string(message), footer.Token+footer.Separator) {
				t.Fatalf("footer %+v not in %q", footer, message)
			}
		}
	})
}
