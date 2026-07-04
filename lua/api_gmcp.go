package lua

import (
	"encoding/json"

	glua "github.com/yuin/gopher-lua"
)

// registerGMCPFuncs registers rune._gmcp.* primitives.
// The public rune.gmcp API (handlers, subscriptions, the Core.Hello
// handshake) is defined in Lua (70_gmcp.lua). Encoding goes through
// the shared JSON bridge (api_store.go).
func (e *Engine) registerGMCPFuncs() {
	gmcp := e.L.NewTable()
	e.L.SetField(e.runeTable, "_gmcp", gmcp)

	// rune._gmcp.send(package, value?): value may be a string, number,
	// boolean, or JSON-able table; nil sends the bare package name.
	// Returns true, or nil + error message (not connected, GMCP not
	// negotiated, unencodable value).
	e.L.SetField(gmcp, "send", e.L.NewFunction(func(L *glua.LState) int {
		pkg := L.CheckString(1)
		value := L.Get(2)

		data := ""
		if value != glua.LNil {
			gv, err := luaToGo(value, make(map[*glua.LTable]bool), 0)
			if err != nil {
				L.Push(glua.LNil)
				L.Push(glua.LString(err.Error()))
				return 2
			}
			raw, err := json.Marshal(gv)
			if err != nil {
				L.Push(glua.LNil)
				L.Push(glua.LString(err.Error()))
				return 2
			}
			data = string(raw)
		}

		if err := e.host.GMCPSend(pkg, data); err != nil {
			L.Push(glua.LNil)
			L.Push(glua.LString(err.Error()))
			return 2
		}
		L.Push(glua.LTrue)
		return 1
	}))

	// rune._gmcp.send_raw(package, json?): sends the JSON text
	// verbatim (no validation) - the debugging escape hatch used by
	// "/gmcp send". Returns true, or nil + error message.
	e.L.SetField(gmcp, "send_raw", e.L.NewFunction(func(L *glua.LState) int {
		pkg := L.CheckString(1)
		data := L.OptString(2, "")
		if err := e.host.GMCPSend(pkg, data); err != nil {
			L.Push(glua.LNil)
			L.Push(glua.LString(err.Error()))
			return 2
		}
		L.Push(glua.LTrue)
		return 1
	}))
}
