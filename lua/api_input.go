package lua

import (
	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/script"
)

// registerInputFuncs registers rune._input.* primitives.
// The public rune.input API is defined in Lua (90_input.lua).
func (e *Engine) registerInputFuncs() {
	e.vm.RegisterModule("rune._input", map[string]script.GoFunc{
		"get": func(c *script.Call) error {
			c.Return(e.host.GetInput())
			return nil
		},

		"set": func(c *script.Call) error {
			text := c.Str(1)
			e.host.SetInput(text)
			return nil
		},

		// rune._input.restore(text, mode) restores both content and submission
		// interpretation. This is intentionally separate from set(): ordinary
		// script edits preserve the current UI mode, while history recall needs
		// to distinguish equal command and verbatim entries.
		"restore": func(c *script.Call) error {
			text := c.Str(1)
			modeName := c.Str(2)
			mode := input.ModeCommand
			switch modeName {
			case "command":
			case "verbatim":
				mode = input.ModeVerbatim
			default:
				return c.Errorf("bad argument #%d (%s)", 2, "mode must be 'command' or 'verbatim'")
			}
			e.host.SetInputSubmission(input.Submission{Text: text, Mode: mode})
			return nil
		},

		// Cursor primitives
		"get_cursor": func(c *script.Call) error {
			c.Return(e.host.InputGetCursor())
			return nil
		},

		"set_cursor": func(c *script.Call) error {
			pos := c.Int(1)
			e.host.InputSetCursor(pos)
			return nil
		},

		// Editor mode primitive. The host call blocks in $EDITOR for as
		// long as the user edits, so it runs outside the watchdog deadline.
		"open_editor": func(c *script.Call) error {
			initial := c.OptStr(1, "")
			var result string
			var ok bool
			e.pauseWatchdog(func() {
				result, ok = e.host.OpenEditor(initial)
			})
			c.Return(result, ok)
			return nil
		},
	}, nil)
}
