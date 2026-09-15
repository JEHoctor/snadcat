// Package jsonfile is an order-preserving JSON document model with the small
// set of edits `sandcat init` makes to settings files.
//
// The bash implementation edits these files with `yq -o json`, which rewrites
// the whole document in a fixed style: 2-space indent, every array element
// and object member on its own line, key order preserved. encoding/json into
// map[string]any would scramble keys and churn the user's file on every init,
// so this keeps an explicit key order and reproduces yq's layout on output.
//
// The edit helpers mirror the jq idioms used in cli/libexec/init/init —
// notably `//`, which treats null and false as absent.
package jsonfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Object is a JSON object with insertion-ordered keys.
type Object struct {
	keys []string
	vals map[string]any
}

// NewObject returns an empty object.
func NewObject() *Object { return &Object{vals: map[string]any{}} }

// Get returns the value for key.
func (o *Object) Get(key string) (any, bool) {
	v, ok := o.vals[key]
	return v, ok
}

// Set assigns key, appending it to the order when new.
func (o *Object) Set(key string, v any) {
	if _, ok := o.vals[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = v
}

// Keys returns the keys in order.
func (o *Object) Keys() []string { return append([]string(nil), o.keys...) }

// Len returns the member count.
func (o *Object) Len() int { return len(o.keys) }

// Parse decodes a document, preserving key order and number spelling.
func Parse(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := parseValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("trailing data after JSON document")
	}
	return v, nil
}

func parseValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			obj := NewObject()
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, ok := kt.(string)
				if !ok {
					return nil, fmt.Errorf("object key is not a string: %v", kt)
				}
				val, err := parseValue(dec)
				if err != nil {
					return nil, err
				}
				obj.Set(key, val)
			}
			_, err := dec.Token() // '}'
			return obj, err
		case '[':
			arr := []any{}
			for dec.More() {
				val, err := parseValue(dec)
				if err != nil {
					return nil, err
				}
				arr = append(arr, val)
			}
			_, err := dec.Token() // ']'
			return arr, err
		}
		return nil, fmt.Errorf("unexpected delimiter %v", t)
	default:
		return tok, nil // string, json.Number, bool, nil
	}
}

// Encode renders v in yq's JSON style, with a trailing newline.
func Encode(v any) ([]byte, error) {
	var sb strings.Builder
	if err := encode(&sb, v, 0); err != nil {
		return nil, err
	}
	sb.WriteByte('\n')
	return []byte(sb.String()), nil
}

func encode(sb *strings.Builder, v any, depth int) error {
	indent := func(n int) { sb.WriteString(strings.Repeat("  ", n)) }
	switch t := v.(type) {
	case *Object:
		if t.Len() == 0 {
			sb.WriteString("{}")
			return nil
		}
		sb.WriteString("{\n")
		for i, k := range t.keys {
			indent(depth + 1)
			if err := encodeScalar(sb, k); err != nil {
				return err
			}
			sb.WriteString(": ")
			if err := encode(sb, t.vals[k], depth+1); err != nil {
				return err
			}
			if i < len(t.keys)-1 {
				sb.WriteByte(',')
			}
			sb.WriteByte('\n')
		}
		indent(depth)
		sb.WriteByte('}')
	case []any:
		if len(t) == 0 {
			sb.WriteString("[]")
			return nil
		}
		sb.WriteString("[\n")
		for i, e := range t {
			indent(depth + 1)
			if err := encode(sb, e, depth+1); err != nil {
				return err
			}
			if i < len(t)-1 {
				sb.WriteByte(',')
			}
			sb.WriteByte('\n')
		}
		indent(depth)
		sb.WriteByte(']')
	default:
		return encodeScalar(sb, v)
	}
	return nil
}

func encodeScalar(sb *strings.Builder, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	// yq writes "<", ">" and "&" literally; Go's default escapes them.
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	sb.Write(bytes.TrimSuffix(buf.Bytes(), []byte("\n")))
	return nil
}

// Load reads and parses a JSON file into an object. A document whose root is
// not an object is an error, since every settings file is one.
func Load(path string) (*Object, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	v, err := Parse(b)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	obj, ok := v.(*Object)
	if !ok {
		return nil, fmt.Errorf("%s: top-level value is not an object", path)
	}
	return obj, nil
}

// Save writes the object to path in yq's layout.
func Save(path string, obj *Object) error {
	b, err := Encode(obj)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// Absent reports whether v counts as missing under jq's `//` operator, which
// falls through on null and false as well as on a missing key.
func Absent(v any, ok bool) bool {
	if !ok || v == nil {
		return true
	}
	b, isBool := v.(bool)
	return isBool && !b
}

// EnsureObject returns obj[key] as an object, replacing it with a new empty
// object when absent — the `.x = (.x // {})` idiom.
func EnsureObject(obj *Object, key string) *Object {
	v, ok := obj.Get(key)
	if child, isObj := v.(*Object); isObj && !Absent(v, ok) {
		return child
	}
	child := NewObject()
	obj.Set(key, child)
	return child
}

// EnsureArray returns obj[key] as an array, setting it to empty when absent.
func EnsureArray(obj *Object, key string) []any {
	v, ok := obj.Get(key)
	if arr, isArr := v.([]any); isArr && !Absent(v, ok) {
		return arr
	}
	obj.Set(key, []any{})
	return []any{}
}

// SetDefault assigns key only when it is absent — `.x = (.x // def)`.
func SetDefault(obj *Object, key string, def any) {
	if v, ok := obj.Get(key); Absent(v, ok) {
		obj.Set(key, def)
	}
}

// UniqueStrings appends extra to arr and removes duplicates, keeping the
// first occurrence — `(arr + extra) | unique` for string arrays.
func UniqueStrings(arr []any, extra ...string) []any {
	seen := map[string]bool{}
	var out []any
	add := func(v any) {
		if s, ok := v.(string); ok {
			if seen[s] {
				return
			}
			seen[s] = true
		}
		out = append(out, v)
	}
	for _, v := range arr {
		add(v)
	}
	for _, s := range extra {
		add(s)
	}
	if out == nil {
		out = []any{}
	}
	return out
}

// UniqueBy appends extra to arr and drops later elements whose key repeats an
// earlier one — `(arr + extra) | unique_by(f)`.
func UniqueBy(arr []any, key func(any) string, extra ...any) []any {
	seen := map[string]bool{}
	var out []any
	for _, v := range append(append([]any{}, arr...), extra...) {
		k := key(v)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, v)
	}
	if out == nil {
		out = []any{}
	}
	return out
}

// String returns obj[key] as a string, or "" when it is absent or not one.
func String(obj *Object, key string) string {
	v, _ := obj.Get(key)
	s, _ := v.(string)
	return s
}
