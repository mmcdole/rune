package lua

import glua "github.com/yuin/gopher-lua"

// registerPersistFuncs registers rune._persist.* primitives.
// The public rune.persist API is defined in Lua (00_init.lua).
func (e *Engine) registerPersistFuncs() {
	persist := e.L.NewTable()
	e.L.SetField(e.runeTable, "_persist", persist)

	// rune._persist.set(key, value): store a string
	e.L.SetField(persist, "set", e.L.NewFunction(func(L *glua.LState) int {
		key := L.CheckString(1)
		value := L.CheckString(2)
		e.host.PersistSet(key, value)
		return 0
	}))

	// rune._persist.get(key): returns the string, or nil if unset
	e.L.SetField(persist, "get", e.L.NewFunction(func(L *glua.LState) int {
		key := L.CheckString(1)
		if v, ok := e.host.PersistGet(key); ok {
			L.Push(glua.LString(v))
		} else {
			L.Push(glua.LNil)
		}
		return 1
	}))

	// rune._persist.delete(key): remove a key
	e.L.SetField(persist, "delete", e.L.NewFunction(func(L *glua.LState) int {
		key := L.CheckString(1)
		e.host.PersistDelete(key)
		return 0
	}))
}
