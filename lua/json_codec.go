package lua

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	jsonMaxDepth   = 64
	jsonMaxBytes   = 5 << 20
	jsonMaxNodes   = 100000
	jsonMaxInteger = 1<<53 - 1
)

type jsonPolicy struct {
	strict                       bool
	maxDepth, maxBytes, maxNodes int
}

// The new rune.json API rejects invalid Unicode, duplicate object keys, and
// integers Lua cannot represent safely. It also limits input size and depth.
// Storage and GMCP keep their previous behavior so existing scripts continue
// to work. GMCP separately repairs raw control characters sent by some servers.
// Its nesting is server-controlled, so it is bounded during decoding rather
// than after the recursive decoder has already walked it.
var scriptJSON = jsonPolicy{strict: true, maxDepth: jsonMaxDepth, maxBytes: jsonMaxBytes, maxNodes: jsonMaxNodes}
var compatibleJSON = jsonPolicy{}
var gmcpJSON = jsonPolicy{maxDepth: jsonMaxDepth}

// jsonWriter writes Go values as JSON, using encoding/json to escape strings
// and format numbers. It checks the output size as it goes, so a large input
// can fail before the entire JSON string has been built.
type jsonWriter struct {
	strings.Builder
	nodes  int
	policy jsonPolicy
}

func (w *jsonWriter) append(s string) error {
	if w.policy.maxBytes > 0 && len(s) > w.policy.maxBytes-w.Len() {
		return fmt.Errorf("JSON exceeds 5 MiB")
	}
	w.WriteString(s)
	return nil
}

func (w *jsonWriter) quoted(s string) error {
	if w.policy.maxBytes > 0 && len(s) > w.policy.maxBytes-w.Len() {
		return fmt.Errorf("JSON exceeds 5 MiB")
	}
	if w.policy.strict && !utf8.ValidString(s) {
		return fmt.Errorf("invalid UTF-8 string")
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return w.append(string(b))
}

func validJSONNumber(n float64) error {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return fmt.Errorf("non-finite number")
	}
	if math.Trunc(n) == n && math.Abs(n) > jsonMaxInteger {
		return fmt.Errorf("integer outside the safe range -(2^53-1)..(2^53-1); use a string for large IDs")
	}
	return nil
}

func encodeJSON(value any, policy jsonPolicy) (string, error) {
	w := jsonWriter{policy: policy}
	if err := w.value(value, 0, "$"); err != nil {
		return "", err
	}
	return w.String(), nil
}

func (w *jsonWriter) value(v any, depth int, path string) error {
	w.nodes++
	if w.policy.maxNodes > 0 && w.nodes > w.policy.maxNodes {
		return fmt.Errorf("%s: JSON exceeds %d values", path, w.policy.maxNodes)
	}
	scalar := func() error {
		switch n := v.(type) {
		case float64:
			if w.policy.strict {
				if err := validJSONNumber(n); err != nil {
					return err
				}
			}
		case string:
			return w.quoted(n)
		}
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return w.append(string(b))
	}
	switch value := v.(type) {
	case nil, bool, string, float64:
		if err := scalar(); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		return nil
	case []any:
		if w.policy.maxDepth > 0 && depth >= w.policy.maxDepth {
			return fmt.Errorf("%s: JSON exceeds %d container levels", path, w.policy.maxDepth)
		}
		if err := w.append("["); err != nil {
			return err
		}
		for i, child := range value {
			if i > 0 {
				if err := w.append(","); err != nil {
					return err
				}
			}
			if err := w.value(child, depth+1, fmt.Sprintf("%s[%d]", path, i+1)); err != nil {
				return err
			}
		}
		return w.append("]")
	case map[string]any:
		if w.policy.maxDepth > 0 && depth >= w.policy.maxDepth {
			return fmt.Errorf("%s: JSON exceeds %d container levels", path, w.policy.maxDepth)
		}
		if err := w.append("{"); err != nil {
			return err
		}
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for i, key := range keys {
			if i > 0 {
				if err := w.append(","); err != nil {
					return err
				}
			}
			if err := w.quoted(key); err != nil {
				return fmt.Errorf("%s: object key: %w", path, err)
			}
			if err := w.append(":"); err != nil {
				return err
			}
			if err := w.value(value[key], depth+1, fmt.Sprintf("%s[%q]", path, key)); err != nil {
				return err
			}
		}
		return w.append("}")
	default:
		return fmt.Errorf("%s: unsupported JSON value %T", path, value)
	}
}

