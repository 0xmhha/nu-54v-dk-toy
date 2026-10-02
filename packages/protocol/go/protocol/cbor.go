package protocol

// Deterministic CBOR for BLE message bodies (payment-protocol.md 4.2), the Go twin of
// ts/src/cbor.ts. Messages use the JSON form of the schema and vectors: byte fields as hex
// strings (sessionId without 0x), uints as decimal strings, text, booleans and the integer v.
// Field kinds come from the generated MessageFields, so both directions follow the schema.

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"unicode/utf8"
)

// Message is a protocol message in the JSON form: string, bool, int (v) or a nested Message.
type Message map[string]any

// ProtocolError is a refusal the receiver reports: BAD_FRAME or UNSUPPORTED_TYPE.
type ProtocolError struct {
	Reason Reason
	Detail string
}

func (e *ProtocolError) Error() string { return string(e.Reason) + ": " + e.Detail }

func badFrame(format string, a ...any) error {
	return &ProtocolError{ReasonBadFrame, fmt.Sprintf(format, a...)}
}

var byteLengths = map[string]int{"hex20": 20, "hex32": 32, "signature": 65, "sessionId": 8}

var uint64Limit = new(big.Int).Lsh(big.NewInt(1), 64)

func head(major byte, n uint64) []byte {
	switch {
	case n < 24:
		return []byte{major<<5 | byte(n)}
	case n < 1<<8:
		return []byte{major<<5 | 24, byte(n)}
	case n < 1<<16:
		return []byte{major<<5 | 25, byte(n >> 8), byte(n)}
	case n < 1<<32:
		return []byte{major<<5 | 26, byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}
	}
	out := []byte{major<<5 | 27}
	for i := 7; i >= 0; i-- {
		out = append(out, byte(n>>(8*i)))
	}
	return out
}

func encodeText(s string) []byte { return append(head(3, uint64(len(s))), s...) }

func parseUint(v any) (*big.Int, error) {
	switch x := v.(type) {
	case string:
		n, ok := new(big.Int).SetString(x, 10)
		if !ok || n.Sign() < 0 || strings.TrimLeft(x, "0123456789") != "" {
			return nil, fmt.Errorf("not a decimal uint: %q", x)
		}
		return n, nil
	case int:
		if x < 0 {
			return nil, errors.New("negative uint")
		}
		return big.NewInt(int64(x)), nil
	case uint64:
		return new(big.Int).SetUint64(x), nil
	case *big.Int:
		if x.Sign() < 0 {
			return nil, errors.New("negative uint")
		}
		return x, nil
	}
	return nil, fmt.Errorf("uint expected, got %T", v)
}

func encodeValue(kind FieldKind, v any, path string) ([]byte, error) {
	if kind.Object != nil {
		m, ok := v.(Message)
		if !ok {
			if raw, ok2 := v.(map[string]any); ok2 {
				m, ok = Message(raw), true
			}
		}
		if !ok {
			return nil, fmt.Errorf("%s: object expected", path)
		}
		return encodeObject(kind.Object, m, path)
	}
	if n, ok := byteLengths[kind.Name]; ok {
		s, _ := v.(string)
		raw, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
		if err != nil || len(raw) != n {
			return nil, fmt.Errorf("%s: %s must be %d bytes", path, kind.Name, n)
		}
		return append(head(2, uint64(n)), raw...), nil
	}
	switch kind.Name {
	case "uint":
		n, err := parseUint(v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if n.Cmp(uint64Limit) < 0 {
			return head(0, n.Uint64()), nil
		}
		raw := n.Bytes() // tag 2 bignum without leading zero bytes
		return append(append(head(6, 2), head(2, uint64(len(raw)))...), raw...), nil
	case "int":
		i, ok := v.(int)
		if !ok || i < 0 {
			return nil, fmt.Errorf("%s: integer expected", path)
		}
		return head(0, uint64(i)), nil
	case "bool":
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("%s: boolean expected", path)
		}
		if b {
			return []byte{0xf5}, nil
		}
		return []byte{0xf4}, nil
	case "text":
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%s: text expected", path)
		}
		return encodeText(s), nil
	}
	return nil, fmt.Errorf("%s: unknown kind %q", path, kind.Name)
}

