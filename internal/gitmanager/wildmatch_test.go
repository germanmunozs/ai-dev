package gitmanager

import "testing"

// Cases from Git's t3070-wildmatch.sh: glob is the non-pathname result and
// path the WM_PATHNAME result.
func TestWildmatchGitCases(t *testing.T) {
	for _, c := range []struct {
		glob, path    bool
		text, pattern string
	}{
		{true, true, "foo", "foo"},
		{false, false, "foo", "bar"},
		{true, true, "", ""},
		{true, true, "foo", "???"},
		{false, false, "foo", "??"},
		{true, true, "foo", "*"},
		{true, true, "foo", "f*"},
		{false, false, "foo", "*f"},
		{true, true, "foo", "*foo*"},
		{true, true, "foobar", "*ob*a*r*"},
		{true, true, "aaaaaaabababab", "*ab"},
		{true, true, "foo*", `foo\*`},
		{false, false, "foobar", `foo\*bar`},
		{true, true, `f\oo`, `f\\oo`},
		{true, true, "ball", "*[al]?"},
		{false, false, "ten", "[ten]"},
		{true, true, "ten", "**[!te]"},
		{false, false, "ten", "**[!ten]"},
		{true, true, "ten", "t[a-g]n"},
		{false, false, "ten", "t[!a-g]n"},
		{true, true, "ton", "t[!a-g]n"},
		{true, true, "ton", "t[^a-g]n"},
		{true, true, "a]b", "a[]]b"},
		{true, true, "a-b", "a[]-]b"},
		{true, true, "a]b", "a[]-]b"},
		{false, false, "aab", "a[]-]b"},
		{true, true, "aab", "a[]a-]b"},
		{true, true, "]", "]"},
		{true, false, "foo/baz/bar", "foo*bar"},
		{true, true, "foobazbar", "foo**bar"},
		{true, true, "foo/baz/bar", "foo/**/bar"},
		{true, true, "foo/baz/bar", "foo/**/**/bar"},
		{true, true, "foo/b/a/z/bar", "foo/**/bar"},
		{true, true, "foo/b/a/z/bar", "foo/**/**/bar"},
		{true, true, "foo/bar", "foo/**/bar"},
		{true, true, "foo/bar", "foo/**/**/bar"},
		{true, false, "foo/bar", "foo?bar"},
		{true, false, "foo/bar", "foo[/]bar"},
		{true, false, "foo/bar", "foo[^a-z]bar"},
		{true, false, "foo/bar", "f[^eiu][^eiu][^eiu][^eiu][^eiu]r"},
		{true, true, "foo-bar", "f[^eiu][^eiu][^eiu][^eiu][^eiu]r"},
		{true, true, "foo", "**/foo"},
		{true, true, "XXX/foo", "**/foo"},
		{true, true, "bar/baz/foo", "**/foo"},
		{true, false, "bar/baz/foo", "*/foo"},
		{true, false, "foo/bar/baz", "**/bar*"},
		{true, true, "deep/foo/bar/baz", "**/bar/*"},
		{true, false, "deep/foo/bar/baz/", "**/bar/*"},
		{true, true, "deep/foo/bar/baz/", "**/bar/**"},
		{false, false, "deep/foo/bar", "**/bar/*"},
		{true, true, "deep/foo/bar/", "**/bar/**"},
		{true, false, "deep/foo/bar/baz/x", "*/bar/**"},
		{true, true, "deep/foo/bar/baz/x", "**/bar/*/*"},
		{true, true, "a1B", "[[:alpha:]][[:digit:]][[:upper:]]"},
		{false, false, "a", "[[:digit:][:upper:][:space:]]"},
		{true, true, "A", "[[:digit:][:upper:][:space:]]"},
		{true, true, "1", "[[:digit:][:upper:][:space:]]"},
		{true, true, " ", "[[:digit:][:upper:][:space:]]"},
		{false, false, ".", "[[:digit:][:upper:][:space:]]"},
		{true, true, "5", "[[:xdigit:]]"},
		{true, true, "f", "[[:xdigit:]]"},
		{true, true, "D", "[[:xdigit:]]"},
		{true, true, "_", "[[:alnum:][:alpha:][:blank:][:cntrl:][:digit:][:graph:][:lower:][:print:][:punct:][:space:][:upper:][:xdigit:]]"},
		{true, true, ".", "[^[:alnum:][:alpha:][:blank:][:cntrl:][:digit:][:lower:][:space:][:upper:][:xdigit:]]"},
		{true, true, "5", "[a-c[:digit:]x-z]"},
		{true, true, "b", "[a-c[:digit:]x-z]"},
		{true, true, "y", "[a-c[:digit:]x-z]"},
		{false, false, "q", "[a-c[:digit:]x-z]"},
		{false, false, "a", "[[:digit:]-z]"},
		{true, true, "-adobe-courier-bold-o-normal--12-120-75-75-m-70-iso8859-1", "-*-*-*-*-*-*-12-*-*-*-m-*-*-*"},
		{false, false, "-adobe-courier-bold-o-normal--12-120-75-75-X-70-iso8859-1", "-*-*-*-*-*-*-12-*-*-*-m-*-*-*"},
		{true, true, "XXX/adobe/courier/bold/o/normal//12/120/75/75/m/70/iso8859/1", "XXX/*/*/*/*/*/*/12/*/*/*/m/*/*/*"},
		{false, false, "XXX/adobe/courier/bold/o/normal//12/120/75/75/X/70/iso8859/1", "XXX/*/*/*/*/*/*/12/*/*/*/m/*/*/*"},
		{true, true, "abcd/abcdefg/abcdefghijk/abcdefghijklmnop.txt", "**/*a*b*g*n*t"},
		{false, false, "abcd/abcdefg/abcdefghijk/abcdefghijklmnop.txtz", "**/*a*b*g*n*t"},
		{false, false, "a", "[\\"},
		{false, false, "a", "[!"},
		{false, false, "a", "[[:bogus:]]"},
		{true, true, "[", "[[]"},
		{true, true, "\\", `[\\]`},
	} {
		if got := wildmatch([]byte(c.pattern), []byte(c.text), 0); got != c.glob {
			t.Errorf("glob %q ~ %q = %t, want %t", c.pattern, c.text, got, c.glob)
		}
		if got := wildmatch([]byte(c.pattern), []byte(c.text), wildmatchPathname); got != c.path {
			t.Errorf("path %q ~ %q = %t, want %t", c.pattern, c.text, got, c.path)
		}
	}
}

