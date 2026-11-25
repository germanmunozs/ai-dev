package editor

import (
	"bytes"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

func b(parts ...any) []byte {
	var out []byte
	for _, p := range parts {
		switch v := p.(type) {
		case int:
			out = append(out, byte(v))
		case string:
			out = append(out, v...)
		case []byte:
			out = append(out, v...)
		}
	}
	return out
}

// The two requests ai-dev sends, byte for byte (research R-02).
func TestEncodeRequests(t *testing.T) {
	got, err := encode([]any{0, 1, "nvim_get_api_info", []any{}})
	want := b(0x94, 0x00, 0x01, 0xb1, "nvim_get_api_info", 0x90)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("api info request = % x, %v; want % x", got, err, want)
	}
	got, err = encode([]any{0, 2, "nvim_exec_lua", []any{"chunk", []any{"/p a", 12, 1}}})
	want = b(0x94, 0x00, 0x02, 0xad, "nvim_exec_lua", 0x92, 0xa5, "chunk", 0x93, 0xa4, "/p a", 0x0c, 0x01)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("exec_lua request = % x, %v; want % x", got, err, want)
	}
}

func TestEncodeWidths(t *testing.T) {
	cases := []struct {
		value any
		want  []byte
	}{
		{nil, b(0xc0)},
		{true, b(0xc3)},
		{false, b(0xc2)},
		{127, b(0x7f)},
		{200, b(0xcc, 0xc8)},
		{300, b(0xcd, 0x01, 0x2c)},
		{70000, b(0xce, 0x00, 0x01, 0x11, 0x70)},
		{int64(1) << 40, b(0xcf, 0, 0, 0x01, 0, 0, 0, 0, 0)},
		{-1, b(0xff)},
		{-32, b(0xe0)},
		{-33, b(0xd0, 0xdf)},
		{-200, b(0xd1, 0xff, 0x38)},
		{-70000, b(0xd2, 0xff, 0xfe, 0xee, 0x90)},
		{-(int64(1) << 40), b(0xd3, 0xff, 0xff, 0xff, 0, 0, 0, 0, 0)},
		{strings.Repeat("x", 31), b(0xbf, strings.Repeat("x", 31))},
		{strings.Repeat("x", 40), b(0xd9, 40, strings.Repeat("x", 40))},
		{strings.Repeat("x", 300), b(0xda, 0x01, 0x2c, strings.Repeat("x", 300))},
		{strings.Repeat("x", 70000), b(0xdb, 0, 0x01, 0x11, 0x70, strings.Repeat("x", 70000))},
		{make([]any, 20), append(b(0xdc, 0, 20), bytes.Repeat([]byte{0xc0}, 20)...)},
		{make([]any, 70000), append(b(0xdd, 0, 0x01, 0x11, 0x70), bytes.Repeat([]byte{0xc0}, 70000)...)},
	}
	for _, c := range cases {
		got, err := encode(c.value)
		if err != nil || !bytes.Equal(got, c.want) {
			t.Errorf("encode(%T %.20v) = % .12x, %v", c.value, c.value, got, err)
		}
	}
	if _, err := encode(3.5); err == nil {
		t.Error("encode accepted a type ai-dev never sends")
	}
}