func encodeObject(kind *ObjectKind, m Message, path string) ([]byte, error) {
	for _, r := range kind.Required {
		if _, ok := m[r]; !ok {
			return nil, fmt.Errorf("%s: missing %s", path, r)
		}
	}
	type kv struct{ k, v []byte }
	items := make([]kv, 0, len(m))
	for k, v := range m {
		fk, ok := kind.Fields[k]
		if !ok {
			return nil, fmt.Errorf("%s: unknown %s", path, k)
		}
		enc, err := encodeValue(fk, v, path+"."+k)
		if err != nil {
			return nil, err
		}
		items = append(items, kv{encodeText(k), enc})
	}
	// RFC 8949 4.2.1: keys in bytewise order of their encoding.
	sort.Slice(items, func(i, j int) bool { return bytes.Compare(items[i].k, items[j].k) < 0 })
	out := head(5, uint64(len(items)))
	for _, it := range items {
		out = append(append(out, it.k...), it.v...)
	}
	return out, nil
}

// EncodeMessage encodes a message in the JSON form to its deterministic CBOR body.
func EncodeMessage(m Message) ([]byte, error) {
	t, _ := m["type"].(string)
	kind, ok := MessageFields[t]
	if !ok {
		return nil, &ProtocolError{ReasonUnsupportedType, "unknown message type " + t}
	}
	return encodeObject(kind, m, t)
}

// ---------------------------------------------------------------- decoding

type item struct {
	major   byte
	u       *big.Int // major 0, and 6 (bignum)
	b       []byte   // major 2
	s       string   // major 3
	entries []entry  // major 5
	boolean bool     // major 7
}

type entry struct {
	key string
	raw []byte // encoded key, for the order check
	val item
}

type reader struct {
	b []byte
	i int
}

func (r *reader) take(n uint64) ([]byte, error) {
	if n > uint64(len(r.b)-r.i) {
		return nil, badFrame("truncated CBOR")
	}
	out := r.b[r.i : r.i+int(n)]
	r.i += int(n)
	return out, nil
}

func (r *reader) head() (major, info byte, arg uint64, err error) {
	ib, err := r.take(1)
	if err != nil {
		return
	}
	major, info = ib[0]>>5, ib[0]&0x1f
	if info < 24 {
		return major, info, uint64(info), nil
	}
	sizes := map[byte]uint64{24: 1, 25: 2, 26: 4, 27: 8}
	size, ok := sizes[info]
	if !ok {
		return 0, 0, 0, badFrame("indefinite length or reserved value")
	}
	raw, err := r.take(size)
	if err != nil {
		return
	}
	for _, x := range raw {
		arg = arg<<8 | uint64(x)
	}
	// Shortest form: the argument must not fit a smaller encoding.
	min := uint64(24)
	if size > 1 {
		min = 1 << (8 * (size / 2))
	}
	if arg < min {
		return 0, 0, 0, badFrame("non-shortest integer or length")
	}
	return major, info, arg, nil
}

