package editor

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"slices"
	"strconv"
)

// The msgpack subset of research R-02: requests carry only nil, booleans,
// integers, strings and arrays; responses may carry every type Neovim
// sends. Decoding is bounded in bytes and depth, so a hostile peer cannot
// make ai-dev allocate or recurse without limit.

const (
	maxMessage = 1 << 20 // bytes of one response
	maxDepth   = 32
)

var errTooLarge = errors.New("msgpack: message above the size limit")

// Ext is an extension value, such as a Neovim buffer or window handle,
// kept opaque.
type Ext struct {
	Type int8
	Data []byte
}

func encode(v any) ([]byte, error) {
	var out []byte
	err := appendValue(&out, v)
	return out, err
}

func appendValue(out *[]byte, v any) error {
	switch v := v.(type) {
	case nil:
		*out = append(*out, 0xc0)
	case bool:
		if v {
			*out = append(*out, 0xc3)
		} else {
			*out = append(*out, 0xc2)
		}
	case int:
		appendInt(out, int64(v))
	case int64:
		appendInt(out, v)
	case string:
		appendString(out, v)
	case map[string]any:
		// Only the tests' fake Neovim sends maps; keys are sorted so the
		// encoding is stable.
		appendMapHeader(out, len(v))
		for _, key := range slices.Sorted(maps.Keys(v)) {
			appendString(out, key)
			if err := appendValue(out, v[key]); err != nil {
				return err
			}
		}
	case []any:
		appendArrayHeader(out, len(v))
		for _, item := range v {
			if err := appendValue(out, item); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("msgpack: cannot encode %T", v)
	}
	return nil
}

func appendInt(out *[]byte, n int64) {
	switch {
	case n >= 0 && n <= 0x7f:
		*out = append(*out, byte(n))
	case n >= -32 && n < 0:
		*out = append(*out, byte(n))
	case n >= 0 && n <= math.MaxUint8:
		*out = append(*out, 0xcc, byte(n))
	case n >= 0 && n <= math.MaxUint16:
		*out = binary.BigEndian.AppendUint16(append(*out, 0xcd), uint16(n))
	case n >= 0 && n <= math.MaxUint32:
		*out = binary.BigEndian.AppendUint32(append(*out, 0xce), uint32(n))
	case n >= 0:
		*out = binary.BigEndian.AppendUint64(append(*out, 0xcf), uint64(n))
	case n >= math.MinInt8:
		*out = append(*out, 0xd0, byte(n))
	case n >= math.MinInt16:
		*out = binary.BigEndian.AppendUint16(append(*out, 0xd1), uint16(n))
	case n >= math.MinInt32:
		*out = binary.BigEndian.AppendUint32(append(*out, 0xd2), uint32(n))
	default:
		*out = binary.BigEndian.AppendUint64(append(*out, 0xd3), uint64(n))
	}
}

func appendString(out *[]byte, s string) {
	switch n := len(s); {
	case n <= 31:
		*out = append(*out, 0xa0|byte(n))
	case n <= math.MaxUint8:
		*out = append(*out, 0xd9, byte(n))
	case n <= math.MaxUint16:
		*out = binary.BigEndian.AppendUint16(append(*out, 0xda), uint16(n))
	default:
		*out = binary.BigEndian.AppendUint32(append(*out, 0xdb), uint32(n))
	}
	*out = append(*out, s...)
}

func appendArrayHeader(out *[]byte, n int) {
	switch {
	case n <= 15:
		*out = append(*out, 0x90|byte(n))
	case n <= math.MaxUint16:
		*out = binary.BigEndian.AppendUint16(append(*out, 0xdc), uint16(n))
	default:
		*out = binary.BigEndian.AppendUint32(append(*out, 0xdd), uint32(n))
	}
}

func appendMapHeader(out *[]byte, n int) {
	switch {
	case n <= 15:
		*out = append(*out, 0x80|byte(n))
	case n <= math.MaxUint16:
		*out = binary.BigEndian.AppendUint16(append(*out, 0xde), uint16(n))
	default:
		*out = binary.BigEndian.AppendUint32(append(*out, 0xdf), uint32(n))
	}
}

// decoder reads one value from r, charging every byte to budget.
type decoder struct {
	r      io.Reader
	budget int
	buf    [8]byte
}

// decode reads exactly one value from r, refusing to read more than limit
// bytes for it.
func decode(r io.Reader, limit int) (any, error) {
	d := &decoder{r: r, budget: limit}
	return d.value(0)
}

func (d *decoder) read(n int) ([]byte, error) {
	if n > d.budget {
		return nil, errTooLarge
	}
	d.budget -= n
	var p []byte
	if n <= len(d.buf) {
		p = d.buf[:n]
	} else {
		p = make([]byte, n)
	}
	if _, err := io.ReadFull(d.r, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (d *decoder) uint(n int) (uint64, error) {
	p, err := d.read(n)
	if err != nil {
		return 0, err
	}
	var v uint64
	for _, c := range p {
		v = v<<8 | uint64(c)
	}
	return v, nil
}

func (d *decoder) length(n int) (int, error) {
	v, err := d.uint(n)
	if err != nil {
		return 0, err
	}
	// Each element takes at least one byte, so a length above the budget
	// cannot be satisfied and is refused before any allocation.
	if v > uint64(d.budget) {
		return 0, errTooLarge
	}
	return int(v), nil
}

func (d *decoder) bytes(n int) ([]byte, error) {
	p, err := d.read(n)
	if err != nil {
		return nil, err
	}
	return append([]byte{}, p...), nil
}

func (d *decoder) value(depth int) (any, error) {
	if depth > maxDepth {
		return nil, errors.New("msgpack: nesting too deep")
	}
	p, err := d.read(1)
	if err != nil {
		return nil, err
	}
	c := p[0]
	switch {
	case c <= 0x7f:
		return int64(c), nil
	case c >= 0xe0:
		return int64(int8(c)), nil
	case c&0xe0 == 0xa0:
		return d.str(int(c & 0x1f))
	case c&0xf0 == 0x90:
		return d.array(int(c&0x0f), depth)
	case c&0xf0 == 0x80:
		return d.dict(int(c&0x0f), depth)
	}
	switch c {
	case 0xc0:
		return nil, nil
	case 0xc2:
		return false, nil
	case 0xc3:
		return true, nil
	case 0xcc, 0xcd, 0xce, 0xcf:
		v, err := d.uint(1 << (c - 0xcc))
		if err != nil {
			return nil, err
		}
		if v > math.MaxInt64 {
			return v, nil
		}
		return int64(v), nil
	case 0xd0, 0xd1, 0xd2, 0xd3:
		size := 1 << (c - 0xd0)
		v, err := d.uint(size)
		if err != nil {
			return nil, err
		}
		shift := 64 - 8*size
		return int64(v<<shift) >> shift, nil
	case 0xca:
		v, err := d.uint(4)
		return float64(math.Float32frombits(uint32(v))), err
	case 0xcb:
		v, err := d.uint(8)
		return math.Float64frombits(v), err
	case 0xd9, 0xda, 0xdb:
		n, err := d.length(1 << (c - 0xd9))
		if err != nil {
			return nil, err
		}
		return d.str(n)
	case 0xc4, 0xc5, 0xc6:
		n, err := d.length(1 << (c - 0xc4))
		if err != nil {
			return nil, err
		}
		return d.bytes(n)
	case 0xdc, 0xdd:
		n, err := d.length(2 << (c - 0xdc))
		if err != nil {
			return nil, err
		}
		return d.array(n, depth)
	case 0xde, 0xdf:
		n, err := d.length(2 << (c - 0xde))
		if err != nil {
			return nil, err
		}
		return d.dict(n, depth)
	case 0xd4, 0xd5, 0xd6, 0xd7, 0xd8:
		return d.ext(1 << (c - 0xd4))
	case 0xc7, 0xc8, 0xc9:
		n, err := d.length(1 << (c - 0xc7))
		if err != nil {
			return nil, err
		}
		return d.ext(n)
	}
	return nil, fmt.Errorf("msgpack: unsupported type byte 0x%02x", c)
}

func (d *decoder) str(n int) (any, error) {
	p, err := d.read(n)
	if err != nil {
		return nil, err
	}
	return string(p), nil
}

func (d *decoder) array(n, depth int) (any, error) {
	items := make([]any, 0, min(n, 64))
	for range n {
		v, err := d.value(depth + 1)
		if err != nil {
			return nil, err
		}
		items = append(items, v)
	}
	return items, nil
}

// dict decodes a map; Neovim's keys are strings, and an integer key is kept
// in its decimal form.
func (d *decoder) dict(n, depth int) (any, error) {
	m := make(map[string]any, min(n, 64))
	for range n {
		k, err := d.value(depth + 1)
		if err != nil {
			return nil, err
		}
		var key string
		switch k := k.(type) {
		case string:
			key = k
		case int64:
			key = strconv.FormatInt(k, 10)
		case uint64:
			key = strconv.FormatUint(k, 10)
		default:
			return nil, fmt.Errorf("msgpack: map key of type %T", k)
		}
		v, err := d.value(depth + 1)
		if err != nil {
			return nil, err
		}
		m[key] = v
	}
	return m, nil
}

func (d *decoder) ext(n int) (any, error) {
	t, err := d.read(1)
	if err != nil {
		return nil, err
	}
	typ := int8(t[0])
	data, err := d.bytes(n)
	if err != nil {
		return nil, err
	}
	return Ext{Type: typ, Data: data}, nil
}
