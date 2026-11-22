package gitmanager

import (
	"bytes"
	"errors"
	"testing"
)

// FuzzPlanDecoder checks the strict decoder on arbitrary input: it never
// panics, it respects every bound, and whatever it accepts is already the
// exact canonical serialization of the decoded value. Schema-level decoding
// of a whole plan (unknown members, field types) builds on it in T055.
func FuzzPlanDecoder(f *testing.F) {
	plan, err := NewGitInitializationPlan(validPlanInput(f))
	if err == nil {
		if covered, err := CanonicalCoveredPlan(plan); err == nil {
			f.Add(covered)
		}
	}
	for _, seed := range []string{
		`{}`, `[]`, `null`, `0`, `-9007199254740991`, `"\u001f"`, `{"a":[true,false,null]}`,
		`{"a":1,"a":2}`, `{"b":1,"a":2}`, `{"a":1.5}`, `"\ud800"`, `"😀"`, "[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]",
	} {
		f.Add([]byte(seed))
	}
	small := DecodeLimits{MaxBytes: 256, MaxDepth: 4, MaxContainerMembers: 8, MaxStringBytes: 16}
	f.Fuzz(func(t *testing.T, data []byte) {
		for _, limits := range []DecodeLimits{PlanDecodeLimits, small} {
			value, err := UnmarshalCanonical(data, limits)
			if err != nil {
				if !errors.Is(err, errCanonicalSyntax) && !errors.Is(err, errCanonicalLimit) && !errors.Is(err, errNonCanonical) {
					t.Fatalf("unclassified decode error: %v", err)
				}
				continue
			}
			if len(data) > limits.MaxBytes {
				t.Fatalf("accepted %d bytes above the %d-byte bound", len(data), limits.MaxBytes)
			}
			encoded, err := MarshalCanonical(value)
			if err != nil || !bytes.Equal(encoded, data) {
				t.Fatalf("accepted input is not its own canonical form: %q -> %q, %v", data, encoded, err)
			}
		}
	})
}