func (r *reader) item(depth int) (item, error) {
	if depth > 4 {
		return item{}, badFrame("nesting too deep")
	}
	start := r.i
	major, _, arg, err := r.head()
	if err != nil {
		return item{}, err
	}
	switch major {
	case 0:
		return item{major: 0, u: new(big.Int).SetUint64(arg)}, nil
	case 2:
		b, err := r.take(arg)
		return item{major: 2, b: b}, err
	case 3:
		b, err := r.take(arg)
		if err != nil {
			return item{}, err
		}
		if !utf8.Valid(b) {
			return item{}, badFrame("invalid UTF-8")
		}
		return item{major: 3, s: string(b)}, nil
	case 5:
		out := item{major: 5}
		var prev []byte
		for k := uint64(0); k < arg; k++ {
			ks := r.i
			key, err := r.item(depth + 1)
			if err != nil {
				return item{}, err
			}
			if key.major != 3 {
				return item{}, badFrame("map key is not text")
			}
			kb := r.b[ks:r.i]
			if prev != nil && bytes.Compare(prev, kb) >= 0 {
				return item{}, badFrame("map keys not in deterministic order or duplicated")
			}
			prev = kb
			val, err := r.item(depth + 1)
			if err != nil {
				return item{}, err
			}
			out.entries = append(out.entries, entry{key.s, kb, val})
		}
		return out, nil
	case 6:
		if arg != 2 {
			return item{}, badFrame("tag other than 2")
		}
		raw, err := r.item(depth + 1)
		if err != nil {
			return item{}, err
		}
		if raw.major != 2 {
			return item{}, badFrame("bignum is not a byte string")
		}
		if len(raw.b) == 0 || raw.b[0] == 0 {
			return item{}, badFrame("bignum with leading zero")
		}
		v := new(big.Int).SetBytes(raw.b)
		if v.Cmp(uint64Limit) < 0 {
			return item{}, badFrame("bignum below 2^64")
		}
		return item{major: 6, u: v}, nil
	case 7:
		switch r.b[start] {
		case 0xf4:
			return item{major: 7, boolean: false}, nil
		case 0xf5:
			return item{major: 7, boolean: true}, nil
		}
		return item{}, badFrame("float or simple value")
	}
	return item{}, badFrame("unsupported major type %d", major)
}

func toJSON(kind FieldKind, it item, path string) (any, error) {
	if kind.Object != nil {
		if it.major != 5 {
			return nil, badFrame("%s: map expected", path)
		}
		out := Message{}
		for _, e := range it.entries {
			fk, ok := kind.Object.Fields[e.key]
			if !ok {
				return nil, badFrame("%s: unknown key %s", path, e.key)
			}
			v, err := toJSON(fk, e.val, path+"."+e.key)
			if err != nil {
				return nil, err
			}
			out[e.key] = v
		}
		for _, r := range kind.Object.Required {
			if _, ok := out[r]; !ok {
				return nil, badFrame("%s: missing %s", path, r)
			}
		}
		return out, nil
	}
	if n, ok := byteLengths[kind.Name]; ok {
		if it.major != 2 || len(it.b) != n {
			return nil, badFrame("%s: %s must be %d bytes", path, kind.Name, n)
		}
		if kind.Name == "sessionId" {
			return hex.EncodeToString(it.b), nil
		}
		return "0x" + hex.EncodeToString(it.b), nil
	}
	switch kind.Name {
	case "uint":
		if it.major != 0 && it.major != 6 {
			return nil, badFrame("%s: uint expected", path)
		}
		return it.u.String(), nil
	case "int":
		if it.major != 0 || !it.u.IsInt64() || it.u.Int64() > 1<<53 {
			return nil, badFrame("%s: integer expected", path)
		}
		return int(it.u.Int64()), nil
	case "bool":
		if it.major != 7 {
			return nil, badFrame("%s: boolean expected", path)
		}
		return it.boolean, nil
	case "text":
		if it.major != 3 {
			return nil, badFrame("%s: text expected", path)
		}
		return it.s, nil
	}
	return nil, badFrame("%s: unknown kind", path)
}

// DecodeMessage decodes and validates a CBOR body; any rule violation is a *ProtocolError.
func DecodeMessage(body []byte) (Message, error) {
	r := &reader{b: body}
	it, err := r.item(0)
	if err != nil {
		return nil, err
	}
	if r.i != len(body) {
		return nil, badFrame("trailing bytes after the CBOR item")
	}
	if it.major != 5 {
		return nil, badFrame("body is not a map")
	}
	var t string
	for _, e := range it.entries {
		if e.key == "type" && e.val.major == 3 {
			t = e.val.s
		}
	}
	if t == "" {
		return nil, badFrame("missing type")
	}
	kind, ok := MessageFields[t]
	if !ok {
		return nil, &ProtocolError{ReasonUnsupportedType, "unknown message type " + t}
	}
	v, err := toJSON(FieldKind{Object: kind}, it, t)
	if err != nil {
		return nil, err
	}
	return v.(Message), nil
}
