package lua

import glua "github.com/yuin/gopher-lua"

// registerHistoryFuncs registers the rune.history API.
func (e *Engine) registerHistoryFuncs() {
	hist := e.L.NewTable()
	e.L.SetField(e.runeTable, "history", hist)

	// rune.history.get() - Returns array of input history strings
	e.L.SetField(hist, "get", e.L.NewFunction(func(L *glua.LState) int {
		history := e.host.GetHistory()
		tbl := L.NewTable()
		for i, cmd := range history {
			tbl.RawSetInt(i+1, glua.LString(cmd))
		}
		L.Push(tbl)
		return 1
	}))

	// rune.history.add(cmd) - Add a command to history
	e.L.SetField(hist, "add", e.L.NewFunction(func(L *glua.LState) int {
		cmd := L.CheckString(1)
		e.host.AddToHistory(cmd)
		return 0
	}))
}
