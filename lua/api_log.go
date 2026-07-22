package lua

import glua "github.com/mmcdole/rune/lua/luavm"

// registerLogFuncs registers rune._log.* primitives.
// The public rune.log API and the hooks that decide what gets written
// are defined in Lua (60_log.lua). Go only owns the file handle, so an
// active log survives /reload.
func (e *Engine) registerLogFuncs() {
	log := e.L.NewTable()
	e.L.SetField(e.runeTable, "_log", log)

	// rune._log.start(path): open a log file (append, parents created).
	// Returns the resolved path, or nil + error message.
	e.L.SetField(log, "start", e.L.NewFunction(func(L *glua.LState) int {
		path := L.CheckString(1)
		resolved, err := e.host.LogStart(path)
		if err != nil {
			L.Push(glua.LNil)
			L.Push(glua.LString(err.Error()))
			return 2
		}
		L.Push(glua.LString(resolved))
		return 1
	}))

	// rune._log.stop(): close the log. Returns true if one was open.
	e.L.SetField(log, "stop", e.L.NewFunction(func(L *glua.LState) int {
		L.Push(glua.LBool(e.host.LogStop()))
		return 1
	}))

	// rune._log.write(text): append one line. No-op when no log is open.
	e.L.SetField(log, "write", e.L.NewFunction(func(L *glua.LState) int {
		e.host.LogWrite(L.CheckString(1))
		return 0
	}))

	// rune._log.status(): returns the active log path, or nil.
	e.L.SetField(log, "status", e.L.NewFunction(func(L *glua.LState) int {
		if path, active := e.host.LogStatus(); active {
			L.Push(glua.LString(path))
		} else {
			L.Push(glua.LNil)
		}
		return 1
	}))
}
