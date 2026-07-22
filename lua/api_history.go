package lua

import glua "github.com/mmcdole/rune/lua/luavm"

// registerHistoryFuncs registers rune._history.* primitives.
// The public rune.history API is defined in Lua (00_init.lua).
func (e *Engine) registerHistoryFuncs() {
	hist := e.L.NewTable()
	e.L.SetField(e.runeTable, "_history", hist)

	// rune._history.get() - Returns array of input history strings
	e.L.SetField(hist, "get", e.L.NewFunction(func(L *glua.LState) int {
		history := e.host.GetHistory()
		tbl := L.NewTable()
		for i, cmd := range history {
			tbl.RawSetInt(i+1, glua.LString(cmd))
		}
		L.Push(tbl)
		return 1
	}))

	// rune._history.entries() - Returns structured history, oldest first.
	// Mode is a stable string so Lua does not depend on Go enum values.
	e.L.SetField(hist, "entries", e.L.NewFunction(func(L *glua.LState) int {
		history := e.host.GetHistoryEntries()
		tbl := L.NewTable()
		for i, entry := range history {
			item := L.NewTable()
			item.RawSetString("text", glua.LString(entry.Text))
			item.RawSetString("mode", glua.LString(entry.Mode.String()))
			tbl.RawSetInt(i+1, item)
		}
		L.Push(tbl)
		return 1
	}))

	// rune._history.add(cmd) - Add a command to history
	e.L.SetField(hist, "add", e.L.NewFunction(func(L *glua.LState) int {
		cmd := L.CheckString(1)
		e.host.AddToHistory(cmd)
		return 0
	}))
}
