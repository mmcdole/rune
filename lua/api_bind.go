package lua

import glua "github.com/yuin/gopher-lua"

// Key bindings are owned by the Lua bind module (30_binds.lua); Go is
// transport only. The Engine methods here bridge Session to the two
// internal Lua entry points.

// HandleKeyBind dispatches a pressed key to the Lua bind module.
func (e *Engine) HandleKeyBind(key string) {
	if e.L == nil {
		return
	}
	dispatch, ok := e.getRuneFunc("binds", "_dispatch")
	if !ok {
		return // Bind module unavailable (core failed to load)
	}

	if err := e.guard(func() error {
		return e.L.CallByParam(glua.P{
			Fn:      dispatch,
			NRet:    0,
			Protect: true,
		}, glua.LString(key))
	}); err != nil {
		e.reportError("keybind '"+key+"'", err)
	}
}

// GetBoundKeys returns all bound key names, pulled from the Lua bind
// module. Session pushes these to the UI so it knows which keys to
// forward instead of feeding them to the input widget.
func (e *Engine) GetBoundKeys() []string {
	if e.L == nil {
		return nil
	}
	keysFn, ok := e.getRuneFunc("binds", "_keys")
	if !ok {
		return nil
	}

	if err := e.guard(func() error {
		return e.L.CallByParam(glua.P{
			Fn:      keysFn,
			NRet:    1,
			Protect: true,
		})
	}); err != nil {
		e.reportError("bind key listing", err)
		return nil
	}

	ret := e.L.Get(-1)
	e.L.Pop(1)

	tbl, ok := ret.(*glua.LTable)
	if !ok {
		return nil
	}
	var keys []string
	tbl.ForEach(func(_, v glua.LValue) {
		keys = append(keys, v.String())
	})
	return keys
}
