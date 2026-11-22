package gitmanager

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"strings"
	"unicode/utf8"
)

// configIncludeDepth is Git's own maximum include depth: Git itself fails
// beyond it, so a deeper chain cannot describe an ordinary Git result.
const configIncludeDepth = 10

// configLine is one parsed variable. key is canonical: lowercase section
// and name, exact subsection.
type configLine struct {
	key      string
	value    string
	hasValue bool
}

var errConfigSyntax = errors.New("invalid Git configuration syntax")

// configReader yields bytes with Git's get_next_char rule: CRLF reads as a
// single newline, and the end of input reads as a newline.
type configReader struct {
	data []byte
	pos  int
}

func (r *configReader) eof() bool { return r.pos >= len(r.data) }

func (r *configReader) next() byte {
	if r.eof() {
		r.pos++
		return '\n'
	}
	c := r.data[r.pos]
	r.pos++
	if c == '\r' && r.pos < len(r.data) && r.data[r.pos] == '\n' {
		r.pos++
		return '\n'
	}
	return c
}

func configSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

func configKeyChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-'
}

// parseConfig parses one configuration file with the grammar of Git's
// config.c: sections with optional quoted subsections or the deprecated
// dotted form, case-insensitive section and variable names, implicit
// boolean variables, quoted and escaped values, line continuation, and
// '#'/';' comments. Anything Git would reject is an error, and so is a
// variable outside any section, which Git tolerates but which has no
// canonical key.
func parseConfig(data []byte) ([]configLine, error) {
	r := &configReader{data: bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))}
	var lines []configLine
	section := ""
	fail := func(what string) ([]configLine, error) {
		return nil, fmt.Errorf("%w: %s at byte %d", errConfigSyntax, what, r.pos)
	}
	for !r.eof() {
		c := r.next()
		switch {
		case configSpace(c):
		case c == '#' || c == ';':
			for !r.eof() && r.next() != '\n' {
			}
		case c == '[':
			name, err := parseConfigSection(r)
			if err != nil {
				return fail(err.Error())
			}
			section = name
		case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			if section == "" {
				return fail("variable outside a section")
			}
			name := []byte{lowerASCII(c)}
			for !r.eof() && configKeyChar(r.data[r.pos]) {
				name = append(name, lowerASCII(r.next()))
			}
			c = r.next()
			for c == ' ' || c == '\t' {
				c = r.next()
			}
			line := configLine{key: section + "." + string(name)}
			if c != '\n' {
				if c != '=' {
					return fail("variable without '='")
				}
				value, err := parseConfigValue(r)
				if err != nil {
					return fail(err.Error())
				}
				line.value, line.hasValue = value, true
			}
			lines = append(lines, line)
		default:
			return fail("unexpected character")
		}
	}
	return lines, nil
}

func parseConfigSection(r *configReader) (string, error) {
	var name []byte
	for {
		if r.eof() {
			return "", errors.New("unterminated section header")
		}
		c := r.next()
		if c == ']' {
			if len(name) == 0 {
				return "", errors.New("empty section name")
			}
			return string(name), nil
		}
		if configSpace(c) {
			break
		}
		if !configKeyChar(c) && c != '.' {
			return "", errors.New("invalid section name")
		}
		name = append(name, lowerASCII(c))
	}
	if len(name) == 0 || bytes.IndexByte(name, '.') >= 0 {
		return "", errors.New("invalid extended section name")
	}
	c := r.next()
	for c == ' ' || c == '\t' {
		c = r.next()
	}
	if c != '"' {
		return "", errors.New("expected quoted subsection")
	}
	var subsection []byte
	for {
		c = r.next()
		if c == '\n' {
			return "", errors.New("newline in subsection")
		}
		if c == '"' {
			break
		}
		if c == '\\' {
			if c = r.next(); c == '\n' {
				return "", errors.New("newline in subsection")
			}
		}
		subsection = append(subsection, c)
	}
	if r.next() != ']' {
		return "", errors.New("junk after subsection")
	}
	return string(name) + "." + string(subsection), nil
}

// parseConfigValue follows Git's parse_value as of the supported
// toolchains (verified against Git 2.54): outside quotes leading and
// trailing whitespace is dropped and internal whitespace is kept verbatim;
// comments end the value; only \n \t \b \\ \" and line continuation are
// valid escapes.
func parseConfigValue(r *configReader) (string, error) {
	var value, space []byte
	quote, comment := false, false
	for {
		c := r.next()
		if c == '\n' {
			if quote {
				return "", errors.New("unterminated quote")
			}
			return string(value), nil
		}
		if comment {
			continue
		}
		if configSpace(c) && !quote {
			if len(value) > 0 {
				space = append(space, c)
			}
			continue
		}
		if !quote && (c == ';' || c == '#') {
			comment = true
			continue
		}
		value, space = append(value, space...), space[:0]
		switch c {
		case '\\':
			switch c = r.next(); c {
			case '\n':
				continue
			case 't':
				c = '\t'
			case 'b':
				c = '\b'
			case 'n':
				c = '\n'
			case '\\', '"':
			default:
				return "", errors.New("invalid escape")
			}
			value = append(value, c)
		case '"':
			quote = !quote
		default:
			value = append(value, c)
		}
	}
}

