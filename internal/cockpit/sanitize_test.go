package cockpit

import (
	"fmt"
	"strings"
	"testing"
)

// TestSanitizeRendersHostileSequencesInert proves F012-FR-016 and SC-005 per
// class of sequence (R-05): the output holds no raw control byte and shows
// the sequence visibly.
func TestSanitizeRendersHostileSequencesInert(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"CSI", "a\x1b[2Jb", `a\x1b[2Jb`},
		{"OSC 52 clipboard", "\x1b]52;c;aGk=\x07", `\x1b]52;c;aGk=\x07`},
		{"OSC 8 hyperlink", "\x1b]8;;https://example.com\x1b\\x\x1b]8;;\x1b\\", `\x1b]8;;https://example.com\x1b\x\x1b]8;;\x1b\`},
		{"OSC title", "\x1b]0;owned\x07", `\x1b]0;owned\x07`},
		{"DCS", "\x1bP1$r\x1b\\", `\x1bP1$r\x1b\`},
		{"C1 NEL", "a\u0085b", `a\u0085b`},
		{"C1 CSI", "a\u009b2Jb", `a\u009b2Jb`},
		{"DEL", "a\x7fb", `a\x7fb`},
		// Bidirectional controls are built from code points, so this source
		// file holds none of them literally.
		{"ALM", "a" + runes(0x061c) + "b", "a" + escapes(0x061c) + "b"},
		{"LRM RLM", runes(0x200e, 0x200f), escapes(0x200e, 0x200f)},
		{"embeddings and overrides", runes(0x202a, 0x202b, 0x202c, 0x202d, 0x202e), escapes(0x202a, 0x202b, 0x202c, 0x202d, 0x202e)},
		{"isolates", runes(0x2066, 0x2067, 0x2068, 0x2069), escapes(0x2066, 0x2067, 0x2068, 0x2069)},
		{"lone ESC", "\x1b", `\x1b`},
		{"newline and carriage return", "a\nb\rc", `a\x0ab\x0dc`},
		{"invalid UTF-8", "a\xffb\xc3", "a�b�"},
		{"raw C1 byte", "a\x9bb", "a�b"},
		{"tab to the next 8-column stop", "ab\tc", "ab      c"},
		{"tab at a stop", "abcdefgh\tc", "abcdefgh        c"},
		{"ordinary text", "outcome=step_recommended feature=\"012\"", "outcome=step_recommended feature=\"012\""},
		{"accents and wide characters", "Germán 日本語 ✓", "Germán 日本語 ✓"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Sanitize(c.in).String()
			if got != c.want {
				t.Fatalf("Sanitize(%q) = %q, want %q", c.in, got, c.want)
			}
			if hasControl(got) {
				t.Fatalf("Sanitize(%q) = %q holds a control character", c.in, got)
			}
		})
	}
}

func runes(codePoints ...rune) string { return string(codePoints) }

// escapes renders code points in Sanitize's visible \uHHHH form.
func escapes(codePoints ...rune) string {
	var out strings.Builder
	for _, r := range codePoints {
		fmt.Fprintf(&out, "%cu%04x", '\\', r)
	}
	return out.String()
}

func hasControl(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool {
		return r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) || bidiControl(r)
	})
}

func TestZeroTextIsEmpty(t *testing.T) {
	var text Text
	if text.String() != "" {
		t.Fatalf("zero Text = %q", text.String())
	}
}
