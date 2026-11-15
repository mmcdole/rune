package lua

import (
	"strings"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/script"
)

// registerHistoryFuncs registers rune._history.* primitives.
// The public rune.history API is defined in Lua (75_input.lua).
func (e *Engine) registerHistoryFuncs() {
	e.vm.RegisterModule("rune._history", map[string]script.GoFunc{
		// rune._history.entries() - Returns structured history, oldest first.
		// Mode is a stable string so Lua does not depend on Go enum values.
		"entries": func(c *script.Call) error {
			returnHistory(c, e.host.GetHistoryEntries())
			return nil
		},

		// rune._history.find(prefix, from, step) - Walk history from position
		// from (1 = newest) by step (1 = older, -1 = newer) and return the
		// first entry whose text starts with prefix as position, text, mode,
		// or nothing. An empty prefix matches the entry at from. Lets Up and
		// Down move one entry without copying the whole history into Lua.
		"find": func(c *script.Call) error {
			prefix, from, step := c.Str(1), c.Int(2), c.Int(3)
			if step != 1 && step != -1 {
				return c.Errorf("rune._history.find: step must be 1 or -1")
			}
			history := e.host.GetHistoryEntries()
			for pos := from; pos >= 1 && pos <= len(history); pos += step {
				entry := history[len(history)-pos]
				if strings.HasPrefix(entry.Text, prefix) {
					c.Return(pos, entry.Text, entry.Mode.String())
					return nil
				}
			}
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
