package lua

import (
	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/script"
)

// registerHistoryFuncs registers rune._history.* primitives.
// The public rune.history API is defined in Lua (00_init.lua).
func (e *Engine) registerHistoryFuncs() {
	e.vm.RegisterModule("rune._history", map[string]script.GoFunc{
		// rune._history.entries() - Returns structured history, oldest first.
		// Mode is a stable string so Lua does not depend on Go enum values.
		"entries": func(c *script.Call) error {
			returnHistory(c, e.host.GetHistoryEntries())
			return nil
		},

		// Internal snapshot for expansion; public history remains live.
		"expansion_entries": func(c *script.Call) error {
			history := e.inputHistory
			if history == nil {
				history = e.host.GetHistoryEntries()
			}
			returnHistory(c, history)
			return nil
		},

		// rune._history.add(cmd) - Add a command to history
		"add": func(c *script.Call) error {
			cmd := c.Str(1)
			if !input.ValidCommandText(cmd) {
				return c.Errorf("rune.history.add only accepts valid command text; terminal controls are not allowed")
			}
			e.host.AddToHistory(cmd)
			return nil
		},
	}, nil)
}

func returnHistory(c *script.Call, history []input.Submission) {
	arr := make([]any, len(history))
	for i, entry := range history {
		arr[i] = map[string]any{"text": entry.Text, "mode": entry.Mode.String()}
	}
	c.Return(script.Tree{V: arr})
}