// ConfigInputs is the real user context the in-process configuration
// reader evaluates: it never consults Git and never runs anything.
type ConfigInputs struct {
	// Home and XDGConfigHome are the user's real HOME and XDG_CONFIG_HOME.
	Home          []byte
	XDGConfigHome []byte
	// ToolchainSources are the frozen toolchain's own configuration files,
	// lowest precedence first.
	ToolchainSources [][]byte
	// GitDir is the repository's Git directory for includeIf gitdir, and
	// CommonDir holds its local configuration; both are empty when the
	// project has no repository yet.
	GitDir    []byte
	CommonDir []byte
	// Branch is HEAD's branch name for includeIf onbranch; nil when it is
	// not known.
	Branch []byte
	// EnvironmentOverrides reports GIT_CONFIG* variables in the real
	// environment, which change what ordinary Git reads.
	EnvironmentOverrides bool
	// Budget is the aggregate source budget shared with the attribute and
	// ignore evaluators; nil gives the evaluation its own.
	Budget *Budget
}

// ConfigEvaluation is the effective configuration in precedence order.
// Entries and Sources are redacted and safe to record; raw values stay in
// memory for evaluation only.
type ConfigEvaluation struct {
	State             ObservationState
	Sources           []InspectedSourceIdentity
	Entries           []ConfigEntry
	Incompatibilities []Incompatibility
	values            []configRecord
}

type configRecord struct {
	line   configLine
	scope  ConfigScope
	source InspectedSourceIdentity
}

// Last returns the highest-precedence value of key.
func (v ConfigEvaluation) Last(key string) (value string, hasValue, found bool) {
	for i := len(v.values) - 1; i >= 0; i-- {
		if v.values[i].line.key == key {
			return v.values[i].line.value, v.values[i].line.hasValue, true
		}
	}
	return "", false, false
}

// configEvaluator accumulates one evaluation.
type configEvaluator struct {
	in        ConfigInputs
	budget    *Budget
	out       ConfigEvaluation
	stack     []string
	unhealthy bool
}

func (e *configEvaluator) incompatible(reason Reason, subject string) {
	e.out.Incompatibilities = append(e.out.Incompatibilities, Incompatibility{Reason: reason, Subject: subject})
	if reason == ReasonAttributesIncompatible {
		e.unhealthy = true
	}
}

// EvaluateConfig reads every applicable configuration source in Git's
// precedence order (toolchain, XDG, global, local), resolving includes in
// place, and records each source and entry. Anything it cannot evaluate
// exactly is an attributes_incompatible incompatibility and makes the
// state error, never a silently partial result.
func EvaluateConfig(in ConfigInputs) ConfigEvaluation {
	budget := in.Budget
	if budget == nil {
		budget, _ = NewBudget(LimitAggregateSourceBytes)
	}
	e := &configEvaluator{in: in, budget: budget}
	if in.EnvironmentOverrides {
		e.incompatible(ReasonAttributesIncompatible, "GIT_CONFIG environment overrides are present")
	}
	for _, source := range in.ToolchainSources {
		e.readFile(ConfigScopeSystem, "system", source, 0)
	}
	if len(in.Home) == 0 || in.Home[0] != '/' {
		e.incompatible(ReasonAttributesIncompatible, "HOME is not an absolute path")
	} else {
		xdg := string(in.XDGConfigHome)
		if xdg == "" {
			xdg = string(in.Home) + "/.config"
		}
		if !strings.HasPrefix(xdg, "/") {
			e.incompatible(ReasonAttributesIncompatible, "XDG_CONFIG_HOME is not an absolute path")
		} else {
			e.readFile(ConfigScopeXDG, "xdg", []byte(xdg+"/git/config"), 0)
		}
		e.readFile(ConfigScopeGlobal, "global", []byte(string(in.Home)+"/.gitconfig"), 0)
	}
	if len(in.CommonDir) > 0 {
		e.readFile(ConfigScopeLocal, "local", []byte(string(in.CommonDir)+"/config"), 0)
		if value, _, found := e.out.Last("extensions.worktreeconfig"); found && configTrue(value) {
			e.incompatible(ReasonWorktreeConfigUnsupported, "extensions.worktreeConfig is enabled")
		}
	}
	switch {
	case e.unhealthy:
		e.out.State = ObservationError
	case len(e.out.Entries) == 0:
		e.out.State = ObservationObservedEmpty
	default:
		e.out.State = ObservationObserved
	}
	if e.out.State != ObservationObserved {
		// Only an observed group carries entries.
		e.out.Entries = nil
	}
	return e.out
}

