package lua

import glua "github.com/yuin/gopher-lua"

// registerCoreFuncs registers internal rune._* primitives (wrapped by Lua)
func (e *Engine) registerCoreFuncs() {
	// rune._send_raw(text): Bypasses alias processing, writes directly to socket
	e.L.SetField(e.runeTable, "_send_raw", e.L.NewFunction(func(L *glua.LState) int {
		cmd := L.CheckString(1)
		if err := e.host.Send(cmd); err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		return 0
	}))

	// rune._echo(text): Outputs text to the local game window
	e.L.SetField(e.runeTable, "_echo", e.L.NewFunction(func(L *glua.LState) int {
		msg := L.CheckString(1)
		e.host.Print(msg)
		return 0
	}))

	// rune._quit(): Exit the client
	e.L.SetField(e.runeTable, "_quit", e.L.NewFunction(func(L *glua.LState) int {
		e.host.Quit()
		return 0
	}))

	// rune._connect(address): Connect to server
	e.L.SetField(e.runeTable, "_connect", e.L.NewFunction(func(L *glua.LState) int {
		addr := L.CheckString(1)
		e.host.Connect(addr)
		return 0
	}))

	// rune._disconnect(): Disconnect from server
	e.L.SetField(e.runeTable, "_disconnect", e.L.NewFunction(func(L *glua.LState) int {
		e.host.Disconnect()
		return 0
	}))

	// rune._reload(): Reload all scripts
	e.L.SetField(e.runeTable, "_reload", e.L.NewFunction(func(L *glua.LState) int {
		e.host.Reload()
		return 0
	}))

	// rune._load(path): Load a Lua script (runs immediately, no round-trip)
	e.L.SetField(e.runeTable, "_load", e.L.NewFunction(func(L *glua.LState) int {
		path := L.CheckString(1)
		if err := e.DoFile(path); err != nil {
			L.Push(glua.LString(err.Error()))
			return 1
		}
		e.CallHook("loaded", path)
		return 0
	}))
}
