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
}
