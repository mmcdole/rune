package lua

import glua "github.com/yuin/gopher-lua"

// registerInputFuncs registers the rune.input API.
func (e *Engine) registerInputFuncs() {
	inp := e.L.NewTable()
	e.L.SetField(e.runeTable, "input", inp)

	// rune.input.set(text) - Set the input line content
	e.L.SetField(inp, "set", e.L.NewFunction(func(L *glua.LState) int {
		text := L.CheckString(1)
		e.host.SetInput(text)
		return 0
	}))
}
