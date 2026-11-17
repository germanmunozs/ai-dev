package gitmanager

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

// Canonical JSON is the schema-restricted RFC 8785 (JCS) form of the plan
// data model. A canonical value is one of: nil, bool, int64, string, []any,
// or map[string]any. Floating-point numbers do not exist in the model, and
// an integer outside the IEEE-754 safe range is never a JSON number
// (F008-FR-171): canonicalInteger turns it into a decimal string instead.

const maxSafeInteger = 1<<53 - 1

// maxCanonicalDepth is the plan nesting bound of F008-FR-051; T013
// centralizes the remaining input limits.
const maxCanonicalDepth = 32

var (
	errCanonicalSyntax = errors.New("invalid canonical JSON")
	errCanonicalLimit  = errors.New("canonical JSON limit exceeded")
	errNonCanonical    = errors.New("JSON is not in canonical form")
)

// MarshalCanonical writes v in RFC 8785 canonical form.
func MarshalCanonical(v any) ([]byte, error) {
	var b bytes.Buffer
	if err := writeCanonical(&b, v, 1); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func writeCanonical(b *bytes.Buffer, v any, depth int) error {
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		b.WriteString(strconv.FormatBool(x))
	case int64:
		if x > maxSafeInteger || x < -maxSafeInteger {
			return fmt.Errorf("%w: unsafe integer %d", errCanonicalSyntax, x)
		}
		b.WriteString(strconv.FormatInt(x, 10))
	case string:
		return writeCanonicalString(b, x)
	case []any:
		if depth > maxCanonicalDepth {
			return fmt.Errorf("%w: nesting", errCanonicalLimit)
		}
		b.WriteByte('[')
		for i, item := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeCanonical(b, item, depth+1); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case map[string]any:
		if depth > maxCanonicalDepth {
			return fmt.Errorf("%w: nesting", errCanonicalLimit)
		}
		keys := make([]string, 0, len(x))
		for key := range x {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool { return utf16Less(keys[i], keys[j]) })
		b.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeCanonicalString(b, key); err != nil {
				return err
			}
			b.WriteByte(':')
			if err := writeCanonical(b, x[key], depth+1); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	default:
		return fmt.Errorf("%w: unsupported value type %T", errCanonicalSyntax, v)
	}
	return nil
}

// writeCanonicalString applies the JCS string serialization: only the
// quotation mark, reverse solidus and C0 controls are escaped, with the
// short forms where JSON defines them and lowercase \u00xx otherwise.
func writeCanonicalString(b *bytes.Buffer, s string) error {
	if !utf8.ValidString(s) {
		return fmt.Errorf("%w: invalid UTF-8 string", errCanonicalSyntax)
	}
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return nil
}

// utf16Less orders property names by their UTF-16 code units, as RFC 8785
// requires; this differs from code point order above U+FFFF.
func utf16Less(a, b string) bool {
	x, y := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(x) && i < len(y); i++ {
		if x[i] != y[i] {
			return x[i] < y[i]
		}
	}
	return len(x) < len(y)
}

// canonicalInteger returns the F008-FR-171 representation of an unsigned
// model integer: a JSON number in the safe range, a decimal string beyond it.
func canonicalInteger(v uint64) any {
	if v <= maxSafeInteger {
		return int64(v)
	}
	return strconv.FormatUint(v, 10)
}

// DecodeLimits bounds a decode before content is retained.
type DecodeLimits struct {
	MaxBytes            int
	MaxDepth            int
	MaxContainerMembers int
	MaxStringBytes      int
}

// PlanDecodeLimits are the F008-FR-051 plan JSON bounds: 8 MiB, nesting 32,
// 16,384 members or items per container, and 1 MiB per decoded string.
var PlanDecodeLimits = DecodeLimits{MaxBytes: 8 << 20, MaxDepth: 32, MaxContainerMembers: 16384, MaxStringBytes: 1 << 20}

