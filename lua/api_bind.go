package lua

import glua "github.com/yuin/gopher-lua"

// bindRegistry holds registered Lua key bindings.
type bindRegistry struct {
	binds map[string]*glua.LFunction
}

func newBindRegistry() *bindRegistry {
	return &bindRegistry{
		binds: make(map[string]*glua.LFunction),
	}
}

// registerBindFuncs registers the rune.bind API.
func (e *Engine) registerBindFuncs() {
	// rune.bind(key, callback) - Register a key binding
	// key is a string like "ctrl+r", "ctrl+t", "f1", etc.
	// callback receives no arguments
	e.L.SetField(e.runeTable, "bind", e.L.NewFunction(func(L *glua.LState) int {
		key := L.CheckString(1)
		fn := L.CheckFunction(2)
		e.binds.binds[key] = fn
		return 0
	}))

	// rune.unbind(key) - Remove a key binding
	e.L.SetField(e.runeTable, "unbind", e.L.NewFunction(func(L *glua.LState) int {
		key := L.CheckString(1)
		delete(e.binds.binds, key)
		return 0
	}))
}

// HandleKeyBind checks if a key has a Lua binding and executes it.
// Returns true if the key was handled by Lua.
func (e *Engine) HandleKeyBind(key string) bool {
	fn, ok := e.binds.binds[key]
	if !ok {
		return false
	}

	// Execute the callback
	e.L.Push(fn)
	if err := e.L.PCall(0, 0, nil); err != nil {
		e.CallHook("error", "keybind: "+err.Error())
	}
	return true
}

// GetBoundKeys returns all bound key names.
func (e *Engine) GetBoundKeys() []string {
	keys := make([]string, 0, len(e.binds.binds))
	for key := range e.binds.binds {
		keys = append(keys, key)
	}
	return keys
}
