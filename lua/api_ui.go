package lua

import "github.com/mmcdole/rune/script"

// registerUIFuncs registers all UI-related API functions
func (e *Engine) registerUIFuncs() {
	e.registerPaneFuncs()
	e.registerUIInternalFuncs()
}

// registerUIInternalFuncs registers rune._ui.* primitives used by the
// core modules (not part of the public API).
func (e *Engine) registerUIInternalFuncs() {
	e.vm.RegisterModule("rune._ui", map[string]script.GoFunc{
		// rune._ui.config_changed(): notify the host that binds/layout
		// changed so it can push fresh state to the UI.
		"config_changed": func(c *script.Call) error {
			e.host.OnConfigChange()
			return nil
		},

		// rune._ui.set_clipboard(text): ask the terminal to set the
		// system clipboard (OSC 52).
		"set_clipboard": func(c *script.Call) error {
			e.host.ClipboardSet(c.Str(1))
			return nil
		},
	}, nil)
}

// registerPaneFuncs registers internal rune._pane.* primitives (wrapped by Lua)
func (e *Engine) registerPaneFuncs() {
	e.vm.RegisterModule("rune._pane", map[string]script.GoFunc{
		// rune._pane.create(name): Create a named pane
		"create": func(c *script.Call) error {
			name := c.Str(1)
			e.host.PaneCreate(name)
			return nil
		},

		// rune._pane.write(name, text): Write to a pane
		"write": func(c *script.Call) error {
			name := c.Str(1)
			text := c.Str(2)
			e.host.PaneWrite(name, text)
			return nil
		},

		// rune._pane.toggle(name): Toggle pane visibility
		"toggle": func(c *script.Call) error {
			name := c.Str(1)
			e.host.PaneToggle(name)
			return nil
		},

		// rune._pane.set_visible(name, visible): Show or hide a pane
		"set_visible": func(c *script.Call) error {
			name := c.Str(1)
			visible := c.Bool(2)
			e.host.PaneSetVisible(name, visible)
			return nil
		},

		// rune._pane.clear(name): Clear pane contents
		"clear": func(c *script.Call) error {
			name := c.Str(1)
			e.host.PaneClear(name)
			return nil
		},

		// rune._pane.scroll_up(name, lines): Scroll pane up
		"scroll_up": func(c *script.Call) error {
			name := c.Str(1)
			lines := c.OptInt(2, 1)
			e.host.PaneScrollUp(name, lines)
			return nil
		},

		// rune._pane.scroll_down(name, lines): Scroll pane down
		"scroll_down": func(c *script.Call) error {
			name := c.Str(1)
			lines := c.OptInt(2, 1)
			e.host.PaneScrollDown(name, lines)
			return nil
		},

		// rune._pane.scroll_to_top(name): Scroll pane to top
		"scroll_to_top": func(c *script.Call) error {
			name := c.Str(1)
			e.host.PaneScrollToTop(name)
			return nil
		},

		// rune._pane.scroll_to_bottom(name): Scroll pane to bottom
		"scroll_to_bottom": func(c *script.Call) error {
			name := c.Str(1)
			e.host.PaneScrollToBottom(name)
			return nil
		},
	}, nil)
}
