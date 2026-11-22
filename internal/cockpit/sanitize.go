package cockpit

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Text is a string that is safe to send to the terminal: Sanitize is its
// only constructor of a non-empty value, so a view that takes Text cannot
// render an untrusted string by mistake (R-05).
type Text struct{ s string }

// String returns the sanitized text.
func (t Text) String() string { return t.s }

// Sanitize renders s as inert text (F012-FR-016). Every C0 control and DEL
// becomes \xHH, every C1 control and bidirectional control becomes \uHHHH,
// invalid UTF-8 becomes U+FFFD, and a tab expands to spaces up to the next
// multiple of 8 columns, as a terminal would, so tab-separated report
// columns stay aligned.
func Sanitize(s string) Text {
	var out strings.Builder
	column := 0
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		s = s[size:]
		var piece string
		switch {
		case r == '\t':
			piece = strings.Repeat(" ", 8-column%8)
		case r == utf8.RuneError && size == 1:
			piece = "�"
		case r < 0x20 || r == 0x7f:
			piece = fmt.Sprintf(`\x%02x`, r)
		case (r >= 0x80 && r <= 0x9f) || bidiControl(r):
			piece = fmt.Sprintf(`\u%04x`, r)
		default:
			piece = string(r)
		}
		out.WriteString(piece)
		column += utf8.RuneCountInString(piece)
	}
	return Text{out.String()}
}

// bidiControl reports the bidirectional formatting characters that can
// reorder what the terminal shows (Trojan Source).
func bidiControl(r rune) bool {
	return r == 0x061c || r == 0x200e || r == 0x200f ||
		(r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069)
}

// SanitizeLines splits a report into lines and sanitizes each one, so no
// newline reaches Sanitize. A trailing newline adds no empty line.
func SanitizeLines(report string) []Text {
	report = strings.TrimSuffix(report, "\n")
	if report == "" {
		return nil
	}
	var lines []Text
	for _, line := range strings.Split(report, "\n") {
		lines = append(lines, Sanitize(line))
	}
	return lines
}
