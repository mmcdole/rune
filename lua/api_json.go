package lua

import (
	"fmt"

	"github.com/mmcdole/rune/script"
)

func (e *Engine) registerJSONFuncs() {
	e.vm.RegisterType("JSONNull", nil)
	e.vm.RegisterModule("rune._json", map[string]script.GoFunc{
		"encode": func(c *script.Call) error {
			nodes := 0
			value, err := unpackJSON(c.Arg(1), 0, &nodes)
			if err != nil {
				c.Return(nil, err.Error())
				return nil
			}
			text, err := encodeJSON(value, scriptJSON)
			if err != nil {
				c.Return(nil, err.Error())
			} else {
				c.Return(text)
			}
			return nil
		},
		"decode": func(c *script.Call) error {
			value, err := decodeJSON(c.Str(1), scriptJSON)
			if err != nil {
				c.Return(nil, err.Error())
			} else {
				c.Return(script.Tree{V: packJSON(value)})
			}
			return nil
		},
	}, map[string]any{
		"null":      script.Obj{Type: "JSONNull", Payload: struct{}{}},
		"max_depth": jsonMaxDepth, "max_bytes": jsonMaxBytes, "max_nodes": jsonMaxNodes,
	})
}

// packJSON prepares decoded JSON for Lua. The usual Go-to-Lua conversion
// turns nil into Lua nil, losing null table entries, and turns both empty
// arrays and empty objects into {}. Wrappers such as {"array", items} and
// {"null"} preserve those distinctions until 10_json.lua restores the values.
// This function modifies the supplied maps and slices, which belong to this
// decode call and are not shared with other callers.
func packJSON(value any) any {
	switch v := value.(type) {
	case nil:
		return []any{"null"}
	case []any:
		for i, child := range v {
			v[i] = packJSON(child)
		}
		return []any{"array", v}
	case map[string]any:
		for key, child := range v {
			v[key] = packJSON(child)
		}
		return []any{"object", v}
	default:
		return value
	}
}

// unpackJSON removes the wrappers built by 10_json.lua before the shared
// Go encoder writes JSON. Check the limits here too because scripts can
// call the internal primitive directly, bypassing the public Lua function.
func unpackJSON(value script.Value, depth int, nodes *int) (any, error) {
	*nodes++
	if *nodes > jsonMaxNodes {
		return nil, fmt.Errorf("JSON exceeds %d values", jsonMaxNodes)
	}
	switch value.Kind() {
	case script.KindString:
		return value.Str(), nil
	case script.KindNumber:
		return value.Num(), nil
	case script.KindBool:
		return value.Bool(), nil
	case script.KindTable:
		node := value.Table()
		kind := node.Index(1).Str()
		if kind == "null" {
			return nil, nil
		}
		if depth >= jsonMaxDepth {
			return nil, fmt.Errorf("JSON exceeds %d container levels", jsonMaxDepth)
		}
		contents := node.Index(2).Table()
		if contents == nil {
			return nil, fmt.Errorf("invalid internal JSON container")
		}
		if kind == "array" {
			n := contents.Len()
			if n > jsonMaxNodes-*nodes {
				return nil, fmt.Errorf("JSON exceeds %d values", jsonMaxNodes)
			}
			result := make([]any, n)
			for i := range result {
				v, err := unpackJSON(contents.Index(i+1), depth+1, nodes)
				if err != nil {
					return nil, err
				}
				result[i] = v
			}
			return result, nil
		}
		if kind == "object" {
			result := map[string]any{}
			var walkErr error
			contents.Each(func(key, child script.Value) bool {
				if key.Kind() != script.KindString {
					walkErr = fmt.Errorf("invalid internal JSON object key")
					return false
				}
				var v any
				v, walkErr = unpackJSON(child, depth+1, nodes)
				if walkErr == nil {
					result[key.Str()] = v
				}
				return walkErr == nil
			})
			return result, walkErr
		}
	}
	return nil, fmt.Errorf("invalid internal JSON value")
}