func TestDecodeTypes(t *testing.T) {
	cases := []struct {
		in   []byte
		want any
	}{
		{b(0xc0), nil},
		{b(0xc3), true},
		{b(0xc2), false},
		{b(0x05), int64(5)},
		{b(0xe0), int64(-32)},
		{b(0xcc, 0xc8), int64(200)},
		{b(0xcd, 0x01, 0x2c), int64(300)},
		{b(0xce, 0, 0x01, 0x11, 0x70), int64(70000)},
		{b(0xcf, 0, 0, 0, 0, 0, 0, 0x01, 0), int64(256)},
		{b(0xcf, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff), uint64(math.MaxUint64)},
		{b(0xd0, 0xdf), int64(-33)},
		{b(0xd1, 0xff, 0x38), int64(-200)},
		{b(0xd2, 0xff, 0xfe, 0xee, 0x90), int64(-70000)},
		{b(0xd3, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff), int64(-1)},
		{b(0xca, 0x3f, 0xc0, 0, 0), float64(1.5)},
		{b(0xcb, 0x3f, 0xf8, 0, 0, 0, 0, 0, 0), float64(1.5)},
		{b(0xa2, "hi"), "hi"},
		{b(0xd9, 2, "hi"), "hi"},
		{b(0xda, 0, 2, "hi"), "hi"},
		{b(0xdb, 0, 0, 0, 2, "hi"), "hi"},
		{b(0xc4, 2, 1, 2), []byte{1, 2}},
		{b(0xc5, 0, 1, 9), []byte{9}},
		{b(0xc6, 0, 0, 0, 1, 9), []byte{9}},
		{b(0x92, 0x01, 0xa1, "a"), []any{int64(1), "a"}},
		{b(0xdc, 0, 1, 0xc0), []any{nil}},
		{b(0xdd, 0, 0, 0, 1, 0xc3), []any{true}},
		{b(0x82, 0xa1, "a", 0x01, 0x07, 0x02), map[string]any{"a": int64(1), "7": int64(2)}},
		{b(0xde, 0, 1, 0xa1, "k", 0xc0), map[string]any{"k": nil}},
		{b(0xdf, 0, 0, 0, 1, 0xa1, "k", 0xc3), map[string]any{"k": true}},
		{b(0xd4, 1, 7), Ext{Type: 1, Data: []byte{7}}},
		{b(0xd5, 2, 7, 8), Ext{Type: 2, Data: []byte{7, 8}}},
		{b(0xd6, 0, 1, 2, 3, 4), Ext{Type: 0, Data: []byte{1, 2, 3, 4}}},
		{b(0xd7, 0, 1, 2, 3, 4, 5, 6, 7, 8), Ext{Type: 0, Data: []byte{1, 2, 3, 4, 5, 6, 7, 8}}},
		{b(0xd8, 0, bytes.Repeat([]byte{1}, 16)), Ext{Type: 0, Data: bytes.Repeat([]byte{1}, 16)}},
		{b(0xc7, 2, 3, 1, 2), Ext{Type: 3, Data: []byte{1, 2}}},
		{b(0xc8, 0, 1, 3, 9), Ext{Type: 3, Data: []byte{9}}},
		{b(0xc9, 0, 0, 0, 1, 3, 9), Ext{Type: 3, Data: []byte{9}}},
	}
	for _, c := range cases {
		got, err := decode(bytes.NewReader(c.in), maxMessage)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("decode(% x) = %#v, %v; want %#v", c.in, got, err, c.want)
		}
	}
}

func TestDecodeRejects(t *testing.T) {
	deep := append(bytes.Repeat([]byte{0x91}, maxDepth+1), 0xc0)
	cases := map[string][]byte{
		"empty":            {},
		"truncated string": b(0xa5, "ab"),
		"truncated array":  b(0x92, 0x01),
		"truncated int":    b(0xcd, 0x01),
		"string too long":  b(0xdb, 0x7f, 0xff, 0xff, 0xff),
		"array too long":   b(0xdd, 0x7f, 0xff, 0xff, 0xff),
		"map too long":     b(0xdf, 0x7f, 0xff, 0xff, 0xff),
		"too deep":         deep,
		"array map key":    b(0x81, 0x90, 0xc0),
		"nil map key":      b(0x81, 0xc0, 0xc0),
		"reserved byte":    b(0xc1),
	}
	for name, in := range cases {
		if _, err := decode(bytes.NewReader(in), maxMessage); err == nil {
			t.Errorf("%s: decode(% .16x) accepted", name, in)
		}
	}
	// The limit counts every byte read, so a message larger than it is
	// refused without reading the rest.
	big := b(0xdb, 0x00, 0x20, 0x00, 0x00) // a 2 MiB string header
	if _, err := decode(bytes.NewReader(big), maxMessage); !errors.Is(err, errTooLarge) {
		t.Errorf("a 2 MiB string: %v, want errTooLarge", err)
	}
	small := b(0xa5, "hello")
	if _, err := decode(bytes.NewReader(small), 3); !errors.Is(err, errTooLarge) {
		t.Errorf("a message above a 3-byte limit: %v", err)
	}
}

func FuzzDecode(f *testing.F) {
	for _, seed := range [][]byte{
		b(0x94, 0x01, 0x01, 0xc0, 0x92, 0x01, 0x81, 0xa7, "version", 0x83, 0xa5, "major", 0x00, 0xa5, "minor", 0x0c, 0xa5, "patch", 0x06),
		b(0x94, 0x01, 0x02, 0x92, 0x00, 0xa4, "E499", 0xc0),
		b(0xd4, 1, 7), b(0xdf, 0, 0, 0, 1, 0xa1, "k", 0xc3), b(0xcb, 0x3f, 0xf8, 0, 0, 0, 0, 0, 0),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		v, err := decode(bytes.NewReader(data), 4096)
		if err == nil && v == nil && (len(data) == 0 || data[0] != 0xc0) {
			t.Fatalf("decode(% x) = nil without an error", data)
		}
	})
}