// UnmarshalCanonical strictly decodes data and requires it to be exactly
// the canonical serialization of the decoded value. Duplicate members,
// fractions, exponents, unsafe integers, invalid UTF-8, lone surrogates,
// and any non-canonical spelling are rejected.
func UnmarshalCanonical(data []byte, limits DecodeLimits) (any, error) {
	if len(data) > limits.MaxBytes {
		return nil, fmt.Errorf("%w: %d bytes", errCanonicalLimit, len(data))
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("%w: invalid UTF-8", errCanonicalSyntax)
	}
	p := canonicalParser{data: data, limits: limits}
	v, err := p.value(1)
	if err != nil {
		return nil, err
	}
	p.skipSpace()
	if p.pos != len(data) {
		return nil, p.fail("trailing data")
	}
	encoded, err := MarshalCanonical(v)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(encoded, data) {
		return nil, errNonCanonical
	}
	return v, nil
}

type canonicalParser struct {
	data   []byte
	pos    int
	limits DecodeLimits
}

func (p *canonicalParser) fail(what string) error {
	return fmt.Errorf("%w: %s at byte %d", errCanonicalSyntax, what, p.pos)
}

func (p *canonicalParser) skipSpace() {
	for p.pos < len(p.data) {
		switch p.data[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *canonicalParser) value(depth int) (any, error) {
	p.skipSpace()
	if p.pos >= len(p.data) {
		return nil, p.fail("unexpected end")
	}
	switch c := p.data[p.pos]; {
	case c == '{':
		return p.object(depth)
	case c == '[':
		return p.array(depth)
	case c == '"':
		return p.string()
	case c == '-' || (c >= '0' && c <= '9'):
		return p.number()
	default:
		for _, literal := range []struct {
			text  string
			value any
		}{{"true", true}, {"false", false}, {"null", nil}} {
			if bytes.HasPrefix(p.data[p.pos:], []byte(literal.text)) {
				p.pos += len(literal.text)
				return literal.value, nil
			}
		}
		return nil, p.fail("unexpected character")
	}
}

func (p *canonicalParser) enter(depth int) error {
	if depth > p.limits.MaxDepth {
		return fmt.Errorf("%w: nesting above %d", errCanonicalLimit, p.limits.MaxDepth)
	}
	p.pos++
	p.skipSpace()
	return nil
}

func (p *canonicalParser) object(depth int) (any, error) {
	if err := p.enter(depth); err != nil {
		return nil, err
	}
	members := make(map[string]any)
	if p.pos < len(p.data) && p.data[p.pos] == '}' {
		p.pos++
		return members, nil
	}
	for {
		p.skipSpace()
		if p.pos >= len(p.data) || p.data[p.pos] != '"' {
			return nil, p.fail("expected member name")
		}
		key, err := p.string()
		if err != nil {
			return nil, err
		}
		if _, duplicate := members[key]; duplicate {
			return nil, p.fail(fmt.Sprintf("duplicate member %q", key))
		}
		if len(members) == p.limits.MaxContainerMembers {
			return nil, fmt.Errorf("%w: more than %d members", errCanonicalLimit, p.limits.MaxContainerMembers)
		}
		p.skipSpace()
		if p.pos >= len(p.data) || p.data[p.pos] != ':' {
			return nil, p.fail("expected ':'")
		}
		p.pos++
		value, err := p.value(depth + 1)
		if err != nil {
			return nil, err
		}
		members[key] = value
		if done, err := p.separator('}'); err != nil || done {
			return members, err
		}
	}
}

func (p *canonicalParser) array(depth int) (any, error) {
	if err := p.enter(depth); err != nil {
		return nil, err
	}
	items := []any{}
	if p.pos < len(p.data) && p.data[p.pos] == ']' {
		p.pos++
		return items, nil
	}
	for {
		if len(items) == p.limits.MaxContainerMembers {
			return nil, fmt.Errorf("%w: more than %d items", errCanonicalLimit, p.limits.MaxContainerMembers)
		}
		value, err := p.value(depth + 1)
		if err != nil {
			return nil, err
		}
		items = append(items, value)
		if done, err := p.separator(']'); err != nil || done {
			return items, err
		}
	}
}

// separator consumes ',' (more to come) or the closing byte (done).
func (p *canonicalParser) separator(closing byte) (bool, error) {
	p.skipSpace()
	if p.pos >= len(p.data) {
		return false, p.fail("unexpected end")
	}
	switch p.data[p.pos] {
	case ',':
		p.pos++
		return false, nil
	case closing:
		p.pos++
		return true, nil
	default:
		return false, p.fail("expected separator")
	}
}

func (p *canonicalParser) number() (any, error) {
	start := p.pos
	if p.data[p.pos] == '-' {
		p.pos++
	}
	digits := p.pos
	for p.pos < len(p.data) && p.data[p.pos] >= '0' && p.data[p.pos] <= '9' {
		p.pos++
	}
	text := string(p.data[start:p.pos])
	switch {
	case p.pos == digits:
		return nil, p.fail("missing digits")
	case p.pos-digits > 1 && p.data[digits] == '0':
		return nil, p.fail("leading zero")
	case text == "-0":
		return nil, p.fail("negative zero")
	}
	if p.pos < len(p.data) && (p.data[p.pos] == '.' || p.data[p.pos] == 'e' || p.data[p.pos] == 'E') {
		return nil, p.fail("non-integer number")
	}
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil || n > maxSafeInteger || n < -maxSafeInteger {
		return nil, p.fail("unsafe integer")
	}
	return n, nil
}

func (p *canonicalParser) string() (string, error) {
	p.pos++ // opening quote
	var b []byte
	for {
		if len(b) > p.limits.MaxStringBytes {
			return "", fmt.Errorf("%w: string above %d bytes", errCanonicalLimit, p.limits.MaxStringBytes)
		}
		if p.pos >= len(p.data) {
			return "", p.fail("unterminated string")
		}
		c := p.data[p.pos]
		switch {
		case c == '"':
			p.pos++
			if len(b) > p.limits.MaxStringBytes {
				return "", fmt.Errorf("%w: string above %d bytes", errCanonicalLimit, p.limits.MaxStringBytes)
			}
			return string(b), nil
		case c < 0x20:
			return "", p.fail("raw control character")
		case c != '\\':
			b = append(b, c)
			p.pos++
			continue
		}
		p.pos++
		if p.pos >= len(p.data) {
			return "", p.fail("unterminated escape")
		}
		escape := p.data[p.pos]
		p.pos++
		switch escape {
		case '"', '\\', '/':
			b = append(b, escape)
		case 'b':
			b = append(b, '\b')
		case 'f':
			b = append(b, '\f')
		case 'n':
			b = append(b, '\n')
		case 'r':
			b = append(b, '\r')
		case 't':
			b = append(b, '\t')
		case 'u':
			r, err := p.unicodeEscape()
			if err != nil {
				return "", err
			}
			b = utf8.AppendRune(b, r)
		default:
			return "", p.fail("invalid escape")
		}
	}
}

// unicodeEscape decodes \uXXXX, joining a surrogate pair and rejecting a
// lone surrogate, which has no UTF-8 representation.
func (p *canonicalParser) unicodeEscape() (rune, error) {
	unit, err := p.hex4()
	if err != nil {
		return 0, err
	}
	if !utf16.IsSurrogate(unit) {
		return unit, nil
	}
	if unit >= 0xdc00 || !bytes.HasPrefix(p.data[p.pos:], []byte(`\u`)) {
		return 0, p.fail("lone surrogate")
	}
	p.pos += 2
	low, err := p.hex4()
	if err != nil {
		return 0, err
	}
	r := utf16.DecodeRune(unit, low)
	if r == utf8.RuneError {
		return 0, p.fail("invalid surrogate pair")
	}
	return r, nil
}

func (p *canonicalParser) hex4() (rune, error) {
	if p.pos+4 > len(p.data) {
		return 0, p.fail("short unicode escape")
	}
	n, err := strconv.ParseUint(string(p.data[p.pos:p.pos+4]), 16, 16)
	if err != nil {
		return 0, p.fail("invalid unicode escape")
	}
	p.pos += 4
	return rune(n), nil
}

// canonicalFields reads a decoded object against a closed member set: an
// unknown member or a missing required member is rejected. Schema decoders
// of untrusted plans (T055) build on it.
func canonicalFields(v any, required, optional []string) (map[string]any, error) {
	object, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: expected object", errCanonicalSyntax)
	}
	known := make(map[string]bool, len(required)+len(optional))
	for _, name := range required {
		if _, present := object[name]; !present {
			return nil, fmt.Errorf("%w: missing member %q", errCanonicalSyntax, name)
		}
		known[name] = true
	}
	for _, name := range optional {
		known[name] = true
	}
	for name := range object {
		if !known[name] {
			return nil, fmt.Errorf("%w: unknown member %q", errCanonicalSyntax, name)
		}
	}
	return object, nil
}

func canonicalSHA256(v any) (string, error) {
	data, err := MarshalCanonical(v)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data)), nil
}

