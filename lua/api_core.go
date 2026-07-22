package lua

import (
	"github.com/mmcdole/rune/text"
	glua "github.com/mmcdole/rune/lua/luavm"
)

// registerCoreFuncs registers internal rune._* primitives (wrapped by Lua).
//
// Convention: recoverable runtime failures (not connected, file not
// found) return nil + error message rather than raising, matching
// rune._regex.compile. Raising is reserved for programmer errors like
// wrong argument types (L.Check*).
func (e *Engine) registerCoreFuncs() {
	// rune._send_raw(text): Bypasses alias processing, writes directly to socket.
	// Returns true, or nil + error message.
	e.L.SetField(e.runeTable, "_send_raw", e.L.NewFunction(func(L *glua.LState) int {
		cmd := L.CheckString(1)
		if err := e.host.Send(cmd); err != nil {
			L.Push(glua.LNil)
			L.Push(glua.LString(err.Error()))
			return 2
		}
		L.Push(glua.LTrue)
		return 1
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

	// rune._strip_ansi(text): Remove ANSI escape sequences.
	// Used by rune.line.new so Lua-built line objects get the same
	// clean text as lines arriving from the server.
	e.L.SetField(e.runeTable, "_strip_ansi", e.L.NewFunction(func(L *glua.LState) int {
		s := L.CheckString(1)
		L.Push(glua.LString(text.StripANSI(s)))
		return 1
	}))

	// rune._load(path): Load a Lua script (runs immediately, no round-trip).
	// Returns true, or nil + error message.
	e.L.SetField(e.runeTable, "_load", e.L.NewFunction(func(L *glua.LState) int {
		path := L.CheckString(1)
		if err := e.DoFile(path); err != nil {
			L.Push(glua.LNil)
			L.Push(glua.LString(err.Error()))
			return 2
		}
		e.CallHook("loaded", path)
		L.Push(glua.LTrue)
		return 1
	}))
}
