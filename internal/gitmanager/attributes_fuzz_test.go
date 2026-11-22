package gitmanager

import "testing"

// FuzzAttributeParser checks that arbitrary gitattributes input either
// fails closed or yields rules whose every assignment is a valid name with
// a known state, and that matching any parsed pattern cannot panic.
func FuzzAttributeParser(f *testing.F) {
	for _, seed := range []string{
		"*.txt text\n", "[attr]m text -diff\n*.c m\n", "\"sp\\tace\" filter=x eol=lf\n", "!neg text\n",
		"dir/ -text\n", "**/a/** !text\n", "\"\\101\\\"\" a=b=c\n", "# c\n\n  *.go linguist=go\r\n",
	} {
		f.Add([]byte(seed), []byte("dir/a.txt"), true)
	}
	f.Fuzz(func(t *testing.T, data, path []byte, topLevel bool) {
		rules, err := parseAttributes(data, topLevel)
		if err != nil {
			return
		}
		for _, rule := range rules {
			if rule.macro != "" && (!topLevel || !validAttrName(rule.macro)) {
				t.Fatalf("macro %q accepted", rule.macro)
			}
			if rule.macro == "" && (len(rule.pattern) == 0 || rule.pattern[0] == '!') {
				t.Fatalf("pattern %q accepted", rule.pattern)
			}
			for _, a := range rule.assigns {
				if !validAttrName(a.name) || (a.state.kind != 's' && a.state.kind != 'u' && a.state.kind != '!' && a.state.kind != 'v') {
					t.Fatalf("assignment %+v accepted", a)
				}
			}
			list := ignoreList{}
			_ = list.matches(ignoreRule{pattern: rule.pattern, dirOnly: rule.dirOnly, basename: rule.basename}, path, false, topLevel)
		}
	})
}
