//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseConfigGrammar(t *testing.T) {
	for name, test := range map[string]struct {
		input string
		want  []configLine
	}{
		"basic":                 {"[core]\n\tbare = false\n", []configLine{{key: "core.bare", value: "false", hasValue: true}}},
		"implicit true":         {"[core]\n\tbare\n", []configLine{{key: "core.bare"}}},
		"case folding":          {"[Core]\n\tBARE = x\n", []configLine{{key: "core.bare", value: "x", hasValue: true}}},
		"subsection keeps case": {"[remote \"Origin\"]\nurl = u\n", []configLine{{key: "remote.Origin.url", value: "u", hasValue: true}}},
		"subsection escapes":    {"[a \"x\\\"y\\\\z\\q\"]\nk = v\n", []configLine{{key: "a.x\"y\\zq.k", value: "v", hasValue: true}}},
		"deprecated subsection": {"[Branch.Main]\nk = v\n", []configLine{{key: "branch.main.k", value: "v", hasValue: true}}},
		"same line":             {"[core] bare = true\n", []configLine{{key: "core.bare", value: "true", hasValue: true}}},
		"comments": {"# c\n; c\n[core]\nk = v # tail\nl = \"a # b\" ; tail\n", []configLine{
			{key: "core.k", value: "v", hasValue: true}, {key: "core.l", value: "a # b", hasValue: true},
		}},
		"internal whitespace": {"[s]\nk =  a \t b  \n", []configLine{{key: "s.k", value: "a \t b", hasValue: true}}},
		"quoted whitespace":   {"[s]\nk = \" a\tb \"\n", []configLine{{key: "s.k", value: " a\tb ", hasValue: true}}},
		"escapes":             {"[s]\nk = a\\tb\\nc\\\\d\\\"e\\bf\n", []configLine{{key: "s.k", value: "a\tb\nc\\d\"e\bf", hasValue: true}}},
		"continuation":        {"[s]\nk = a\\\nb\n", []configLine{{key: "s.k", value: "ab", hasValue: true}}},
		"crlf":                {"[s]\r\nk = v\r\n", []configLine{{key: "s.k", value: "v", hasValue: true}}},
		"bom":                 {"\xef\xbb\xbf[s]\nk = v\n", []configLine{{key: "s.k", value: "v", hasValue: true}}},
		"empty value":         {"[s]\nk =\n", []configLine{{key: "s.k", value: "", hasValue: true}}},
		"no final newline":    {"[s]\nk = v", []configLine{{key: "s.k", value: "v", hasValue: true}}},
	} {
		got, err := parseConfig([]byte(test.input))
		if err != nil || !reflect.DeepEqual(got, test.want) {
			t.Errorf("%s: %+v %v, want %+v", name, got, err, test.want)
		}
	}
	for name, input := range map[string]string{
		"key without section":   "k = v\n",
		"bad escape":            "[s]\nk = a\\xb\n",
		"unterminated quote":    "[s]\nk = \"a\n",
		"comment after name":    "[s]\nk ; c\n",
		"newline in subsection": "[s \"a\nb\"]\n",
		"unclosed section":      "[s\n",
		"bad name":              "[s]\n1k = v\n",
		"junk after subsection": "[s \"a\" x]\n",
		"empty section":         "[]\n",
	} {
		if _, err := parseConfig([]byte(input)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

// configFixture is a fake user context: HOME, XDG and toolchain sources.
type configFixture struct {
	t      *testing.T
	home   string
	inputs ConfigInputs
	gitDir string
}

func newConfigFixture(t *testing.T) *configFixture {
	base := physicalTempDir(t)
	home := filepath.Join(base, "home")
	mustMkdir(t, home)
	gitDir := filepath.Join(base, "project", ".git")
	mustMkdir(t, gitDir)
	return &configFixture{t: t, home: home, gitDir: gitDir, inputs: ConfigInputs{
		Home: []byte(home), ToolchainSources: [][]byte{[]byte(filepath.Join(base, "toolchain", "gitconfig"))},
		GitDir: []byte(gitDir), CommonDir: []byte(gitDir),
	}}
}

func (f *configFixture) write(path, content string) string {
	mustWrite(f.t, path, content)
	return path
}

func (f *configFixture) evaluate() ConfigEvaluation {
	f.t.Helper()
	return EvaluateConfig(f.inputs)
}

func TestEvaluateConfigPrecedence(t *testing.T) {
	f := newConfigFixture(t)
	f.write(string(f.inputs.ToolchainSources[0]), "[core]\n\tautocrlf = true\n\tk = toolchain\n")
	f.write(filepath.Join(f.home, ".config", "git", "config"), "[core]\n\tk = xdg\n")
	f.write(filepath.Join(f.home, ".gitconfig"), "[core]\n\tk = global\n")
	f.write(filepath.Join(f.gitDir, "config"), "[core]\n\tk = local\n")
	evaluation := f.evaluate()
	if evaluation.State != ObservationObserved || len(evaluation.Incompatibilities) != 0 {
		t.Fatalf("evaluation %+v", evaluation)
	}
	if value, _, _ := evaluation.Last("core.k"); value != "local" {
		t.Fatalf("core.k = %q", value)
	}
	if value, _, _ := evaluation.Last("core.autocrlf"); value != "true" {
		t.Fatalf("core.autocrlf = %q", value)
	}
	var scopes []ConfigScope
	for i, entry := range evaluation.Entries {
		if entry.Ordinal != uint64(i) || entry.Validate() != nil {
			t.Fatalf("entry %d %+v", i, entry)
		}
		scopes = append(scopes, entry.Scope)
	}
	want := []ConfigScope{ConfigScopeSystem, ConfigScopeSystem, ConfigScopeXDG, ConfigScopeGlobal, ConfigScopeLocal}
	if !reflect.DeepEqual(scopes, want) {
		t.Fatalf("scopes %v", scopes)
	}
	// Every expected source is recorded, read or absent, in precedence order.
	for i, source := range evaluation.Sources {
		if source.Precedence != uint64(i) || source.Validate() != nil || source.ReadState != SourceReadRead {
			t.Fatalf("source %d %+v", i, source)
		}
	}
	// An explicit XDG_CONFIG_HOME replaces the default location.
	f.inputs.XDGConfigHome = []byte(filepath.Join(f.home, "xdg"))
	f.write(filepath.Join(f.home, "xdg", "git", "config"), "[core]\n\tx = 1\n")
	if _, _, found := f.evaluate().Last("core.x"); !found {
		t.Fatal("explicit XDG configuration ignored")
	}
}

func TestEvaluateConfigIncludes(t *testing.T) {
	f := newConfigFixture(t)
	f.write(filepath.Join(f.home, "inc", "rel.inc"), "[a]\n\trel = 1\n")
	f.write(filepath.Join(f.home, "tilde.inc"), "[a]\n\ttilde = 1\n")
	f.write(filepath.Join(f.home, "gitdir.inc"), "[a]\n\tgitdir = 1\n")
	f.write(filepath.Join(f.home, "nogitdir.inc"), "[a]\n\tnogitdir = 1\n")
	f.write(filepath.Join(f.home, "casefold.inc"), "[a]\n\tcasefold = 1\n")
	f.write(filepath.Join(f.home, "branch.inc"), "[a]\n\tbranch = 1\n")
	projectDir := filepath.Dir(f.gitDir)
	f.write(filepath.Join(f.home, ".gitconfig"), strings.Join([]string{
		"[include]", "\tpath = inc/rel.inc", "\tpath = ~/tilde.inc", "\tpath = missing.inc",
		"[includeIf \"gitdir:" + projectDir + "/\"]", "\tpath = gitdir.inc",
		"[includeIf \"gitdir:/nowhere/\"]", "\tpath = nogitdir.inc",
		"[includeIf \"gitdir/i:" + strings.ToUpper(projectDir) + "/\"]", "\tpath = casefold.inc",
		"[includeIf \"onbranch:feature/\"]", "\tpath = branch.inc",
		"[a]", "\tafter = 1", "",
	}, "\n"))
	f.inputs.Branch = []byte("feature/x")
	evaluation := f.evaluate()
	if evaluation.State != ObservationObserved || len(evaluation.Incompatibilities) != 0 {
		t.Fatalf("evaluation %+v", evaluation.Incompatibilities)
	}
	for key, want := range map[string]bool{"a.rel": true, "a.tilde": true, "a.gitdir": true, "a.nogitdir": false, "a.casefold": true, "a.branch": true, "a.after": true} {
		if _, _, found := evaluation.Last(key); found != want {
			t.Errorf("%s found=%t want %t", key, found, want)
		}
	}
	// Included entries follow their include entry, in order.
	var keys []string
	for _, entry := range evaluation.Entries {
		if entry.Scope == ConfigScopeGlobal {
			keys = append(keys, entry.Key)
		}
	}
	if keys[0] != "include.path" || keys[1] != "a.rel" || keys[len(keys)-1] != "a.after" {
		t.Fatalf("include order %v", keys)
	}
	absent := false
	for _, source := range evaluation.Sources {
		if source.ReadState == SourceReadAbsent && strings.HasSuffix(source.Path.DisplayPathUTF8, "missing.inc") {
			absent = true
		}
	}
	if !absent {
		t.Fatal("missing include not recorded as an absent source")
	}
	// Without a repository a gitdir condition is false, and an unknown
	// branch makes onbranch unprovable.
	f.inputs.Branch = nil
	if evaluation := f.evaluate(); evaluation.State != ObservationError || !hasReason(evaluation.Incompatibilities, ReasonAttributesIncompatible) {
		t.Fatalf("unknown branch: %+v", evaluation.Incompatibilities)
	}
	f.inputs.GitDir, f.inputs.CommonDir = nil, nil
	evaluation = f.evaluate()
	if _, _, found := evaluation.Last("a.gitdir"); found || evaluation.State != ObservationObserved {
		t.Fatalf("gitdir condition without a repository: %+v", evaluation.Incompatibilities)
	}
}

func TestEvaluateConfigFailsClosed(t *testing.T) {
	for name, setup := range map[string]func(f *configFixture){
		"hasconfig": func(f *configFixture) {
			f.write(filepath.Join(f.home, ".gitconfig"), "[includeIf \"hasconfig:remote.*.url:https://x/**\"]\n\tpath = x\n")
		},
		"unknown condition": func(f *configFixture) {
			f.write(filepath.Join(f.home, ".gitconfig"), "[includeIf \"bogus:x\"]\n\tpath = x\n")
		},
		"cycle": func(f *configFixture) {
			f.write(filepath.Join(f.home, ".gitconfig"), "[include]\n\tpath = ~/.gitconfig\n")
		},
		"depth": func(f *configFixture) {
			for i := 0; i < 12; i++ {
				f.write(filepath.Join(f.home, "d"+string(rune('a'+i))), "[include]\n\tpath = d"+string(rune('a'+i+1))+"\n")
			}
			f.write(filepath.Join(f.home, ".gitconfig"), "[include]\n\tpath = da\n")
		},
		"syntax": func(f *configFixture) { f.write(filepath.Join(f.home, ".gitconfig"), "[core\n") },
		"oversized": func(f *configFixture) {
			f.write(filepath.Join(f.home, ".gitconfig"), "[a]\n\tk = "+strings.Repeat("x", 1<<20)+"\n")
		},
		"include without value": func(f *configFixture) {
			f.write(filepath.Join(f.home, ".gitconfig"), "[include]\n\tpath\n")
		},
		"user tilde": func(f *configFixture) {
			f.write(filepath.Join(f.home, ".gitconfig"), "[include]\n\tpath = ~other/x\n")
		},
		"directory source":      func(f *configFixture) { mustMkdir(f.t, filepath.Join(f.home, ".gitconfig")) },
		"environment overrides": func(f *configFixture) { f.inputs.EnvironmentOverrides = true },
	} {
		t.Run(name, func(t *testing.T) {
			f := newConfigFixture(t)
			setup(f)
			evaluation := f.evaluate()
			if evaluation.State != ObservationError || !hasReason(evaluation.Incompatibilities, ReasonAttributesIncompatible) {
				t.Fatalf("state %s, incompatibilities %+v", evaluation.State, evaluation.Incompatibilities)
			}
		})
	}
	if os.Geteuid() != 0 {
		f := newConfigFixture(t)
		path := f.write(filepath.Join(f.home, ".gitconfig"), "[a]\n\tk = v\n")
		if err := os.Chmod(path, 0); err != nil {
			t.Fatal(err)
		}
		if evaluation := f.evaluate(); evaluation.State != ObservationError {
			t.Fatalf("unreadable source: %+v", evaluation)
		}
	}
}

func TestEvaluateConfigWorktreeExtension(t *testing.T) {
	f := newConfigFixture(t)
	f.write(filepath.Join(f.gitDir, "config"), "[extensions]\n\tworktreeConfig = true\n")
	evaluation := f.evaluate()
	if !hasReason(evaluation.Incompatibilities, ReasonWorktreeConfigUnsupported) {
		t.Fatalf("incompatibilities %+v", evaluation.Incompatibilities)
	}
}

// No credential, token or userinfo survives into the recorded entries.
func TestEvaluateConfigRedaction(t *testing.T) {
	f := newConfigFixture(t)
	f.write(filepath.Join(f.gitDir, "config"), strings.Join([]string{
		"[remote \"origin\"]", "\turl = https://alice:s3cret-token@example.com/org/repo.git?private_token=qtok#frag",
		"\tpushurl = git@example.com:org/repo.git",
		"[url \"https://bob:urltoken@mirror.example/\"]", "\tinsteadOf = https://example.com/",
		"[http]", "\textraHeader = Authorization: Bearer hdrtoken",
		"[credential]", "\thelper = store",
		"[core]", "\teditor = vim", "",
	}, "\n"))
	evaluation := f.evaluate()
	var rendered bytes.Buffer
	for _, entry := range evaluation.Entries {
		if err := entry.Validate(); err != nil {
			t.Fatalf("%+v: %v", entry, err)
		}
		rendered.WriteString(entry.Key + "=" + entry.ValueRedacted + "\n")
	}
	for _, secret := range []string{"alice", "s3cret-token", "qtok", "frag", "bob", "urltoken", "hdrtoken", "store"} {
		if strings.Contains(rendered.String(), secret) {
			t.Errorf("secret %q in %s", secret, rendered.String())
		}
	}
	for _, kept := range []string{"remote.origin.url=https://<redacted>@example.com/org/repo.git", "example.com:org/repo.git", "core.editor=vim"} {
		if !strings.Contains(rendered.String(), kept) {
			t.Errorf("missing %q in %s", kept, rendered.String())
		}
	}
	// The evaluator still reads the raw value internally.
	if value, _, _ := evaluation.Last("remote.origin.url"); !strings.Contains(value, "s3cret-token") {
		t.Fatal("raw value not available to evaluation")
	}
}

func TestRedactURL(t *testing.T) {
	for raw, want := range map[string]struct {
		display    string
		state      RedactionState
		credential bool
	}{
		"https://example.com/r.git":       {"https://example.com/r.git", RedactionNotNeeded, false},
		"https://u:p@example.com/r.git":   {"https://<redacted>@example.com/r.git", RedactionRedacted, true},
		"https://token@example.com/r.git": {"https://<redacted>@example.com/r.git", RedactionRedacted, true},
		"https://example.com/r.git?x=1":   {"https://example.com/r.git?<redacted>", RedactionRedacted, true},
		"git@example.com:org/r.git":       {"<redacted>@example.com:org/r.git", RedactionRedacted, false},
		"/srv/git/r.git":                  {"/srv/git/r.git", RedactionNotNeeded, false},
		"https://example.com/a b":         {OpaqueMarker, RedactionOpaque, false},
		"ssh://u@h/p\x01":                 {OpaqueMarker, RedactionOpaque, false},
	} {
		display, state, credential := redactURL(raw)
		if display != want.display || state != want.state || credential != want.credential {
			t.Errorf("%q: %q %s %t, want %+v", raw, display, state, credential, want)
		}
	}
}

func hasReason(list []Incompatibility, reason Reason) bool {
	for _, item := range list {
		if item.Reason == reason {
			return true
		}
	}
	return false
}

// ignoreFixture mirrors a worktree whose expected results were taken from
// Git 2.54 "git check-ignore --no-index" over the same files.
func ignoreFixture(t *testing.T) (string, IgnoreInputs) {
	base := physicalTempDir(t)
	root := filepath.Join(base, "w")
	mustWrite(t, filepath.Join(root, ".gitignore"), "*.log\n!keep.log\nbuild/\n/anchored.txt\ndocs/**/*.tmp\n\\#hash\ntrailing\\ \nfoo/bar\n!build/keep.c\n# comment\n\n   \n")
	mustWrite(t, filepath.Join(root, "sub", ".gitignore"), "!important.log\n*.dat\n/local-only\r\n")
	mustWrite(t, filepath.Join(root, ".git", "info", "exclude"), "secret.txt\n")
	mustWrite(t, filepath.Join(base, "excludes"), "*.bak\n")
	return root, IgnoreInputs{Root: []byte(root), GitDir: []byte(filepath.Join(root, ".git")), ExcludesFile: []byte(filepath.Join(base, "excludes"))}
}

func TestIgnoreEvaluatorMatchesGit(t *testing.T) {
	_, inputs := ignoreFixture(t)
	evaluator := NewIgnoreEvaluator(inputs, nil)
	for _, c := range []struct {
		path    string
		ignored bool
	}{
		{"a.log", true}, {"keep.log", false}, {"sub/important.log", false}, {"sub/other.log", true},
		{"build/x.c", true}, {"build/keep.c", true}, {"sub/build/y", true}, {"anchored.txt", true},
		{"sub/anchored.txt", false}, {"docs/a/b/c.tmp", true}, {"docs/c.tmp", true}, {"#hash", true},
		{"trailing ", true}, {"foo/bar", true}, {"sub/foo/bar", false}, {"sub/x.dat", true}, {"x.dat", false},
		{"sub/local-only", true}, {"local-only", false}, {"secret.txt", true}, {"sub/secret.txt", true},
		{"a.bak", true}, {"plain.txt", false},
	} {
		got := evaluator.Evaluate([]byte(c.path), false)
		if got.Ignored != c.ignored || !got.EquivalenceProven {
			t.Errorf("%q ignored=%t proven=%t, want %t", c.path, got.Ignored, got.EquivalenceProven, c.ignored)
		}
		evaluation, err := NewIgnoreEvaluation(got)
		if err != nil {
			t.Fatalf("%q: %v", c.path, err)
		}
		if evaluation.Ignored && len(evaluation.MatchedRules()) != 1 {
			t.Errorf("%q matched rules %v", c.path, evaluation.MatchedRules())
		}
	}
	for _, source := range evaluator.Sources() {
		if err := source.Validate(); err != nil {
			t.Fatalf("source %+v: %v", source, err)
		}
	}
	if !evaluator.Usable() {
		t.Fatal("evaluator unusable")
	}
}

func TestIgnoreEvaluatorCaseAndRefusals(t *testing.T) {
	root, inputs := ignoreFixture(t)
	inputs.IgnoreCase = true
	if got := NewIgnoreEvaluator(inputs, nil).Evaluate([]byte("A.LOG"), false); !got.Ignored {
		t.Fatal("case-insensitive match missed")
	}
	if err := os.Remove(filepath.Join(root, "sub", ".gitignore")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, ".gitignore"), filepath.Join(root, "sub", ".gitignore")); err != nil {
		t.Fatal(err)
	}
	inputs.IgnoreCase = false
	evaluator := NewIgnoreEvaluator(inputs, nil)
	got := evaluator.Evaluate([]byte("sub/x"), false)
	if evaluator.Usable() || got.EquivalenceProven || got.IncompatibilityReason != ReasonAttributesIncompatible {
		t.Fatalf("symlinked .gitignore accepted: %+v", got)
	}
}

func TestExcludesFilePath(t *testing.T) {
	f := newConfigFixture(t)
	if path, ok := ExcludesFilePath(f.evaluate(), f.inputs.Home, nil); !ok || string(path) != f.home+"/.config/git/ignore" {
		t.Fatalf("default %q %t", path, ok)
	}
	if path, ok := ExcludesFilePath(f.evaluate(), f.inputs.Home, []byte("/x")); !ok || string(path) != "/x/git/ignore" {
		t.Fatalf("XDG default %q %t", path, ok)
	}
	f.write(filepath.Join(f.home, ".gitconfig"), "[core]\n\texcludesFile = ~/ignores\n")
	if path, ok := ExcludesFilePath(f.evaluate(), f.inputs.Home, nil); !ok || string(path) != f.home+"/ignores" {
		t.Fatalf("configured %q %t", path, ok)
	}
	f.write(filepath.Join(f.home, ".gitconfig"), "[core]\n\texcludesFile = relative\n")
	if _, ok := ExcludesFilePath(f.evaluate(), f.inputs.Home, nil); ok {
		t.Fatal("relative excludesFile accepted")
	}
}

// attributeFixture mirrors a worktree whose expected attributes were taken
// from Git 2.54 "git check-attr -a" over the same files.
func attributeFixture(t *testing.T) (string, AttributeInputs) {
	base := physicalTempDir(t)
	root := filepath.Join(base, "w")
	mustWrite(t, filepath.Join(root, ".gitattributes"), "[attr]mymacro text -diff\n*.txt text\n*.bin binary\ndocs/*.md eol=crlf\n\"sp ace.c\" filter=lfs\n*.mac mymacro diff\nsub/*.txt eol=lf\n")
	mustWrite(t, filepath.Join(root, "sub", ".gitattributes"), "*.txt -text\nlocal.c ident\n")
	mustWrite(t, filepath.Join(root, ".git", "info", "attributes"), "*.c whitespace=fix\n")
	mustWrite(t, filepath.Join(base, "global"), "*.go linguist=go\n*.txt merge=union\n")
	return root, AttributeInputs{Root: []byte(root), GitDir: []byte(filepath.Join(root, ".git")), GlobalFile: []byte(filepath.Join(base, "global")), GlobalExplicit: true}
}

func TestAttributeEvaluatorMatchesGit(t *testing.T) {
	_, inputs := attributeFixture(t)
	evaluator := NewAttributeEvaluator(inputs, nil)
	for path, want := range map[string]map[string]string{
		"a.txt":          {"merge": "=union", "text": "set"},
		"sub/a.txt":      {"merge": "=union", "text": "unset", "eol": "=lf"},
		"x.bin":          {"binary": "set", "diff": "unset", "merge": "unset", "text": "unset"},
		"docs/r.md":      {"eol": "=crlf"},
		"docs/deep/r.md": {},
		"sp ace.c":       {"filter": "=lfs", "whitespace": "=fix"},
		"m.mac":          {"diff": "set", "text": "set", "mymacro": "set"},
		"sub/local.c":    {"whitespace": "=fix", "ident": "set"},
		"z.c":            {"whitespace": "=fix"},
		"file.go":        {"linguist": "=go"},
	} {
		attrs, _, ok := evaluator.Attributes([]byte(path))
		got := make(map[string]string, len(attrs))
		for name, state := range attrs {
			got[name] = state.String()
		}
		if !ok || !reflect.DeepEqual(got, want) {
			t.Errorf("%q: %v (ok=%t), want %v", path, got, ok, want)
		}
	}
	for _, source := range evaluator.Sources() {
		if err := source.Validate(); err != nil {
			t.Fatalf("source %+v: %v", source, err)
		}
	}
}

func TestAttributeEvaluatorFailsClosed(t *testing.T) {
	for name, content := range map[string]string{
		"negative pattern": "!x text\n",
		"invalid name":     "*.c te$t\n",
		"bad quote":        "\"unterminated text\n",
		"bad escape":       "\"a\\q\" text\n",
	} {
		root, inputs := attributeFixture(t)
		mustWrite(t, filepath.Join(root, ".gitattributes"), content)
		evaluator := NewAttributeEvaluator(inputs, nil)
		if _, _, ok := evaluator.Attributes([]byte("a.c")); ok {
			t.Errorf("%s accepted", name)
		}
	}
	root, inputs := attributeFixture(t)
	mustWrite(t, filepath.Join(root, "sub", ".gitattributes"), "[attr]m text\n")
	if _, _, ok := NewAttributeEvaluator(inputs, nil).Attributes([]byte("sub/x")); ok {
		t.Error("macro definition in a subdirectory accepted")
	}
	quoted, _, err := unquoteCStyle([]byte(`"a\tb\101\"c" rest`))
	if err != nil || string(quoted) != "a\tbA\"c" {
		t.Errorf("unquote %q %v", quoted, err)
	}
}

func TestConversionRisk(t *testing.T) {
	f := newConfigFixture(t)
	f.write(filepath.Join(f.home, ".gitconfig"), "[filter \"lfs\"]\n\tclean = git-lfs clean -- %f\n[filter \"smudgeonly\"]\n\tsmudge = x\n[filter \"strict\"]\n\trequired\n")
	config := f.evaluate()
	f.write(filepath.Join(f.home, ".gitconfig"), "[core]\n\tautocrlf = input\n")
	autocrlf := f.evaluate()
	set, unset, value := attrState{kind: 's'}, attrState{kind: 'u'}, func(v string) attrState { return attrState{kind: 'v', value: v} }
	for name, test := range map[string]struct {
		attrs   map[string]attrState
		config  ConfigEvaluation
		content []byte
		want    ConversionRisk
	}{
		"plain":                   {nil, config, []byte("a\n"), ConversionRiskNeutral},
		"text without CR":         {map[string]attrState{"text": set}, config, []byte("a\n"), ConversionRiskNeutral},
		"text with CRLF":          {map[string]attrState{"text": set}, config, []byte("a\r\n"), ConversionRiskMayTransform},
		"binary with CRLF":        {map[string]attrState{"text": unset}, config, []byte("a\r\n"), ConversionRiskNeutral},
		"auto binary content":     {map[string]attrState{"text": value("auto")}, config, []byte("a\r\n\x00"), ConversionRiskNeutral},
		"autocrlf with CRLF":      {nil, autocrlf, []byte("a\r\n"), ConversionRiskMayTransform},
		"eol implies text":        {map[string]attrState{"eol": value("lf")}, config, []byte("a\r\n"), ConversionRiskMayTransform},
		"unknown content":         {map[string]attrState{"text": set}, config, nil, ConversionRiskMayTransform},
		"clean filter":            {map[string]attrState{"filter": value("lfs")}, config, []byte("a"), ConversionRiskExternalCode},
		"smudge-only filter":      {map[string]attrState{"filter": value("smudgeonly")}, config, []byte("a"), ConversionRiskNeutral},
		"undefined filter":        {map[string]attrState{"filter": value("none")}, config, []byte("a"), ConversionRiskNeutral},
		"required missing filter": {map[string]attrState{"filter": value("strict")}, config, []byte("a"), ConversionRiskIncompatible},
		"ident with keyword":      {map[string]attrState{"ident": set}, config, []byte("$Id$"), ConversionRiskMayTransform},
		"ident without keyword":   {map[string]attrState{"ident": set}, config, []byte("x"), ConversionRiskNeutral},
		"working tree encoding":   {map[string]attrState{"working-tree-encoding": value("UTF-16")}, config, []byte("x"), ConversionRiskMayTransform},
	} {
		if got := conversionRisk(test.attrs, test.config, test.content); got != test.want {
			t.Errorf("%s: %s, want %s", name, got, test.want)
		}
	}
}
