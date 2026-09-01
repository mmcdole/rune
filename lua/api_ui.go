package lua

import (
	"math"

	"github.com/mmcdole/rune/script"
	"github.com/mmcdole/rune/ui"
)

// registerUIFuncs registers all UI-related API functions
func (e *Engine) registerUIFuncs() {
	e.registerPaneFuncs()
	e.registerUIInternalFuncs()
}

// registerUIInternalFuncs registers rune._ui.* primitives used by the
// core modules (not part of the public API).
func (e *Engine) registerUIInternalFuncs() {
	e.vm.RegisterModule("rune._ui", map[string]script.GoFunc{
		// rune._ui.presentation_changed(): notify the host that binds/layout
		// changed so it can push fresh state to the UI.
		"presentation_changed": func(c *script.Call) error {
			e.host.OnPresentationChange()
			return nil
		},

		// rune._ui.set_clipboard(text): ask the terminal to set the
		// system clipboard (OSC 52).
		"set_clipboard": func(c *script.Call) error {
			e.host.ClipboardSet(c.Str(1))
			return nil
		},

		// rune._ui.search_show(opts): open the scrollback-search
		// overlay. opts = { query = "..." } (optional; empty keeps the
		// previous search's query). Fire-and-forget - unlike the picker
		// there is no callback: the search is self-contained in the UI.
		"search_show": func(c *script.Call) error {
			query := ""
			if c.NArgs() >= 1 && c.Arg(1).Kind() == script.KindTable {
				query = c.Arg(1).Table().Field("query").Str()
			}
			e.host.ShowSearch(ui.ShowSearchMsg{Query: query})
			return nil
		},
	}, nil)
}

// registerPaneFuncs registers internal rune._pane.* primitives (wrapped by Lua)
func (e *Engine) registerPaneFuncs() {
	e.vm.RegisterModule("rune._pane", map[string]script.GoFunc{
		// rune._pane.create(name): Create a named pane
		"create": func(c *script.Call) error {
			name, err := paneName(c, "create")
			if err != nil {
				return err
			}
			e.host.PaneCreate(name)
			return nil
		},

		// rune._pane.write(name, text): Write to a pane
		"write": func(c *script.Call) error {
			name, err := paneName(c, "write")
			if err != nil {
				return err
			}
			text := c.Str(2)
			e.host.PaneWrite(name, text)
			return nil
		},

		// rune._pane.replace(name, text): Replace the pane's contents in one
		// UI update.
		"replace": func(c *script.Call) error {
			name, err := paneName(c, "replace")
			if err != nil {
				return err
			}
			e.host.PaneReplace(name, c.Str(2))
			return nil
		},

		// rune._pane.toggle(name): Toggle the pane's placement in the active
		// layout. Returns whether the layout places the pane at all.
		"toggle": func(c *script.Call) error {
			name, err := paneName(c, "toggle")
			if err != nil {
				return err
			}
			visible, found := e.layout.PaneVisible(name)
			if !found {
				c.Return(false)
				return nil
			}
			updated, _, changed := e.layout.WithPaneVisibility(name, !visible)
			e.layout = updated
			c.Return(true)
			if changed {
				e.host.OnPresentationChange()
			}
			return nil
		},

		// rune._pane.show(name): Show the pane's placement in the active layout.
		"show": func(c *script.Call) error {
			return e.setPaneVisible(c, "show", true)
		},

		// rune._pane.hide(name): Hide the pane's placement in the active layout.
		"hide": func(c *script.Call) error {
			return e.setPaneVisible(c, "hide", false)
		},

		// rune._pane.is_visible(name): Report the pane placement's own gate,
		// or nil when the layout does not place the pane.
		"is_visible": func(c *script.Call) error {
			name, err := paneName(c, "is_visible")
			if err != nil {
				return err
			}
			visible, found := e.layout.PaneVisible(name)
			if !found {
				c.Return(nil)
				return nil
			}
			c.Return(visible)
			return nil
		},

		// rune._pane.clear(name): Clear pane contents
		"clear": func(c *script.Call) error {
			name, err := paneName(c, "clear")
			if err != nil {
				return err
			}
			e.host.PaneClear(name)
			return nil
		},

		// rune._pane.scroll_up(name, lines): Scroll pane up
		"scroll_up": func(c *script.Call) error {
			name, err := paneName(c, "scroll_up")
			if err != nil {
				return err
			}
			lines, err := paneScrollLines(c, "scroll_up")
			if err != nil {
				return err
			}
			e.host.PaneScrollUp(name, lines)
			return nil
		},

		// rune._pane.scroll_down(name, lines): Scroll pane down
		"scroll_down": func(c *script.Call) error {
			name, err := paneName(c, "scroll_down")
			if err != nil {
				return err
			}
			lines, err := paneScrollLines(c, "scroll_down")
			if err != nil {
				return err
			}
			e.host.PaneScrollDown(name, lines)
			return nil
		},

		// rune._pane.scroll_to_top(name): Scroll pane to top
		"scroll_to_top": func(c *script.Call) error {
			name, err := paneName(c, "scroll_to_top")
			if err != nil {
				return err
			}
			e.host.PaneScrollToTop(name)
			return nil
		},

		// rune._pane.scroll_to_bottom(name): Scroll pane to bottom
		"scroll_to_bottom": func(c *script.Call) error {
			name, err := paneName(c, "scroll_to_bottom")
			if err != nil {
				return err
			}
			e.host.PaneScrollToBottom(name)
			return nil
		},
	}, nil)
}

func paneName(c *script.Call, operation string) (string, error) {
	value := c.Arg(1)
	if value.Kind() != script.KindString || value.Str() == "" {
		return "", c.Errorf("rune.pane.%s: name must be a non-empty string", operation)
	}
	return value.Str(), nil
}

func paneScrollLines(c *script.Call, operation string) (int, error) {
	if c.NArgs() < 2 || c.Arg(2).Kind() == script.KindNil {
		return 1, nil
	}
	value := c.Arg(2)
	if value.Kind() != script.KindNumber {
		return 0, c.Errorf("rune.pane.%s: lines must be a positive integer", operation)
	}
	n := value.Num()
	maxInt := int(^uint(0) >> 1)
	if math.IsNaN(n) || math.IsInf(n, 0) || math.Trunc(n) != n ||
		n < 1 || n > float64(maxInt) {
		return 0, c.Errorf("rune.pane.%s: lines must be a positive integer", operation)
	}
	return int(n), nil
}