func configTrue(value string) bool {
	switch strings.ToLower(value) {
	case "true", "yes", "on", "1":
		return true
	}
	return false
}

// readFile reads one source and its includes; depth counts include levels.
func (e *configEvaluator) readFile(scope ConfigScope, label string, file []byte, depth int) {
	name := string(file)
	for _, open := range e.stack {
		if open == name {
			e.incompatible(ReasonAttributesIncompatible, "configuration include cycle")
			return
		}
	}
	if depth > configIncludeDepth {
		e.incompatible(ReasonAttributesIncompatible, "configuration include depth exceeds Git's limit")
		return
	}
	source := readSource(label, file, uint64(len(e.out.Sources)), e.budget)
	e.out.Sources = append(e.out.Sources, source.identity)
	if source.failed() {
		e.incompatible(ReasonAttributesIncompatible, label+" configuration source cannot be read safely")
		return
	}
	if source.identity.ReadState != SourceReadRead {
		return
	}
	lines, err := parseConfig(source.data)
	if err != nil {
		e.incompatible(ReasonAttributesIncompatible, label+" configuration source has unsupported syntax")
		return
	}
	e.stack = append(e.stack, name)
	defer func() { e.stack = e.stack[:len(e.stack)-1] }()
	for _, line := range lines {
		e.record(line, scope, source.identity)
		include, ok := e.includeTarget(line, file)
		if !ok {
			continue
		}
		if include != nil {
			e.readFile(scope, label, include, depth+1)
		}
	}
}

// record appends one entry in redacted form.
func (e *configEvaluator) record(line configLine, scope ConfigScope, source InspectedSourceIdentity) {
	e.out.values = append(e.out.values, configRecord{line: line, scope: scope, source: source})
	key, value, state := redactConfigEntry(line)
	entry := ConfigEntry{
		Ordinal: uint64(len(e.out.Entries)), Key: key, ValuePresent: line.hasValue, ValueRedacted: value,
		ValueRedaction: state, Scope: scope, Origin: source.Path, SourceDigest: source.ContentDigest,
	}
	if !line.hasValue {
		entry.ValueRedacted, entry.ValueRedaction = "", RedactionNotNeeded
	}
	if entry.Validate() != nil {
		e.incompatible(ReasonAttributesIncompatible, "configuration entry cannot be represented")
		return
	}
	e.out.Entries = append(e.out.Entries, entry)
}

// includeTarget returns the file an include entry names, nil when its
// condition is false, and ok=false when line is not an include.
func (e *configEvaluator) includeTarget(line configLine, from []byte) ([]byte, bool) {
	var condition string
	switch {
	case line.key == "include.path":
	case strings.HasPrefix(line.key, "includeif.") && strings.HasSuffix(line.key, ".path"):
		condition = strings.TrimSuffix(strings.TrimPrefix(line.key, "includeif."), ".path")
		if condition == "" {
			return nil, false
		}
	default:
		return nil, false
	}
	if !line.hasValue || line.value == "" {
		e.incompatible(ReasonAttributesIncompatible, "configuration include without a path")
		return nil, true
	}
	if condition != "" {
		matched, supported := e.includeCondition(condition, from)
		if !supported {
			e.incompatible(ReasonAttributesIncompatible, "unsupported configuration include condition")
			return nil, true
		}
		if !matched {
			return nil, true
		}
	}
	target, ok := e.expandIncludePath(line.value, from)
	if !ok {
		e.incompatible(ReasonAttributesIncompatible, "unsupported configuration include path")
		return nil, true
	}
	return target, true
}

func (e *configEvaluator) expandIncludePath(value string, from []byte) ([]byte, bool) {
	switch {
	case strings.HasPrefix(value, "~/"):
		return []byte(string(e.in.Home) + value[1:]), len(e.in.Home) > 0
	case strings.HasPrefix(value, "~"):
		return nil, false
	case strings.HasPrefix(value, "/"):
		return []byte(value), true
	default:
		return []byte(path.Dir(string(from)) + "/" + value), true
	}
}

