package gitmanager

// wildmatch is a port of Git's wildmatch.c, the matcher behind gitignore,
// gitattributes and includeIf patterns. Ignore and attribute matching use
// pathname mode, where '*', '?' and bracket expressions never match '/'
// and "**" between slashes matches any number of directories. Character
// classes use Git's ASCII-only ctype, so bytes are compared exactly.

type wildmatchFlags uint8

const (
	wildmatchPathname wildmatchFlags = 1 << iota
	wildmatchCasefold
)

const (
	wildMatch = iota
	wildNoMatch
	wildAbortAll
	wildAbortToStarStar
)

func wildmatch(pattern, text []byte, flags wildmatchFlags) bool {
	return dowild(pattern, text, flags) == wildMatch
}

func byteAt(b []byte, i int) byte {
	if i < len(b) {
		return b[i]
	}
	return 0
}

func lowerASCII(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

func dowild(pattern, text []byte, flags wildmatchFlags) int {
	p, t := 0, 0
	for ; p < len(pattern); t, p = t+1, p+1 {
		pc := pattern[p]
		tc := byteAt(text, t)
		if t >= len(text) && pc != '*' {
			return wildAbortAll
		}
		if flags&wildmatchCasefold != 0 {
			tc, pc = lowerASCII(tc), lowerASCII(pc)
		}
		switch pc {
		case '\\':
			// As in Git, the escaped byte itself is not case-folded.
			p++
			pc = byteAt(pattern, p)
			if p >= len(pattern) || tc != pc {
				return wildNoMatch
			}
			continue
		case '?':
			if flags&wildmatchPathname != 0 && tc == '/' {
				return wildNoMatch
			}
			continue
		case '*':
			var matchSlash bool
			p++
			if byteAt(pattern, p) == '*' {
				prev := p - 2
				for p++; byteAt(pattern, p) == '*'; p++ {
				}
				if (prev < 0 || pattern[prev] == '/') &&
					(p >= len(pattern) || pattern[p] == '/' || (pattern[p] == '\\' && byteAt(pattern, p+1) == '/')) {
					if byteAt(pattern, p) == '/' && p < len(pattern) && dowild(pattern[p+1:], text[t:], flags) == wildMatch {
						return wildMatch
					}
					matchSlash = true
				}
			} else {
				matchSlash = flags&wildmatchPathname == 0
			}
			if p >= len(pattern) {
				// A trailing "**" matches everything; a trailing "*" only
				// when no slash remains.
				if !matchSlash {
					for _, c := range text[t:] {
						if c == '/' {
							return wildNoMatch
						}
					}
				}
				return wildMatch
			}
			if !matchSlash && pattern[p] == '/' {
				// One star followed by a slash matches the next directory.
				slash := -1
				for i := t; i < len(text); i++ {
					if text[i] == '/' {
						slash = i
						break
					}
				}
				if slash < 0 {
					return wildNoMatch
				}
				t = slash
				// The slash is consumed by the loop.
				continue
			}
			for {
				if t >= len(text) {
					break
				}
				// Advance quickly to a literal that follows the star.
				if next := pattern[p]; next != '*' && next != '?' && next != '[' && next != '\\' {
					want := next
					if flags&wildmatchCasefold != 0 {
						want = lowerASCII(want)
					}
					for t < len(text) && (matchSlash || text[t] != '/') {
						c := text[t]
						if flags&wildmatchCasefold != 0 {
							c = lowerASCII(c)
						}
						if c == want {
							break
						}
						t++
					}
					c := byteAt(text, t)
					if flags&wildmatchCasefold != 0 {
						c = lowerASCII(c)
					}
					if t >= len(text) || c != want {
						return wildNoMatch
					}
				}
				matched := dowild(pattern[p:], text[t:], flags)
				if matched != wildNoMatch {
					if !matchSlash || matched != wildAbortToStarStar {
						return matched
					}
				} else if !matchSlash && byteAt(text, t) == '/' {
					return wildAbortToStarStar
				}
				t++
			}
			return wildAbortAll
		case '[':
			p++
			pc = byteAt(pattern, p)
			negated := pc == '!' || pc == '^'
			if negated {
				p++
				pc = byteAt(pattern, p)
			}
			var prev byte
			matched := false
			for {
				if p >= len(pattern) {
					return wildAbortAll
				}
				switch {
				case pc == '\\':
					p++
					if p >= len(pattern) {
						return wildAbortAll
					}
					pc = pattern[p]
					if tc == pc {
						matched = true
					}
				case pc == '-' && prev != 0 && p+1 < len(pattern) && pattern[p+1] != ']':
					p++
					pc = pattern[p]
					if pc == '\\' {
						p++
						if p >= len(pattern) {
							return wildAbortAll
						}
						pc = pattern[p]
					}
					if tc <= pc && tc >= prev {
						matched = true
					} else if flags&wildmatchCasefold != 0 && tc >= 'a' && tc <= 'z' {
						if upper := tc - 'a' + 'A'; upper <= pc && upper >= prev {
							matched = true
						}
					}
					pc = 0
				case pc == '[' && byteAt(pattern, p+1) == ':':
					start := p + 2
					end := start
					for end < len(pattern) && pattern[end] != ']' {
						end++
					}
					if end >= len(pattern) {
						return wildAbortAll
					}
					if end-start-1 < 0 || pattern[end-1] != ':' {
						// No ":]": a literal '['.
						p = start - 2
						pc = '['
						if tc == pc {
							matched = true
						}
						break
					}
					p = end
					class, ok := characterClass(string(pattern[start:end-1]), tc, flags)
					if !ok {
						return wildAbortAll
					}
					if class {
						matched = true
					}
					pc = 0
				default:
					if tc == pc {
						matched = true
					}
				}
				prev = pc
				p++
				pc = byteAt(pattern, p)
				if pc == ']' {
					break
				}
			}
			if matched == negated || (flags&wildmatchPathname != 0 && tc == '/') {
				return wildNoMatch
			}
			continue
		default:
			if tc != pc {
				return wildNoMatch
			}
		}
	}
	if t < len(text) {
		return wildNoMatch
	}
	return wildMatch
}

// characterClass evaluates a POSIX class with Git's ASCII-only ctype; ok is
// false for an unknown class name.
func characterClass(name string, c byte, flags wildmatchFlags) (bool, bool) {
	upper := c >= 'A' && c <= 'Z'
	lower := c >= 'a' && c <= 'z'
	digit := c >= '0' && c <= '9'
	space := c == ' ' || c == '\t' || c == '\n' || c == '\r'
	print := c >= 0x20 && c <= 0x7e
	switch name {
	case "alnum":
		return upper || lower || digit, true
	case "alpha":
		return upper || lower, true
	case "blank":
		return c == ' ' || c == '\t', true
	case "cntrl":
		return c < 0x20 || c == 0x7f, true
	case "digit":
		return digit, true
	case "graph":
		return print && c != ' ', true
	case "lower":
		return lower, true
	case "print":
		return print, true
	case "punct":
		return print && c != ' ' && !upper && !lower && !digit, true
	case "space":
		return space, true
	case "upper":
		return upper || (flags&wildmatchCasefold != 0 && lower), true
	case "xdigit":
		return digit || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F'), true
	default:
		return false, false
	}
}
