package gitmanager

import (
	"bytes"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ignoreRule is one parsed gitignore line, with Git's dir.c flags.
type ignoreRule struct {
	pattern  []byte
	negated  bool
	dirOnly  bool
	basename bool // the pattern has no '/', so it matches the basename
	text     string
}

// ignoreList is the rules of one source, matched relative to base (the
// source's directory relative to the worktree root, "" for the root and
// for the global sources).
type ignoreList struct {
	source int
	base   []byte
	rules  []ignoreRule
}

// parseIgnore splits a gitignore source the way dir.c does: CRLF ends a
// line, empty lines and '#' comments are skipped, unescaped trailing
// spaces are trimmed, a leading '!' negates, a trailing '/' restricts the
// rule to directories, and a pattern without '/' matches basenames.
func parseIgnore(data []byte) []ignoreRule {
	var rules []ignoreRule
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimSuffix(line, []byte("\r"))
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		line = trimTrailingSpaces(line)
		if len(line) == 0 {
			continue
		}
		rule := ignoreRule{text: ruleText(line)}
		if line[0] == '!' {
			rule.negated, line = true, line[1:]
		}
		if n := len(line); n > 0 && line[n-1] == '/' {
			rule.dirOnly, line = true, line[:n-1]
		}
		if len(line) == 0 {
			continue
		}
		rule.basename = bytes.IndexByte(line, '/') < 0
		rule.pattern = line
		rules = append(rules, rule)
	}
	return rules
}

// trimTrailingSpaces removes trailing spaces that are not escaped by a
// backslash, as Git's trim_trailing_spaces does.
func trimTrailingSpaces(line []byte) []byte {
	end := -1
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case ' ':
			if end < 0 {
				end = i
			}
		case '\\':
			i++
			end = -1
		default:
			end = -1
		}
	}
	if end >= 0 {
		return line[:end]
	}
	return line
}

// ruleText is the rule as recorded evidence: the line itself when it is
// valid UTF-8, otherwise its quoted form.
func ruleText(line []byte) string {
	if utf8.Valid(line) {
		return string(line)
	}
	return strconv.Quote(string(line))
}

// matches reports whether rule matches path (relative to the worktree
// root) as Git's match_basename and match_pathname do.
func (l ignoreList) matches(rule ignoreRule, path []byte, isDir, ignoreCase bool) bool {
	if rule.dirOnly && !isDir {
		return false
	}
	var flags wildmatchFlags
	if ignoreCase {
		flags |= wildmatchCasefold
	}
	if rule.basename {
		name := path
		if i := bytes.LastIndexByte(path, '/'); i >= 0 {
			name = path[i+1:]
		}
		return wildmatch(rule.pattern, name, flags)
	}
	pattern := bytes.TrimPrefix(rule.pattern, []byte("/"))
	name := path
	if len(l.base) > 0 {
		prefix := append(append([]byte(nil), l.base...), '/')
		if len(path) < len(prefix) || !samePrefix(path[:len(prefix)], prefix, ignoreCase) {
			return false
		}
		name = path[len(prefix):]
	}
	return wildmatch(pattern, name, flags|wildmatchPathname)
}

func samePrefix(a, b []byte, ignoreCase bool) bool {
	if ignoreCase {
		return bytes.EqualFold(a, b)
	}
	return bytes.Equal(a, b)
}

// IgnoreInputs are the ignore sources of one worktree.
type IgnoreInputs struct {
	// Root is the physical worktree root.
	Root []byte
	// GitDir holds info/exclude; empty when the project has no repository.
	GitDir []byte
	// ExcludesFile is the effective core.excludesFile, already expanded;
	// empty when no such file applies.
	ExcludesFile []byte
	IgnoreCase   bool
}

// IgnoreEvaluator decides ignore status in process (F008-FR-026), reading
// each applicable source once.
type IgnoreEvaluator struct {
	in       IgnoreInputs
	budget   *Budget
	sources  []IgnoreSource
	global   []ignoreList // lowest precedence first
	perDir   map[string]*ignoreList
	unusable bool
}

// NewIgnoreEvaluator reads the global sources: core.excludesFile, then
// info/exclude.
func NewIgnoreEvaluator(in IgnoreInputs, budget *Budget) *IgnoreEvaluator {
	e := &IgnoreEvaluator{in: in, budget: budget, perDir: make(map[string]*ignoreList)}
	if len(in.ExcludesFile) > 0 {
		e.load(IgnoreSourceCoreExcludesFile, "core_excludes_file", in.ExcludesFile, nil, false, false)
	}
	if len(in.GitDir) > 0 {
		e.load(IgnoreSourceInfoExclude, "info_exclude", []byte(string(in.GitDir)+"/info/exclude"), nil, false, false)
	}
	return e
}

