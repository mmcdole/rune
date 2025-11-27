package lua

import glua "github.com/yuin/gopher-lua"

// registerUIFuncs registers all UI-related API functions
func (e *Engine) registerUIFuncs() {
	e.registerStatusFuncs()
	e.registerPaneFuncs()
	e.registerInfobarFuncs()
}

// registerStatusFuncs registers internal rune._status.* primitives (wrapped by Lua)
func (e *Engine) registerStatusFuncs() {
	statusTable := e.L.NewTable()
	e.L.SetField(e.runeTable, "_status", statusTable)

	// rune._status.set(text): Set the status bar text
	e.L.SetField(statusTable, "set", e.L.NewFunction(func(L *glua.LState) int {
		text := L.CheckString(1)
		e.host.SetStatus(text)
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
		e.host.Pane("create", name, "")
		return 0
	}))

	// rune._pane.write(name, text): Write to a pane
	e.L.SetField(paneTable, "write", e.L.NewFunction(func(L *glua.LState) int {
		name := L.CheckString(1)
		text := L.CheckString(2)
		e.host.Pane("write", name, text)
		return 0
	}))

	// rune._pane.toggle(name): Toggle pane visibility
	e.L.SetField(paneTable, "toggle", e.L.NewFunction(func(L *glua.LState) int {
		name := L.CheckString(1)
		e.host.Pane("toggle", name, "")
		return 0
	}))

	// rune._pane.clear(name): Clear pane contents
	e.L.SetField(paneTable, "clear", e.L.NewFunction(func(L *glua.LState) int {
		name := L.CheckString(1)
		e.host.Pane("clear", name, "")
		return 0
	}))

	// rune._pane.bind(key, name): Bind key to toggle pane
	e.L.SetField(paneTable, "bind", e.L.NewFunction(func(L *glua.LState) int {
		key := L.CheckString(1)
		name := L.CheckString(2)
		e.host.Pane("bind", name, key)
		return 0
	}))
}

// registerInfobarFuncs registers internal rune._infobar.* primitives (wrapped by Lua)
func (e *Engine) registerInfobarFuncs() {
	infobarTable := e.L.NewTable()
	e.L.SetField(e.runeTable, "_infobar", infobarTable)

	// rune._infobar.set(text): Set the info bar from Lua
	e.L.SetField(infobarTable, "set", e.L.NewFunction(func(L *glua.LState) int {
		text := L.CheckString(1)
		e.host.SetInfobar(text)
		return 0
	}))
}
