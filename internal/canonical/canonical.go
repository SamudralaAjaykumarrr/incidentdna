// Package canonical produces a deterministic JSON encoding of a value: object
// keys sorted, no insignificant whitespace, fixed string/number formatting.
// Two values that are structurally equal but differ in source formatting
// (key order, indentation, quoting style) always canonicalize to identical
// bytes; this is the input to internal/fingerprint's hash.
package canonical

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// Marshal encodes v as canonical JSON. v is first marshaled with the
// standard library (so any json.Marshaler / struct tags on v are honored),
// then decoded into a generic tree with json.Number preserved (avoiding
// float64 precision loss) and re-encoded with object keys sorted and no
// insignificant whitespace.
func Marshal(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("canonical: marshal input: %w", err)
	}
	return MarshalJSON(raw)
}

// MarshalJSON re-encodes already-serialized JSON bytes into canonical form.
// It is the primitive Marshal builds on, and is exposed directly because
// callers sometimes already hold JSON bytes (e.g. read from a file) and want
// to canonicalize without a round trip through a typed Go value.
func MarshalJSON(data []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var tree any
	if err := dec.Decode(&tree); err != nil {
		return nil, fmt.Errorf("canonical: decode: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF { //nolint:errcheck // deliberate EOF check
		return nil, fmt.Errorf("canonical: trailing data after JSON value")
	}

	var buf bytes.Buffer
	if err := encode(&buf, tree); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encode(buf *bytes.Buffer, v any) error {
	switch val := v.(type) {
	case nil:
		buf.WriteString("null")
		return nil
	case bool:
		if val {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
		return nil
	case json.Number:
		buf.WriteString(val.String())
		return nil
	case string:
		return encodeString(buf, val)
	case []any:
		return encodeArray(buf, val)
	case map[string]any:
		return encodeObject(buf, val)
	default:
		return fmt.Errorf("canonical: unsupported type %T", v)
	}
}

func encodeArray(buf *bytes.Buffer, arr []any) error {
	buf.WriteByte('[')
	for i, elem := range arr {
		if i > 0 {
			buf.WriteByte(',')
		}
		if err := encode(buf, elem); err != nil {
			return err
		}
	}
	buf.WriteByte(']')
	return nil
}

func encodeObject(buf *bytes.Buffer, obj map[string]any) error {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		if err := encodeString(buf, k); err != nil {
			return err
		}
		buf.WriteByte(':')
		if err := encode(buf, obj[k]); err != nil {
			return err
		}
	}
	buf.WriteByte('}')
	return nil
}

// encodeString writes s as a JSON string using encoding/json's own escaping
// rules (via a throwaway Marshal), so canonical output stays byte-for-byte
// consistent with the standard library's escaping of control characters,
// unicode, and quotes rather than reimplementing that logic.
func encodeString(buf *bytes.Buffer, s string) error {
	b, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("canonical: encode string: %w", err)
	}
	buf.Write(b)
	return nil
}
