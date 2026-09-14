package lua

import "github.com/mmcdole/rune/script"

// registerLogFuncs registers rune._log.* primitives.
// The public rune.log API and the hooks that decide what gets written
// are defined in Lua (60_log.lua). Go only owns the file handle, so an
// active log survives /reload.
func (e *Engine) registerLogFuncs() {
	e.vm.RegisterModule("rune._log", map[string]script.GoFunc{
		// rune._log.start(path): open a log file (append, parents created).
		// Returns the resolved path, or nil + error message.
		"start": func(c *script.Call) error {
			path := c.Str(1)
			resolved, err := e.host.LogStart(path)
			if err != nil {
				c.Return(nil, err.Error())
				return nil
			}
			c.Return(resolved)
			return nil
		},

		// rune._log.stop(): close the log. Returns true if one was open.
		"stop": func(c *script.Call) error {
			c.Return(e.host.LogStop())
			return nil
		},

		// rune._log.write(text): append one line. No-op when no log is open.
		"write": func(c *script.Call) error {
			e.host.LogWrite(c.Str(1))
			return nil
		},

		// rune._log.status(): returns the active log path, or nil.
		"status": func(c *script.Call) error {
			if path, active := e.host.LogStatus(); active {
				c.Return(path)
			} else {
				c.Return(nil)
			}
			return nil
		},
	}, nil)
}
