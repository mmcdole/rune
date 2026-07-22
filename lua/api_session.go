package lua

import glua "github.com/mmcdole/rune/lua/luavm"

// registerSessionFuncs registers rune._session.* primitives.
// The public rune.session API is defined in Lua (00_init.lua).
func (e *Engine) registerSessionFuncs() {
	session := e.L.NewTable()
	e.L.SetField(e.runeTable, "_session", session)

	// rune._session.set(key, value): store a string
	e.L.SetField(session, "set", e.L.NewFunction(func(L *glua.LState) int {
		key := L.CheckString(1)
		value := L.CheckString(2)
		e.host.SessionSet(key, value)
		return 0
	}))

	// rune._session.get(key): returns the string, or nil if unset
	e.L.SetField(session, "get", e.L.NewFunction(func(L *glua.LState) int {
		key := L.CheckString(1)
		if v, ok := e.host.SessionGet(key); ok {
			L.Push(glua.LString(v))
		} else {
			L.Push(glua.LNil)
		}
		return 1
	}))

	// rune._session.delete(key): remove a key
	e.L.SetField(session, "delete", e.L.NewFunction(func(L *glua.LState) int {
		key := L.CheckString(1)
		e.host.SessionDelete(key)
		return 0
	}))
}
