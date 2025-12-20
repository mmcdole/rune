package lua

import glua "github.com/yuin/gopher-lua"

func (e *Engine) registerInputFuncs() {
	inp := e.L.NewTable()
	e.L.SetField(e.runeTable, "input", inp)

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

	// Ghost text primitive - Go just renders, Lua is source of truth
	e.L.SetField(inp, "set_ghost", e.L.NewFunction(func(L *glua.LState) int {
		text := ""
		if L.Get(1) != glua.LNil {
			text = L.CheckString(1)
		}
		e.host.SetGhost(text)
		return 0
	}))

	// Editor mode primitive
	e.L.SetField(inp, "open_editor", e.L.NewFunction(func(L *glua.LState) int {
		initial := L.OptString(1, "")
		result, ok := e.host.OpenEditor(initial)
		L.Push(glua.LString(result))
		L.Push(glua.LBool(ok))
		return 2
	}))
}