// includeCondition evaluates gitdir, gitdir/i and onbranch with Git's
// pattern rules; supported is false for any other condition or for
// onbranch when the branch is unknown.
func (e *configEvaluator) includeCondition(condition string, from []byte) (matched, supported bool) {
	switch {
	case strings.HasPrefix(condition, "gitdir:"), strings.HasPrefix(condition, "gitdir/i:"):
		flags := wildmatchPathname
		pattern := strings.TrimPrefix(condition, "gitdir:")
		if strings.HasPrefix(condition, "gitdir/i:") {
			flags |= wildmatchCasefold
			pattern = strings.TrimPrefix(condition, "gitdir/i:")
		}
		if len(e.in.GitDir) == 0 {
			return false, true
		}
		switch {
		case strings.HasPrefix(pattern, "~/"):
			pattern = string(e.in.Home) + pattern[1:]
		case strings.HasPrefix(pattern, "./"):
			pattern = path.Dir(string(from)) + pattern[1:]
		case !strings.HasPrefix(pattern, "/") && !strings.HasPrefix(pattern, "**/"):
			pattern = "**/" + pattern
		}
		if strings.HasSuffix(pattern, "/") {
			pattern += "**"
		}
		return wildmatch([]byte(pattern), e.in.GitDir, flags), true
	case strings.HasPrefix(condition, "onbranch:"):
		if len(e.in.GitDir) == 0 {
			return false, true
		}
		if e.in.Branch == nil {
			return false, false
		}
		pattern := strings.TrimPrefix(condition, "onbranch:")
		if strings.HasSuffix(pattern, "/") {
			pattern += "**"
		}
		return wildmatch([]byte(pattern), e.in.Branch, wildmatchPathname), true
	default:
		return false, false
	}
}

// redactConfigEntry removes credentials from a key's subsection and from
// its value (F008-FR-027). Credential sections and header or secret-named
// variables become opaque; URL-valued variables lose userinfo, query and
// fragment; any value that is not valid UTF-8 is opaque.
func redactConfigEntry(line configLine) (string, string, RedactionState) {
	key := line.key
	first, last := strings.IndexByte(key, '.'), strings.LastIndexByte(key, '.')
	section, name := key[:first], key[last+1:]
	subsection := ""
	if first != last {
		subsection = key[first+1 : last]
	}
	if subsection != "" && strings.Contains(subsection, "://") || strings.Contains(subsection, "@") {
		if display, state, _ := redactURL(subsection); state != RedactionNotNeeded {
			subsection = display
			key = section + "." + subsection + "." + name
		}
	}
	value := line.value
	if !utf8.ValidString(value) {
		return key, OpaqueMarker, RedactionOpaque
	}
	lower := strings.ToLower(name)
	if section == "credential" || strings.Contains(lower, "password") || strings.Contains(lower, "token") ||
		strings.Contains(lower, "secret") || strings.Contains(lower, "extraheader") || strings.Contains(lower, "cookie") {
		return key, OpaqueMarker, RedactionOpaque
	}
	urlValued := name == "url" || name == "pushurl" || name == "insteadof" || name == "pushinsteadof" || name == "proxy"
	if urlValued || strings.Contains(value, "://") {
		display, state, _ := redactURL(value)
		return key, display, state
	}
	return key, value, RedactionNotNeeded
}

// redactURL removes userinfo, query and fragment from a URL or an scp-like
// address. A value it cannot parse safely becomes the opaque marker.
// credential reports userinfo of a URL, or a query, that may hold secret
// material.
func redactURL(raw string) (display string, state RedactionState, credential bool) {
	for _, c := range raw {
		if c <= ' ' || c == 0x7f {
			return OpaqueMarker, RedactionOpaque, false
		}
	}
	if raw == "" {
		return OpaqueMarker, RedactionOpaque, false
	}
	if scheme, rest, ok := strings.Cut(raw, "://"); ok {
		if scheme == "" {
			return OpaqueMarker, RedactionOpaque, false
		}
		authority, tail := rest, ""
		if i := strings.IndexAny(rest, "/?#"); i >= 0 {
			authority, tail = rest[:i], rest[i:]
		}
		redacted := false
		if at := strings.LastIndexByte(authority, '@'); at >= 0 {
			authority, redacted, credential = "<redacted>@"+authority[at+1:], true, true
		}
		if i := strings.IndexAny(tail, "?#"); i >= 0 {
			tail, redacted, credential = tail[:i]+"?<redacted>", true, true
		}
		if redacted {
			return scheme + "://" + authority + tail, RedactionRedacted, credential
		}
		return raw, RedactionNotNeeded, false
	}
	// scp-like user@host:path; a local path has no colon before a slash.
	colon, slash := strings.IndexByte(raw, ':'), strings.IndexByte(raw, '/')
	if colon > 0 && (slash < 0 || colon < slash) {
		if at := strings.LastIndexByte(raw[:colon], '@'); at >= 0 {
			return "<redacted>@" + raw[at+1:], RedactionRedacted, false
		}
	}
	return raw, RedactionNotNeeded, false
}