// DeriveOperationID computes the F008-FR-173 identifier of an operation:
// its zero-based ordinal and the SHA-256 of the canonical JSON of its kind,
// exact argv, target identity, preconditions, expected effects, and
// partial-effect classification. For a Git invocation it also binds the
// environment profile name, the exact frozen environment path bytes
// (plan L-2), and the digest of the exact stdin bytes when Git reads stdin
// (O-T011-1). The OperationID field of in is ignored.
func DeriveOperationID(in DerivedOperationInput, environment EnvironmentProfile) (OperationID, error) {
	return deriveOperationID(in.Ordinal, in.Kind, in.ExactArgv, in.EnvironmentProfile, in.StdinDigest, in.TargetIdentity,
		in.Preconditions, in.ExpectedEffects, in.PartialEffectClassification, environment)
}

func deriveOperationID(ordinal uint64, kind OperationKind, argv []string, profile string, stdin ContentDigest, target string,
	preconditions, effects []Condition, partial PartialEffectClassification, environment EnvironmentProfile) (OperationID, error) {
	e := &canonicalEncoder{}
	content := map[string]any{
		"kind":                          string(kind),
		"exact_argv":                    canonicalStrings(argv),
		"target_identity":               target,
		"preconditions":                 canonicalList(preconditions, e.condition),
		"expected_effects":              canonicalList(effects, e.condition),
		"partial_effect_classification": string(partial),
	}
	if len(argv) > 0 {
		content["environment_profile"] = profile
		content["environment"] = e.environment(environment)
	}
	optional(content, "stdin_digest", string(stdin), stdin != "")
	if e.err != nil {
		return "", e.err
	}
	digest, err := canonicalSHA256(content)
	if err != nil {
		return "", err
	}
	return OperationID(strconv.FormatUint(ordinal, 10) + ":" + digest), nil
}

