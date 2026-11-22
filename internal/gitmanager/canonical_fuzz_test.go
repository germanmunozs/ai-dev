package gitmanager

import (
	"bytes"
	"errors"
	"testing"
	"unicode/utf8"
)

// FuzzCanonicalWriter checks that the writer is deterministic, that its
// output is accepted unchanged by the strict decoder, and that the digest
// of a value is stable across a decode/encode round trip.
func FuzzCanonicalWriter(f *testing.F) {
	f.Add("key", "value", int64(1), true)
	f.Add("\r", "€$\x0f\nA'B\"\\", int64(maxSafeInteger), false)
	f.Add("\U0001F600", "café", int64(-maxSafeInteger), true)
	f.Add("דּ", "", int64(maxSafeInteger+1), false)
	f.Add("bad\xff", "x", int64(0), false)
	f.Fuzz(func(t *testing.T, key, text string, number int64, flag bool) {
		value := map[string]any{
			key:    []any{text, number, flag, nil, map[string]any{text: key}},
			"list": []any{},
		}
		first, err := MarshalCanonical(value)
		safe := number <= maxSafeInteger && number >= -maxSafeInteger
		if !utf8.ValidString(key) || !utf8.ValidString(text) || !safe {
			if !errors.Is(err, errCanonicalSyntax) {
				t.Fatalf("unrepresentable value encoded: %v", err)
			}
			return
		}
		if err != nil {
			t.Fatalf("representable value rejected: %v", err)
		}
		second, err := MarshalCanonical(value)
		if err != nil || !bytes.Equal(first, second) {
			t.Fatalf("writer is not deterministic: %q vs %q", first, second)
		}
		decoded, err := UnmarshalCanonical(first, PlanDecodeLimits)
		if err != nil {
			t.Fatalf("writer output rejected by the decoder: %v (%q)", err, first)
		}
		again, err := MarshalCanonical(decoded)
		if err != nil || !bytes.Equal(again, first) {
			t.Fatalf("round trip changed bytes: %q -> %q", first, again)
		}
		before, _ := canonicalSHA256(value)
		after, _ := canonicalSHA256(decoded)
		if before != after {
			t.Fatal("digest changed across a round trip")
		}
	})
}
