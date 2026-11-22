package conventionalcommit

import (
	"strings"
	"unicode/utf8"
)

// MaxMessageBytes bounds a validated message (F008-FR-051).
const MaxMessageBytes = 1 << 20

// Footer is one parsed trailer: its token, its separator (": " or " #")
// and its value, continuation lines included.
type Footer struct {
	Token     string
	Separator string
	Value     string
}

// Validation is the read-only result of validating one message. Validity
// and breaking-change recognition are separate: an invalid message is
// never breaking, and a valid one is breaking only through "!" or an
// uppercase BREAKING CHANGE / BREAKING-CHANGE footer.
type Validation struct {
	Valid       bool
	Type        string
	Scope       string
	Description string
	Body        string
	Footers     []Footer
	Breaking    bool
	// Diagnostics explains an invalid message with fixed labels; it never
	// quotes the message.
	Diagnostics []string
}

// Validate checks message against Conventional Commits 1.0.0 (F008-FR-102,
// 105–107). It accepts one optional final LF; CR, NUL, invalid UTF-8 and
// any other line ending are invalid. Types of any letter case are accepted.
func Validate(message []byte) Validation {
	invalid := func(diagnostic string) Validation {
		return Validation{Diagnostics: []string{diagnostic}}
	}
	switch {
	case len(message) > MaxMessageBytes:
		return invalid("message exceeds its byte bound")
	case !utf8.Valid(message):
		return invalid("message is not valid UTF-8")
	case strings.ContainsAny(string(message), "\x00\r"):
		return invalid("message contains NUL or CR")
	}
	lines := strings.Split(strings.TrimSuffix(string(message), "\n"), "\n")
	v, ok := parseHeader(lines[0])
	if !ok {
		return invalid("header is not type[(scope)][!]: description")
	}
	if len(lines) > 1 {
		if lines[1] != "" {
			return invalid("the body must follow the header after one blank line")
		}
		if lines[len(lines)-1] == "" {
			return invalid("message ends with a blank line")
		}
		paragraphs := splitParagraphs(lines[2:])
		if len(paragraphs) == 0 {
			return invalid("message ends with a blank line")
		}
		last := paragraphs[len(paragraphs)-1]
		if _, _, _, isFooter := footerLine(last[0]); isFooter {
			footers, ok := parseFooters(last)
			if !ok {
				return invalid("a footer has no value")
			}
			v.Footers = footers
			paragraphs = paragraphs[:len(paragraphs)-1]
		}
		var body []string
		for _, paragraph := range paragraphs {
			body = append(body, strings.Join(paragraph, "\n"))
		}
		v.Body = strings.Join(body, "\n\n")
	}
	for _, footer := range v.Footers {
		if footer.Token == "BREAKING CHANGE" || footer.Token == "BREAKING-CHANGE" {
			v.Breaking = true
		}
	}
	v.Valid = true
	return v
}

// parseHeader parses type[(scope)][!]: description.
func parseHeader(header string) (Validation, bool) {
	i := 0
	for i < len(header) && isLetter(header[i]) {
		i++
	}
	if i == 0 {
		return Validation{}, false
	}
	v := Validation{Type: header[:i]}
	rest := header[i:]
	if strings.HasPrefix(rest, "(") {
		end := strings.IndexByte(rest, ')')
		if end < 2 || strings.ContainsAny(rest[1:end], "()") {
			return Validation{}, false
		}
		v.Scope, rest = rest[1:end], rest[end+1:]
	}
	if strings.HasPrefix(rest, "!") {
		v.Breaking, rest = true, rest[1:]
	}
	description, found := strings.CutPrefix(rest, ": ")
	if !found || strings.TrimSpace(description) == "" || description[0] == ' ' {
		return Validation{}, false
	}
	v.Description = description
	return v, true
}

func splitParagraphs(lines []string) [][]string {
	var paragraphs [][]string
	var current []string
	for _, line := range lines {
		if line == "" {
			if len(current) > 0 {
				paragraphs = append(paragraphs, current)
				current = nil
			}
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		paragraphs = append(paragraphs, current)
	}
	return paragraphs
}

// footerLine recognizes "token: value" and "token #value". A token is a
// word whose spaces are written as '-', or exactly BREAKING CHANGE.
func footerLine(line string) (token, separator, value string, ok bool) {
	for _, sep := range []string{": ", " #"} {
		at := strings.Index(line, sep)
		if at <= 0 {
			continue
		}
		token := line[:at]
		if token == "BREAKING CHANGE" || validToken(token) {
			return token, sep, line[at+len(sep):], true
		}
	}
	return "", "", "", false
}

func validToken(token string) bool {
	if !isLetter(token[0]) && !isDigit(token[0]) {
		return false
	}
	for i := 0; i < len(token); i++ {
		if !isLetter(token[i]) && !isDigit(token[i]) && token[i] != '-' {
			return false
		}
	}
	return true
}

// parseFooters parses the footer paragraph: each footer line starts a
// footer and any other line continues the previous one. A footer whose
// value is empty is malformed.
func parseFooters(lines []string) ([]Footer, bool) {
	var footers []Footer
	for _, line := range lines {
		if token, separator, value, ok := footerLine(line); ok {
			footers = append(footers, Footer{Token: token, Separator: separator, Value: value})
			continue
		}
		last := &footers[len(footers)-1]
		last.Value += "\n" + line
	}
	for _, footer := range footers {
		if strings.TrimSpace(footer.Value) == "" {
			return nil, false
		}
	}
	return footers, true
}

func isLetter(b byte) bool { return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' }
func isDigit(b byte) bool  { return b >= '0' && b <= '9' }