// ComputePlanDigest returns the plan_digest of the covered members of in
// (F008-FR-172). Plan metadata and in.PlanDigest itself never contribute.
func ComputePlanDigest(in GitInitializationPlanInput) (PlanDigest, error) {
	covered, err := coveredPlan(in.SchemaVersion, in.ImmutableIntent, in.PreconditionSnapshot, in.DerivedOperations,
		in.ExpectedPostconditions, in.ProhibitedOperations, in.PartialEffectPoints, in.RecoveryPolicy)
	if err != nil {
		return "", err
	}
	digest, err := canonicalSHA256(covered)
	return PlanDigest(digest), err
}

// CanonicalCoveredPlan returns the canonical bytes that plan_digest hashes.
func CanonicalCoveredPlan(plan GitInitializationPlan) ([]byte, error) {
	covered, err := coveredPlan(plan.SchemaVersion, plan.ImmutableIntent, plan.PreconditionSnapshot, plan.derivedOperations,
		plan.expectedPostconditions, plan.prohibitedOperations, plan.partialEffectPoints, plan.RecoveryPolicy)
	if err != nil {
		return nil, err
	}
	return MarshalCanonical(covered)
}

func coveredPlan(schema string, intent ImmutableIntent, snapshot PreconditionSnapshot, operations []DerivedOperation,
	postconditions []Condition, prohibited []ProhibitedOperation, points []OperationID, policy RecoveryPolicy) (any, error) {
	e := &canonicalEncoder{}
	covered := map[string]any{
		"schema_version":          schema,
		"immutable_intent":        e.intent(intent),
		"precondition_snapshot":   e.snapshot(snapshot),
		"derived_operations":      canonicalList(operations, e.operation),
		"expected_postconditions": canonicalList(postconditions, e.condition),
		"prohibited_operations":   canonicalList(prohibited, func(v ProhibitedOperation) any { return string(v) }),
		"partial_effect_points":   canonicalList(points, func(v OperationID) any { return string(v) }),
		"recovery_policy":         string(policy),
	}
	return covered, e.err
}
