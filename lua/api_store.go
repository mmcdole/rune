package lua

import (
	"encoding/json"

	"github.com/mmcdole/rune/script"
)

// maxStoreDepth bounds nesting during script→JSON conversion; combined
// with cycle detection it keeps rune.store.set from recursing forever.
const maxStoreDepth = 64

// registerStoreFuncs registers rune._store.* primitives.
// The public rune.store API is defined in Lua (00_init.lua).
func (e *Engine) registerStoreFuncs() {
	e.vm.RegisterModule("rune._store", map[string]script.GoFunc{
		// rune._store.set(key, value): value may be a string, number,
		// boolean, or JSON-able table; nil deletes the key.
		// Returns true, or nil + error message.
		"set": func(c *script.Call) error {
			key := c.Str(1)
			value := c.Arg(2)

			if value.IsNil() {
				if err := e.host.StoreDelete(key); err != nil {
					c.Return(nil, err.Error())
					return nil
				}
				c.Return(true)
				return nil
			}

			gv, err := script.DecodeTree(value, maxStoreDepth)
			if err != nil {
				c.Return(nil, err.Error())
				return nil
			}
			raw, err := json.Marshal(gv)
			if err != nil {
				c.Return(nil, err.Error())
				return nil
			}
			if err := e.host.StoreSet(key, string(raw)); err != nil {
				c.Return(nil, err.Error())
				return nil
			}
			c.Return(true)
			return nil
		},

		// rune._store.get(key): returns the decoded value, or nil.
		"get": func(c *script.Call) error {
			key := c.Str(1)
			raw, ok := e.host.StoreGet(key)
			if !ok {
				c.Return(nil)
				return nil
			}
			var v any
			if err := json.Unmarshal([]byte(raw), &v); err != nil {
				c.Return(nil, err.Error())
				return nil
			}
			c.Return(script.Tree{V: v})
			return nil
		},

		// rune._store.delete(key): returns true, or nil + error message.
		"delete": func(c *script.Call) error {
			key := c.Str(1)
			if err := e.host.StoreDelete(key); err != nil {
				c.Return(nil, err.Error())
				return nil
			}
			c.Return(true)
			return nil
		},
	}, nil)
}
