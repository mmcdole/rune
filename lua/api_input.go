package lua

import glua "github.com/yuin/gopher-lua"

// registerInputFuncs registers rune._input.* primitives.
// The public rune.input API is defined in Lua (90_input.lua).
func (e *Engine) registerInputFuncs() {
	inp := e.L.NewTable()
	e.L.SetField(e.runeTable, "_input", inp)

	e.L.SetField(inp, "get", e.L.NewFunction(func(L *glua.LState) int {
		L.Push(glua.LString(e.host.GetInput()))
		return 1
	}))

	e.L.SetField(inp, "set", e.L.NewFunction(func(L *glua.LState) int {
		text := L.CheckString(1)
		e.host.SetInput(text)
		return 0
	}))

	// Cursor primitives
	e.L.SetField(inp, "get_cursor", e.L.NewFunction(func(L *glua.LState) int {
		L.Push(glua.LNumber(e.host.InputGetCursor()))
		return 1
	}))

	e.L.SetField(inp, "set_cursor", e.L.NewFunction(func(L *glua.LState) int {
		pos := L.CheckInt(1)
		e.host.InputSetCursor(pos)
		return 0
	}))

	// Editor mode primitive. The host call blocks in $EDITOR for as
	// long as the user edits, so it runs outside the watchdog deadline.
	e.L.SetField(inp, "open_editor", e.L.NewFunction(func(L *glua.LState) int {
		initial := L.OptString(1, "")
		var result string
		var ok bool
		e.pauseWatchdog(func() {
			result, ok = e.host.OpenEditor(initial)
		})
		L.Push(glua.LString(result))
		L.Push(glua.LBool(ok))
		return 2
	}))
}
