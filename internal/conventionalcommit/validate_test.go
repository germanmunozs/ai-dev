package conventionalcommit

import (
	"strings"
	"testing"
)

// T097: the grammar corpus distinguishes validity from breaking-change
// recognition (F008-FR-102–107, F008-FR-147; AC-016).
func TestValidateCorpus(t *testing.T) {
	for _, test := range []struct {
		name            string
		message         string
		valid, breaking bool
	}{
		{"ordinary", "feat: add a parser\n", true, false},
		{"no final LF", "fix: handle EOF", true, false},
		{"scope", "fix(parser): handle EOF\n", true, false},
		{"uppercase type", "FEAT: shout\n", true, false},
		{"mixed case type", "Docs(readme): update\n", true, false},
		{"bang", "feat!: drop the old API\n", true, true},
		{"scope and bang", "feat(api)!: drop the old API\n", true, true},
		{"body", "feat: add\n\nA longer body.\nSecond line.\n", true, false},
		{"refs footer", "fix: crash\n\nRefs #123\n", true, false},
		{"body and footers", "fix: crash\n\nBody.\n\nReviewed-by: Z\nRefs #123\n", true, false},
		{"BREAKING CHANGE footer", "feat: api\n\nBREAKING CHANGE: the old call is gone\n", true, true},
		{"BREAKING-CHANGE footer", "feat: api\n\nBREAKING-CHANGE: the old call is gone\n", true, true},
		{"lowercase breaking-change is an ordinary footer", "feat: api\n\nbreaking-change: not breaking\n", true, false},
		{"multi-line footer value", "feat: api\n\nBREAKING CHANGE: first\ncontinued here\n", true, true},
		{"empty", "", false, false},
		{"no type", ": nothing\n", false, false},
		{"no space after colon", "feat:x\n", false, false},
		{"empty description", "feat: \n", false, false},
		{"empty scope", "feat(): x\n", false, false},
		{"digit type", "1feat: x\n", false, false},
		{"body without blank line", "feat: x\nbody\n", false, false},
		{"trailing blank line", "feat: x\n\nbody\n\n", false, false},
		{"CRLF", "feat: x\r\n", false, false},
		{"NUL", "feat: x\x00\n", false, false},
		{"invalid UTF-8", "feat: \xff\n", false, false},
		{"malformed footer", "fix: x\n\nRefs: \n", false, false},
		{"breaking bang on invalid header", "feat!:x\n", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			v := Validate([]byte(test.message))
			if v.Valid != test.valid || v.Breaking != test.breaking {
				t.Fatalf("valid=%v breaking=%v, want %v %v (%v)", v.Valid, v.Breaking, test.valid, test.breaking, v.Diagnostics)
			}
			if !v.Valid && len(v.Diagnostics) == 0 {
				t.Fatal("an invalid message has no diagnostic")
			}
		})
	}
}

func TestValidateParsesParts(t *testing.T) {
	v := Validate([]byte("fix(core)!: stop\n\nWhy it stops.\n\nRefs #9\nBREAKING CHANGE: gone\n"))
	if !v.Valid || v.Type != "fix" || v.Scope != "core" || v.Description != "stop" || v.Body != "Why it stops." || !v.Breaking {
		t.Fatalf("%+v", v)
	}
	if len(v.Footers) != 2 || v.Footers[0] != (Footer{"Refs", " #", "9"}) || v.Footers[1].Token != "BREAKING CHANGE" {
		t.Fatalf("footers %+v", v.Footers)
	}
}

func TestValidateBound(t *testing.T) {
	header := "feat: x\n\n"
	exact := header + strings.Repeat("a", MaxMessageBytes-len(header)-1) + "\n"
	if v := Validate([]byte(exact)); !v.Valid {
		t.Fatalf("exact bound refused: %v", v.Diagnostics)
	}
	if v := Validate([]byte(exact + "a")); v.Valid {
		t.Fatal("one byte over the bound accepted")
	}
}
