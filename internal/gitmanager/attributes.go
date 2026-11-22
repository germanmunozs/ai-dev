package gitmanager

import (
	"bytes"
	"errors"
	"strings"
)

// attrState is one attribute state: set, unset (-name), unspecified
// (!name) or a value (name=value).
type attrState struct {
	kind  byte // 's' set, 'u' unset, '!' unspecified, 'v' value
	value string
}

func (s attrState) String() string {
	switch s.kind {
	case 's':
		return "set"
	case 'u':
		return "unset"
	case 'v':
		return "=" + s.value
	default:
		return "unspecified"
	}
}

type attrAssignment struct {
	name  string
	state attrState
}

// attrRule is one .gitattributes line: a pattern with assignments, or a
// macro definition when macro is set.
type attrRule struct {
	pattern  []byte
	dirOnly  bool
	basename bool
	macro    string
	assigns  []attrAssignment
	text     string
}

var errAttributeSyntax = errors.New("unsupported gitattributes syntax")

// builtinMacros are Git's predefined macros.
var builtinMacros = map[string][]attrAssignment{
	"binary": {{"diff", attrState{kind: 'u'}}, {"merge", attrState{kind: 'u'}}, {"text", attrState{kind: 'u'}}},
}

// parseAttributes parses one gitattributes source with the grammar of
// Git's attr.c. Lines Git would warn about and skip are errors here, so an
// evaluation never silently differs: negative patterns, invalid attribute
// names, and macro definitions outside a top-level source.
func parseAttributes(data []byte, topLevel bool) ([]attrRule, error) {
	var rules []attrRule
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimLeft(line, " \t\r\n")
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		rule := attrRule{text: ruleText(bytes.TrimRight(line, " \t\r"))}
		var pattern []byte
		if line[0] == '"' {
			unquoted, rest, err := unquoteCStyle(line)
			if err != nil {
				return nil, err
			}
			pattern, line = unquoted, rest
		} else {
			end := bytes.IndexAny(line, " \t\r")
			if end < 0 {
				end = len(line)
			}
			pattern, line = line[:end], line[end:]
		}
		if name, ok := bytes.CutPrefix(pattern, []byte("[attr]")); ok {
			if !topLevel || !validAttrName(string(name)) {
				return nil, errAttributeSyntax
			}
			rule.macro = string(name)
		} else {
			if len(pattern) == 0 || pattern[0] == '!' {
				return nil, errAttributeSyntax
			}
			if n := len(pattern); pattern[n-1] == '/' {
				rule.dirOnly, pattern = true, pattern[:n-1]
			}
			if len(pattern) == 0 {
				return nil, errAttributeSyntax
			}
			rule.basename = bytes.IndexByte(pattern, '/') < 0
			rule.pattern = pattern
		}
		for _, token := range strings.Fields(string(line)) {
			assignment := attrAssignment{state: attrState{kind: 's'}}
			switch token[0] {
			case '-':
				assignment.state.kind, token = 'u', token[1:]
			case '!':
				assignment.state.kind, token = '!', token[1:]
			}
			if name, value, found := strings.Cut(token, "="); found && assignment.state.kind == 's' {
				token, assignment.state = name, attrState{kind: 'v', value: value}
			}
			if !validAttrName(token) {
				return nil, errAttributeSyntax
			}
			assignment.name = token
			rule.assigns = append(rule.assigns, assignment)
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

// validAttrName is Git's attr_name_valid: [-._0-9A-Za-z], not starting
// with '-'.
func validAttrName(name string) bool {
	if name == "" || name[0] == '-' {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !(c == '-' || c == '.' || c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return false
		}
	}
	return true
}

// unquoteCStyle decodes a C-style quoted string starting at line[0] and
// returns the decoded bytes and the rest of the line, like Git's
// unquote_c_style.
func unquoteCStyle(line []byte) ([]byte, []byte, error) {
	var out []byte
	for i := 1; i < len(line); i++ {
		c := line[i]
		switch c {
		case '"':
			return out, line[i+1:], nil
		case '\\':
			i++
			if i >= len(line) {
				return nil, nil, errAttributeSyntax
			}
			switch e := line[i]; e {
			case 'a':
				out = append(out, '\a')
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'v':
				out = append(out, '\v')
			case '\\', '"':
				out = append(out, e)
			case '0', '1', '2', '3':
				if i+2 >= len(line) || line[i+1] < '0' || line[i+1] > '7' || line[i+2] < '0' || line[i+2] > '7' {
					return nil, nil, errAttributeSyntax
				}
				out = append(out, (e-'0')<<6|(line[i+1]-'0')<<3|(line[i+2]-'0'))
				i += 2
			default:
				return nil, nil, errAttributeSyntax
			}
		default:
			out = append(out, c)
		}
	}
	return nil, nil, errAttributeSyntax
}

// attrList is the rules of one source; base is its directory relative to
// the worktree root.
type attrList struct {
	source int
	base   []byte
	rules  []attrRule
}

// AttributeInputs are the attribute sources of one worktree, lowest
// precedence first for the global files.
type AttributeInputs struct {
	Root []byte
	// GitDir holds info/attributes; empty without a repository.
	GitDir []byte
	// SystemFiles are the frozen toolchain's system attribute files.
	SystemFiles [][]byte
	// GlobalFile is core.attributesFile, expanded, or the default
	// $XDG_CONFIG_HOME/git/attributes; GlobalExplicit tells which.
	GlobalFile     []byte
	GlobalExplicit bool
	IgnoreCase     bool
}

// AttributeEvaluator evaluates gitattributes in process (F008-FR-029,
// F008-FR-175); it never runs a filter or reads Git's opinion.
type AttributeEvaluator struct {
	in       AttributeInputs
	budget   *Budget
	sources  []AttributeSource
	low      []attrList // system then global
	info     *attrList
	perDir   map[string]*attrList
	unusable bool
}

func NewAttributeEvaluator(in AttributeInputs, budget *Budget) *AttributeEvaluator {
	e := &AttributeEvaluator{in: in, budget: budget, perDir: make(map[string]*attrList)}
	for _, file := range in.SystemFiles {
		if list := e.load(AttributeSourceSystem, "system_attributes", file, nil, false); list != nil {
			e.low = append(e.low, *list)
		}
	}
	if len(in.GlobalFile) > 0 {
		kind := AttributeSourceGlobalDefault
		if in.GlobalExplicit {
			kind = AttributeSourceGlobalExplicit
		}
		if list := e.load(kind, string(kind), in.GlobalFile, nil, false); list != nil {
			e.low = append(e.low, *list)
		}
	}
	if len(in.GitDir) > 0 {
		e.info = e.load(AttributeSourceInfo, "info_attributes", []byte(string(in.GitDir)+"/info/attributes"), nil, false)
	}
	return e
}

func (e *AttributeEvaluator) load(kind AttributeSourceKind, scope string, path, base []byte, inTree bool) *attrList {
	var read sourceRead
	if inTree {
		read = readTreeSource(scope, path, uint64(len(e.sources)), e.budget)
	} else {
		read = readSource(scope, path, uint64(len(e.sources)), e.budget)
	}
	e.sources = append(e.sources, AttributeSource{Identity: read.identity, Kind: kind})
	if read.failed() {
		e.unusable = true
		return nil
	}
	// Macros are allowed in every source except a subdirectory one.
	rules, err := parseAttributes(read.data, !inTree || len(base) == 0)
	if err != nil {
		e.unusable = true
		e.sources[len(e.sources)-1].Identity.ReadState = SourceReadUnsupported
		return nil
	}
	return &attrList{source: len(e.sources) - 1, base: base, rules: rules}
}

func (e *AttributeEvaluator) dirList(dir []byte) *attrList {
	if list, ok := e.perDir[string(dir)]; ok {
		return list
	}
	path := string(e.in.Root) + "/.gitattributes"
	if len(dir) > 0 {
		path = string(e.in.Root) + "/" + string(dir) + "/.gitattributes"
	}
	list := e.load(AttributeSourceRepository, "repository_gitattributes", []byte(path), append([]byte(nil), dir...), true)
	e.perDir[string(dir)] = list
	return list
}

// lists returns the sources that apply to path, highest precedence first:
// info/attributes, the per-directory files from the deepest to the root,
// then global and system.
func (e *AttributeEvaluator) lists(path []byte) []*attrList {
	var lists []*attrList
	if e.info != nil {
		lists = append(lists, e.info)
	}
	for end := len(path) - 1; end >= 0; end-- {
		if path[end] == '/' {
			if list := e.dirList(path[:end]); list != nil {
				lists = append(lists, list)
			}
		}
	}
	if list := e.dirList(nil); list != nil {
		lists = append(lists, list)
	}
	for i := len(e.low) - 1; i >= 0; i-- {
		lists = append(lists, &e.low[i])
	}
	return lists
}

// macros returns the macro definitions visible to path: the highest
// precedence definition of each name wins, then the built-in ones.
func (e *AttributeEvaluator) macros(lists []*attrList) map[string][]attrAssignment {
	macros := make(map[string][]attrAssignment)
	for _, list := range lists {
		for i := len(list.rules) - 1; i >= 0; i-- {
			rule := list.rules[i]
			if _, defined := macros[rule.macro]; rule.macro != "" && !defined {
				macros[rule.macro] = rule.assigns
			}
		}
	}
	for name, assigns := range builtinMacros {
		if _, defined := macros[name]; !defined {
			macros[name] = assigns
		}
	}
	return macros
}

// Attributes returns the effective attributes of a file path relative to
// the root, the rules that assigned them, and whether every applicable
// source was read and understood.
func (e *AttributeEvaluator) Attributes(path []byte) (map[string]attrState, []string, bool) {
	lists := e.lists(path)
	macros := e.macros(lists)
	effective := make(map[string]attrState)
	var matched []string
	var assign func(assigns []attrAssignment, depth int)
	assign = func(assigns []attrAssignment, depth int) {
		for i := len(assigns) - 1; i >= 0; i-- {
			a := assigns[i]
			if _, done := effective[a.name]; done {
				continue
			}
			effective[a.name] = a.state
			if expansion, isMacro := macros[a.name]; isMacro && a.state.kind == 's' && depth < 16 {
				assign(expansion, depth+1)
			}
		}
	}
	for _, list := range lists {
		matcher := ignoreList{base: list.base}
		for i := len(list.rules) - 1; i >= 0; i-- {
			rule := list.rules[i]
			if rule.macro != "" {
				continue
			}
			if matcher.matches(ignoreRule{pattern: rule.pattern, dirOnly: rule.dirOnly, basename: rule.basename}, path, false, e.in.IgnoreCase) {
				matched = append(matched, rule.text)
				assign(rule.assigns, 0)
			}
		}
	}
	for name, state := range effective {
		if state.kind == '!' {
			delete(effective, name)
		}
	}
	return effective, matched, !e.unusable
}

func (e *AttributeEvaluator) Sources() []AttributeSource { return cloneSlice(e.sources) }
func (e *AttributeEvaluator) Usable() bool               { return !e.unusable }

// conversionRisk decides whether Git's clean conversion (convert_to_git)
// would change content added at path (F008-FR-030–032). content nil means
// the bytes are not known yet, so a possible conversion is unproven.
func conversionRisk(attrs map[string]attrState, config ConfigEvaluation, content []byte) ConversionRisk {
	if _, set := attrs["working-tree-encoding"]; set {
		return ConversionRiskMayTransform
	}
	risk := ConversionRiskNeutral
	if filter, set := attrs["filter"]; set && filter.kind == 'v' {
		driver := filter.value
		for _, command := range []string{"clean", "process"} {
			if value, _, found := config.Last("filter." + driver + "." + command); found && value != "" {
				return ConversionRiskExternalCode
			}
		}
		if value, hasValue, found := config.Last("filter." + driver + ".required"); found && (!hasValue || configTrue(value)) {
			return ConversionRiskIncompatible
		}
	}
	if ident, set := attrs["ident"]; set && ident.kind == 's' && (content == nil || bytes.Contains(content, []byte("$Id"))) {
		return ConversionRiskMayTransform
	}
	if crlfAction(attrs, config) != crlfBinary {
		switch {
		case content == nil:
			return ConversionRiskMayTransform
		case bytes.IndexByte(content, '\r') >= 0 && !(crlfAction(attrs, config) == crlfAuto && bytes.IndexByte(content, 0) >= 0):
			return ConversionRiskMayTransform
		}
	}
	return risk
}

const (
	crlfBinary = iota
	crlfText
	crlfAuto
)

// crlfAction follows convert.c: the text attribute (or the legacy crlf
// attribute) decides, an eol attribute implies text, and otherwise
// core.autocrlf true or input means auto.
func crlfAction(attrs map[string]attrState, config ConfigEvaluation) int {
	text, set := attrs["text"]
	if !set {
		if crlf, legacy := attrs["crlf"]; legacy {
			text, set = crlf, true
			if crlf.kind == 'v' && crlf.value == "input" {
				text = attrState{kind: 's'}
			}
		}
	}
	if set {
		switch {
		case text.kind == 'u':
			return crlfBinary
		case text.kind == 'v' && text.value == "auto":
			return crlfAuto
		default:
			// Set, or a value Git does not define: assume conversion.
			return crlfText
		}
	}
	if eol, set := attrs["eol"]; set && eol.kind == 'v' {
		return crlfText
	}
	if value, _, found := config.Last("core.autocrlf"); found && (configTrue(value) || strings.EqualFold(value, "input")) {
		return crlfAuto
	}
	return crlfBinary
}

// loadedLists returns every attribute source read so far.
func (e *AttributeEvaluator) loadedLists() []*attrList {
	var lists []*attrList
	for i := range e.low {
		lists = append(lists, &e.low[i])
	}
	if e.info != nil {
		lists = append(lists, e.info)
	}
	for _, list := range e.perDir {
		if list != nil {
			lists = append(lists, list)
		}
	}
	return lists
}
