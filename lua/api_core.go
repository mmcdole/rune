package lua

import (
	"github.com/mmcdole/rune/script"
	"github.com/mmcdole/rune/text"
)

// registerCoreFuncs registers internal rune._* primitives (wrapped by Lua).
//
// Convention: recoverable runtime failures (not connected, file not
// found) return nil + error message rather than raising, matching
// rune._regex.compile. Raising is reserved for programmer errors like
// wrong argument types (c.Str and friends).
func (e *Engine) registerCoreFuncs() {
	e.vm.RegisterModule("rune", map[string]script.GoFunc{
		// rune._send_raw(text): Bypasses alias processing, writes directly to socket.
		// Returns true, or nil + error message.
		"_send_raw": func(c *script.Call) error {
			cmd := c.Str(1)
			if err := e.host.Send(cmd); err != nil {
				c.Return(nil, err.Error())
				return nil
			}
			c.Return(true)
			return nil
		},

		// rune._echo(text): Outputs text to the local game window
		"_echo": func(c *script.Call) error {
			msg := c.Str(1)
			e.host.Print(msg)
			return nil
		},

		// rune._quit(): Exit the client
		"_quit": func(c *script.Call) error {
			e.host.Quit()
			return nil
		},

		// rune._connect(address): Connect to server
		"_connect": func(c *script.Call) error {
			addr := c.Str(1)
			e.host.Connect(addr)
			return nil
		},

		// rune._disconnect(): Disconnect from server
		"_disconnect": func(c *script.Call) error {
			e.host.Disconnect()
			return nil
		},

		// rune._reload(): Reload all scripts
		"_reload": func(c *script.Call) error {
			e.host.Reload()
			return nil
		},

		// rune._strip_ansi(text): Remove ANSI escape sequences.
		// Used by rune.line.new so Lua-built line objects get the same
		// clean text as lines arriving from the server.
		"_strip_ansi": func(c *script.Call) error {
			s := c.Str(1)
			c.Return(text.StripANSI(s))
			return nil
		},

		// rune._load(path): Load a Lua script (runs immediately, no round-trip).
		// Returns true, or nil + error message.
		"_load": func(c *script.Call) error {
			path := c.Str(1)
			if err := e.DoFile(path); err != nil {
				c.Return(nil, err.Error())
				return nil
			}
			e.CallHook("loaded", path)
			c.Return(true)
			return nil
		},
	}, nil)
}