// load reads one source and registers its rules. In-tree sources are
// refused when they are symlinks, as Git refuses them.
func (e *IgnoreEvaluator) load(kind IgnoreSourceKind, scope string, path, base []byte, inTree, global bool) *ignoreList {
	var read sourceRead
	if inTree {
		read = readTreeSource(scope, path, uint64(len(e.sources)), e.budget)
	} else {
		read = readSource(scope, path, uint64(len(e.sources)), e.budget)
	}
	source := IgnoreSource{Identity: read.identity, Kind: kind}
	if kind == IgnoreSourceRepository {
		source.BaseDirectory = NewObservedPath(append([]byte(nil), e.in.Root...))
		if len(base) > 0 {
			source.BaseDirectory = NewObservedPath([]byte(string(e.in.Root) + "/" + string(base)))
		}
		source.HasDirectoryDepth = true
		if len(base) > 0 {
			source.DirectoryDepth = uint64(bytes.Count(base, []byte("/")) + 1)
		}
	}
	e.sources = append(e.sources, source)
	if read.failed() {
		e.unusable = true
	}
	list := &ignoreList{source: len(e.sources) - 1, base: base, rules: parseIgnore(read.data)}
	if kind != IgnoreSourceRepository {
		e.global = append(e.global, *list)
	}
	return list
}

// dirList returns the .gitignore rules of one directory, read on first use.
func (e *IgnoreEvaluator) dirList(dir []byte) *ignoreList {
	if list, ok := e.perDir[string(dir)]; ok {
		return list
	}
	path := string(e.in.Root) + "/.gitignore"
	if len(dir) > 0 {
		path = string(e.in.Root) + "/" + string(dir) + "/.gitignore"
	}
	list := e.load(IgnoreSourceRepository, "repository_gitignore", []byte(path), append([]byte(nil), dir...), true, false)
	e.perDir[string(dir)] = list
	return list
}

// lastMatch applies Git's precedence to one path: per-directory lists from
// the deepest ancestor to the root, then info/exclude, then
// core.excludesFile; inside a list the last matching rule wins.
func (e *IgnoreEvaluator) lastMatch(path []byte, isDir bool) (*ignoreList, *ignoreRule) {
	var lists []*ignoreList
	for end := len(path); end >= 0; end-- {
		if end < len(path) && path[end] != '/' && end != 0 {
			continue
		}
		if end == len(path) {
			continue
		}
		lists = append(lists, e.dirList(path[:end]))
	}
	for i := len(e.global) - 1; i >= 0; i-- {
		lists = append(lists, &e.global[i])
	}
	for _, list := range lists {
		for i := len(list.rules) - 1; i >= 0; i-- {
			if list.matches(list.rules[i], path, isDir, e.in.IgnoreCase) {
				return list, &list.rules[i]
			}
		}
	}
	return nil, nil
}

// Evaluate decides whether path (relative to the root, '/'-separated) is
// ignored. A path inside an excluded directory is ignored by that
// directory's rule and cannot be re-included, as in Git.
func (e *IgnoreEvaluator) Evaluate(path []byte, isDir bool) IgnoreEvaluationInput {
	in := IgnoreEvaluationInput{Path: NewObservedPath(append([]byte(nil), path...))}
	decided := false
	for end := 0; end < len(path) && !decided; end++ {
		if path[end] != '/' {
			continue
		}
		if list, rule := e.lastMatch(path[:end], true); rule != nil && !rule.negated {
			e.decide(&in, list, rule)
			decided = true
		}
	}
	if !decided {
		if list, rule := e.lastMatch(path, isDir); rule != nil {
			e.decide(&in, list, rule)
		}
	}
	for _, source := range e.sources {
		if source.Identity.ReadState == SourceReadRead {
			in.OrderedSourceDigests = append(in.OrderedSourceDigests, source.Identity.ContentDigest)
		}
	}
	if e.unusable {
		in.IncompatibilityReason = ReasonAttributesIncompatible
	} else {
		in.EquivalenceProven = true
	}
	return in
}

func (e *IgnoreEvaluator) decide(in *IgnoreEvaluationInput, list *ignoreList, rule *ignoreRule) {
	in.Ignored = !rule.negated
	in.MatchedRules = []string{rule.text}
	in.WinningRuleSource = e.sources[list.source].Identity.ContentDigest
}

// Sources lists every source read so far, in reading order.
func (e *IgnoreEvaluator) Sources() []IgnoreSource { return cloneSlice(e.sources) }

// Usable reports whether every source was read safely.
func (e *IgnoreEvaluator) Usable() bool { return !e.unusable }

// ExcludesFilePath resolves core.excludesFile as Git does: the configured
// value with "~/" expanded, or the default $XDG_CONFIG_HOME/git/ignore
// (with $HOME/.config as the default XDG base). ok is false for a value
// Git would expand differently.
func ExcludesFilePath(config ConfigEvaluation, home, xdgConfigHome []byte) ([]byte, bool) {
	if value, hasValue, found := config.Last("core.excludesfile"); found {
		switch {
		case !hasValue || value == "":
			return nil, false
		case strings.HasPrefix(value, "~/"):
			return []byte(string(home) + value[1:]), len(home) > 0
		case strings.HasPrefix(value, "/"):
			return []byte(value), true
		default:
			return nil, false
		}
	}
	xdg := string(xdgConfigHome)
	if xdg == "" {
		if len(home) == 0 {
			return nil, true
		}
		xdg = string(home) + "/.config"
	}
	return []byte(xdg + "/git/ignore"), true
}
