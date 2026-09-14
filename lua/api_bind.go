package lua

import (
	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/script"
)

// Lua owns binding registration. Session pulls a snapshot for local editor
// actions and forwards callback key presses through HandleKeyBind.

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

// GetBindings snapshots the registry for local action routing and callback transport.
// Nil means unavailable; an empty non-nil map means all keys were unbound.
func (e *Engine) GetBindings() input.Bindings {
	var bindings input.Bindings
	err := e.guard(func() error {
		_, callErr := e.vm.CallModuleScoped("rune.binds", "_bindings", 1,
			nil, func(vals []script.Value) error {
				tbl := vals[0].Table()
				if tbl == nil {
					return nil
				}
				bindings = input.Bindings{}
				tbl.Each(func(k, v script.Value) bool {
					row := v.Table()
					if row != nil {
						bindings[k.Str()] = input.Binding{
							Action:  row.Field("action").Str(),
							Enabled: row.Field("enabled").Bool(),
							Order:   int(row.Field("order").Num()),
						}
					}
					return true
				})
				return nil
			})
		return callErr
	})
	if err != nil {
		e.reportError("bind listing", err)
		return nil
	}
	return bindings
}
