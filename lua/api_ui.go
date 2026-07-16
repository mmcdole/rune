package lua

import glua "github.com/yuin/gopher-lua"

// registerUIFuncs registers all UI-related API functions
func (e *Engine) registerUIFuncs() {
	e.registerPaneFuncs()
	e.registerUIInternalFuncs()
}

// registerUIInternalFuncs registers rune._ui.* primitives used by the
// core modules (not part of the public API).
func (e *Engine) registerUIInternalFuncs() {
	internal := e.L.NewTable()
	e.L.SetField(e.runeTable, "_ui", internal)

	// rune._ui.config_changed(): notify the host that binds/layout
	// changed so it can push fresh state to the UI.
	e.L.SetField(internal, "config_changed", e.L.NewFunction(func(L *glua.LState) int {
		e.host.OnConfigChange()
		return 0
	}))

	// rune._ui.set_clipboard(text): ask the terminal to set the
	// system clipboard (OSC 52).
	e.L.SetField(internal, "set_clipboard", e.L.NewFunction(func(L *glua.LState) int {
		e.host.ClipboardSet(L.CheckString(1))
		return 0
	}))
}

// registerPaneFuncs registers internal rune._pane.* primitives (wrapped by Lua)
func (e *Engine) registerPaneFuncs() {
	paneTable := e.L.NewTable()
	e.L.SetField(e.runeTable, "_pane", paneTable)

	// rune._pane.create(name): Create a named pane
	e.L.SetField(paneTable, "create", e.L.NewFunction(func(L *glua.LState) int {
		name := L.CheckString(1)
		e.host.PaneCreate(name)
		return 0
	}))

	// rune._pane.write(name, text): Write to a pane
	e.L.SetField(paneTable, "write", e.L.NewFunction(func(L *glua.LState) int {
		name := L.CheckString(1)
		text := L.CheckString(2)
		e.host.PaneWrite(name, text)
		return 0
	}))

	// rune._pane.toggle(name): Toggle pane visibility
	e.L.SetField(paneTable, "toggle", e.L.NewFunction(func(L *glua.LState) int {
		name := L.CheckString(1)
		e.host.PaneToggle(name)
		return 0
	}))

	// rune._pane.set_visible(name, visible): Show or hide a pane
	e.L.SetField(paneTable, "set_visible", e.L.NewFunction(func(L *glua.LState) int {
		name := L.CheckString(1)
		visible := L.CheckBool(2)
		e.host.PaneSetVisible(name, visible)
		return 0
	}))

	// rune._pane.clear(name): Clear pane contents
	e.L.SetField(paneTable, "clear", e.L.NewFunction(func(L *glua.LState) int {
		name := L.CheckString(1)
		e.host.PaneClear(name)
		return 0
	}))

	// rune._pane.scroll_up(name, lines): Scroll pane up
	e.L.SetField(paneTable, "scroll_up", e.L.NewFunction(func(L *glua.LState) int {
		name := L.CheckString(1)
		lines := L.OptInt(2, 1)
		e.host.PaneScrollUp(name, lines)
		return 0
	}))

	// rune._pane.scroll_down(name, lines): Scroll pane down
	e.L.SetField(paneTable, "scroll_down", e.L.NewFunction(func(L *glua.LState) int {
		name := L.CheckString(1)
		lines := L.OptInt(2, 1)
		e.host.PaneScrollDown(name, lines)
		return 0
	}))

	// rune._pane.scroll_to_top(name): Scroll pane to top
	e.L.SetField(paneTable, "scroll_to_top", e.L.NewFunction(func(L *glua.LState) int {
		name := L.CheckString(1)
		e.host.PaneScrollToTop(name)
		return 0
	}))

	// rune._pane.scroll_to_bottom(name): Scroll pane to bottom
	e.L.SetField(paneTable, "scroll_to_bottom", e.L.NewFunction(func(L *glua.LState) int {
		name := L.CheckString(1)
		e.host.PaneScrollToBottom(name)
		return 0
	}))
}
