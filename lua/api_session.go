package lua

import "github.com/mmcdole/rune/script"

// registerSessionFuncs registers rune._session.* primitives.
// The public rune.session API is defined in Lua (00_init.lua).
func (e *Engine) registerSessionFuncs() {
	e.vm.RegisterModule("rune._session", map[string]script.GoFunc{
		// rune._session.set(key, value): store a string
		"set": func(c *script.Call) error {
			key := c.Str(1)
			value := c.Str(2)
			e.host.SessionSet(key, value)
			return nil
		},

		// rune._session.get(key): returns the string, or nil if unset
		"get": func(c *script.Call) error {
			key := c.Str(1)
			if v, ok := e.host.SessionGet(key); ok {
				c.Return(v)
			} else {
				c.Return(nil)
			}
			return nil
		},

		// rune._session.delete(key): remove a key
		"delete": func(c *script.Call) error {
			key := c.Str(1)
			e.host.SessionDelete(key)
			return nil
		},
	}, nil)
}
