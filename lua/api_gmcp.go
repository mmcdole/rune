package lua

import (
	"encoding/json"

	"github.com/mmcdole/rune/script"
)

// registerGMCPFuncs registers rune._gmcp.* primitives.
// The public rune.gmcp API (handlers, subscriptions, the Core.Hello
// handshake) is defined in Lua (70_gmcp.lua). Encoding goes through
// the shared script-tree/JSON bridge (see api_store.go).
func (e *Engine) registerGMCPFuncs() {
	e.vm.RegisterModule("rune._gmcp", map[string]script.GoFunc{
		// rune._gmcp.send(package, value?): value may be a string, number,
		// boolean, or JSON-able table; nil sends the bare package name.
		// Returns true, or nil + error message (not connected, GMCP not
		// negotiated, unencodable value).
		"send": func(c *script.Call) error {
			pkg := c.Str(1)
			value := c.Arg(2)

			data := ""
			if !value.IsNil() {
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
				data = string(raw)
			}

			if err := e.host.GMCPSend(pkg, data); err != nil {
				c.Return(nil, err.Error())
				return nil
			}
			c.Return(true)
			return nil
		},

		// rune._gmcp.is_active(): whether GMCP is negotiated on the
		// current connection. Queried live so it cannot go stale across
		// /reload (see Host.GMCPActive).
		"is_active": func(c *script.Call) error {
			c.Return(e.host.GMCPActive())
			return nil
		},

		// rune._gmcp.send_raw(package, json?): sends the JSON text
		// verbatim (no validation) - the debugging escape hatch used by
		// "/gmcp send". Returns true, or nil + error message.
		"send_raw": func(c *script.Call) error {
			pkg := c.Str(1)
			data := c.OptStr(2, "")
			if err := e.host.GMCPSend(pkg, data); err != nil {
				c.Return(nil, err.Error())
				return nil
			}
			c.Return(true)
			return nil
		},
	}, nil)
}
