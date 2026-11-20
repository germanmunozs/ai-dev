package report

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// hostileValues are supplied strings that would run or expand something if
// a report line quoting them were pasted into a shell.
var hostileValues = []string{
	"f",
	"$(touch PWNED)",
	"`touch PWNED`",
	"${HOME}",
	"$HOME",
	"!!",
	"a!b",
	"\\$(touch PWNED)",
	"\\`touch PWNED\\`",
	`a"b`,
	"a\\b",
	"it's",
	"a;b",
	"a\nb",
	"a" + string(rune(0x202e)) + "b",
	"ñandú",
}

// unquoteShellInert reverses shellInertQuote: drop the extra backslash
// before '$', '`' and '!', then Go-unquote.
func unquoteShellInert(t *testing.T, quoted string) string {
	t.Helper()
	var plain strings.Builder
	for i := 0; i < len(quoted); i++ {
		if quoted[i] == '\\' && i+1 < len(quoted) {
			switch quoted[i+1] {
			case '$', '`', '!':
				continue
			case '\\':
				plain.WriteString(`\\`)
				i++
				continue
			}
		}
		plain.WriteByte(quoted[i])
	}
	value, err := strconv.Unquote(plain.String())
	if err != nil {
		t.Fatalf("unquote %s: %v", quoted, err)
	}
	return value
}

func TestShellInertQuoteIsReversibleAndOneLine(t *testing.T) {
	for _, value := range hostileValues {
		quoted := shellInertQuote(value)
		if strings.ContainsAny(quoted, "\n\r") {
			t.Fatalf("%q renders across lines: %s", value, quoted)
		}
		if got := unquoteShellInert(t, quoted); got != value {
			t.Fatalf("round trip of %q = %q via %s", value, got, quoted)
		}
		for i := 0; i < len(quoted); i++ {
			if strings.IndexByte("$`!", quoted[i]) >= 0 && (i == 0 || quoted[i-1] != '\\') {
				t.Fatalf("%q: unescaped %q at %d in %s", value, quoted[i], i, quoted)
			}
		}
	}
	if got := shellInertQuote("007-workflow-orchestration"); got != `"007-workflow-orchestration"` {
		t.Fatalf("ordinary identifier changed: %s", got)
	}
}

// TestShellInertQuotePastesInert pastes a status line quoting each hostile
// value into every shell mode: nothing may run, no file may appear, and the
// shell must see the quoted text, not an expansion of it.
func TestShellInertQuotePastesInert(t *testing.T) {
	shells := guideShells(t)
	sentinels := sentinelPath(t)
	for _, value := range hostileValues {
		if strings.ContainsAny(value, "\n") {
			continue // a raw newline is escaped as \n; nothing to paste across lines
		}
		line := "ai-dev: outcome=feature_unknown feature=" + shellInertQuote(value)
		for name, shell := range shells {
			invocations, changed := paste(t, sentinels, shell, line)
			if changed {
				t.Fatalf("%s: pasting %s changed files", name, line)
			}
			for _, invocation := range invocations {
				t.Fatalf("%s: pasting %s ran %q", name, line, invocation)
			}
		}
	}
}

// TestShellInertQuoteEchoesLiterally checks what the shell actually sees:
// printing the quoted value yields the value itself, never an expansion.
// ('!' is escaped for interactive history expansion, which a -c shell does
// not perform, so it is covered by the reversibility test instead.)
func TestShellInertQuoteEchoesLiterally(t *testing.T) {
	shells := guideShells(t)
	sentinels := sentinelPath(t)
	for _, value := range []string{"$(echo X)", "`echo X`", "${HOME}", "$HOME", `\$(echo X)`, `a"b`} {
		quoted := shellInertQuote(value)
		for name, shell := range shells {
			out := filepath.Join(t.TempDir(), "out")
			_, _ = paste(t, sentinels, shell, "printf '%s' "+quoted+" > "+out)
			data, err := os.ReadFile(out)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if string(data) != value {
				t.Fatalf("%s: printf %s = %q, want %q", name, quoted, data, value)
			}
		}
	}
}
