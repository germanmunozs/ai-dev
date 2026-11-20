package report

import (
	"strconv"
	"strings"
)

// shellInertQuote renders a supplied value in the escaped, quoted form of
// 004 FR-028 (Amendment 5). It is Go's double-quoted form, so no value can
// split a one-line report or forge an outcome, with every '$', '`' and '!'
// also backslash-escaped. Inside double quotes a POSIX shell then performs
// no command substitution, parameter expansion or history expansion, so
// pasting a report line runs nothing the value contains.
func shellInertQuote(value string) string {
	quoted := strconv.Quote(value)
	var out strings.Builder
	out.Grow(len(quoted))
	for i := 0; i < len(quoted); i++ {
		switch quoted[i] {
		case '$', '`', '!':
			out.WriteByte('\\')
		}
		out.WriteByte(quoted[i])
	}
	return out.String()
}