func decodeJSON(text string, policy jsonPolicy) (any, error) {
	if policy.maxBytes > 0 && len(text) > policy.maxBytes {
		return nil, fmt.Errorf("JSON exceeds 5 MiB")
	}
	if policy.strict && !utf8.ValidString(text) {
		return nil, fmt.Errorf("invalid UTF-8 JSON")
	}
	if policy.strict {
		if err := validateJSONSurrogates(text); err != nil {
			return nil, err
		}
	}
	d := json.NewDecoder(strings.NewReader(text))
	d.UseNumber()
	nodes := 0
	value, err := readJSONValue(d, 0, &nodes, policy)
	if err != nil {
		return nil, fmt.Errorf("JSON near byte %d: %w", d.InputOffset(), err)
	}
	if _, err := d.Token(); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("unexpected trailing value")
		}
		return nil, fmt.Errorf("JSON near byte %d: %w", d.InputOffset(), err)
	}
	return value, nil
}

// readJSONValue parses one value into Go maps, slices, and scalar values.
// It leaves JSON null as Go nil. The caller decides how to present those
// values to Lua: rune.json preserves null and array/object types, while the
// existing storage and GMCP APIs use nil and ordinary tables.
func readJSONValue(d *json.Decoder, depth int, nodes *int, policy jsonPolicy) (any, error) {
	*nodes++
	if policy.maxNodes > 0 && *nodes > policy.maxNodes {
		return nil, fmt.Errorf("JSON exceeds %d values", policy.maxNodes)
	}
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	switch v := token.(type) {
	case nil:
		return nil, nil
	case string, bool:
		return v, nil
	case json.Number:
		n, err := strconv.ParseFloat(string(v), 64)
		if err != nil {
			return nil, fmt.Errorf("number outside Lua's range")
		}
		if policy.strict {
			if err := validJSONNumber(n); err != nil {
				return nil, err
			}
		}
		return n, nil
	case json.Delim:
		if policy.maxDepth > 0 && depth >= policy.maxDepth {
			return nil, fmt.Errorf("JSON exceeds %d container levels", policy.maxDepth)
		}
		if v == '[' {
			values := []any{}
			for d.More() {
				child, err := readJSONValue(d, depth+1, nodes, policy)
				if err != nil {
					return nil, err
				}
				values = append(values, child)
			}
			if _, err := d.Token(); err != nil {
				return nil, err
			}
			return values, nil
		}
		if v == '{' {
			values := map[string]any{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return nil, err
				}
				name, ok := key.(string)
				if !ok {
					return nil, fmt.Errorf("object key must be a string")
				}
				if _, exists := values[name]; exists && policy.strict {
					return nil, fmt.Errorf("duplicate object key %q", name)
				}
				child, err := readJSONValue(d, depth+1, nodes, policy)
				if err != nil {
					return nil, err
				}
				values[name] = child
			}
			if _, err := d.Token(); err != nil {
				return nil, err
			}
			return values, nil
		}
	}
	return nil, fmt.Errorf("unexpected token %v", token)
}

// A Unicode escape such as \uD800 must be followed by its matching low
// surrogate. Go's JSON parser replaces an unmatched surrogate with the
// replacement character (U+FFFD). Check these escapes first so rune.json can
// report malformed input instead of silently changing it. The standard
// decoder checks the rest of the JSON syntax.
func validateJSONSurrogates(text string) error {
	inString := false
	hex4 := func(s string) (uint64, bool) { n, err := strconv.ParseUint(s, 16, 16); return n, err == nil }
	for i := 0; i < len(text); i++ {
		if text[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || text[i] != '\\' {
			continue
		}
		i++
		if i >= len(text) {
			break
		}
		if text[i] != 'u' || i+4 >= len(text) {
			continue
		}
		n, ok := hex4(text[i+1 : i+5])
		if !ok {
			continue
		}
		if n >= 0xDC00 && n <= 0xDFFF {
			return fmt.Errorf("unpaired Unicode surrogate at byte %d", i)
		}
		if n >= 0xD800 && n <= 0xDBFF {
			if i+10 >= len(text) || text[i+5:i+7] != `\u` {
				return fmt.Errorf("unpaired Unicode surrogate at byte %d", i)
			}
			low, ok := hex4(text[i+7 : i+11])
			if !ok || low < 0xDC00 || low > 0xDFFF {
				return fmt.Errorf("unpaired Unicode surrogate at byte %d", i)
			}
			i += 6
		}
		i += 4
	}
	return nil
}
