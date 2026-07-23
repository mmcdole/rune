package lua

import "github.com/mmcdole/rune/script"

// registerHistoryFuncs registers rune._history.* primitives.
// The public rune.history API is defined in Lua (00_init.lua).
func (e *Engine) registerHistoryFuncs() {
	e.vm.RegisterModule("rune._history", map[string]script.GoFunc{
		// rune._history.get() - Returns array of input history strings
		"get": func(c *script.Call) error {
			history := e.host.GetHistory()
			arr := make([]any, len(history))
			for i, cmd := range history {
				arr[i] = cmd
			}
			c.Return(script.Tree{V: arr})
			return nil
		},

		// rune._history.entries() - Returns structured history, oldest first.
		// Mode is a stable string so Lua does not depend on Go enum values.
		"entries": func(c *script.Call) error {
			history := e.host.GetHistoryEntries()
			arr := make([]any, len(history))
			for i, entry := range history {
				arr[i] = map[string]any{
					"text": entry.Text,
					"mode": entry.Mode.String(),
				}
			}
			c.Return(script.Tree{V: arr})
			return nil
		},

		// rune._history.add(cmd) - Add a command to history
		"add": func(c *script.Call) error {
			cmd := c.Str(1)
			e.host.AddToHistory(cmd)
			return nil
		},
	}, nil)
}
