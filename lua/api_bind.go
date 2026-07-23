package lua

import "github.com/mmcdole/rune/script"

// Key bindings are owned by the Lua bind module (30_binds.lua); Go is
// transport only. The Engine methods here bridge Session to the two
// internal Lua entry points.

// HandleKeyBind dispatches a pressed key to the Lua bind module.
// found=false (bind module unavailable, e.g. core failed to load) is
// ignored silently.
func (e *Engine) HandleKeyBind(key string) {
	if err := e.guard(func() error {
		_, _, callErr := e.vm.CallModule("rune.binds", "_dispatch", 0, key)
		return callErr
	}); err != nil {
		e.reportError("keybind '"+key+"'", err)
	}
}

// GetBoundKeys returns all bound key names, pulled from the Lua bind
// module. Session pushes these to the UI so it knows which keys to
// forward instead of feeding them to the input widget.
func (e *Engine) GetBoundKeys() []string {
	var keys []string
	err := e.guard(func() error {
		_, callErr := e.vm.CallModuleScoped("rune.binds", "_keys", 1,
			nil, func(vals []script.Value) error {
				tbl := vals[0].Table()
				if tbl == nil {
					return nil
				}
				tbl.Each(func(_, v script.Value) bool {
					keys = append(keys, v.String())
					return true
				})
				return nil
			})
		return callErr
	})
	if err != nil {
		e.reportError("bind key listing", err)
		return nil
	}
	return keys
}