// Pathname results confirmed against Git 2.54 glob pathspecs, which match
// with WM_PATHNAME.
func TestWildmatchVerifiedAgainstGit(t *testing.T) {
	for _, c := range []struct {
		match         bool
		text, pattern string
	}{
		{false, "foo/baz/bar", "foo**bar"},
		{true, "foobazbar", "foo**bar"},
		{true, "foo/baz/bar", "**/bar**"},
		{false, "foo/bar/baz", "**/bar**"},
		{true, "foo/bar/baz/x", "*/bar/**"},
		{false, "deep/foo/bar/baz/x", "*/bar/**"},
		{true, "deep/foo/bar/baz/x", "*/foo/**"},
		{true, "foo/bar/baz/x", "**/bar/*/*"},
	} {
		if got := wildmatch([]byte(c.pattern), []byte(c.text), wildmatchPathname); got != c.match {
			t.Errorf("path %q ~ %q = %t, want %t", c.pattern, c.text, got, c.match)
		}
	}
}

func TestWildmatchCasefold(t *testing.T) {
	for _, c := range []struct {
		match         bool
		text, pattern string
	}{
		{true, "a", "[A-Z]"},
		{true, "A", "[a-z]"},
		{true, "a", "[[:upper:]]"},
		{true, "FOO", "foo"},
		{true, "Foo/Bar", "foo/*"},
		{false, "a", `\A`},
	} {
		if got := wildmatch([]byte(c.pattern), []byte(c.text), wildmatchPathname|wildmatchCasefold); got != c.match {
			t.Errorf("casefold %q ~ %q = %t, want %t", c.pattern, c.text, got, c.match)
		}
	}
}

func FuzzWildmatch(f *testing.F) {
	for _, seed := range []string{"**/foo", "[a-c[:digit:]]", `\*`, "a/**/b", "[!", "*?*"} {
		f.Add(seed, "a/b/foo")
	}
	f.Fuzz(func(t *testing.T, pattern, text string) {
		_ = wildmatch([]byte(pattern), []byte(text), wildmatchPathname)
		_ = wildmatch([]byte(pattern), []byte(text), wildmatchCasefold)
	})
}
