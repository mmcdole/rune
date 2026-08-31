package lua

import (
	"github.com/mmcdole/rune/script"
	"github.com/mmcdole/rune/ui"
)

// Bar renderers are owned by the Lua bar module (35_bars.lua), which
// also applies the standard failure quarantine. Go's role is calling
// rune.bars._render_all on the tick and marshaling the result.

// registerBarFuncs registers rune._ui.refresh_bars. Its public wrapper is
// defined in 00_init.lua.
func (e *Engine) registerBarFuncs() {
	e.vm.RegisterModule("rune._ui", map[string]script.GoFunc{
		// rune._ui.refresh_bars() - Force immediate bar refresh
		// Use when bar state changes and you don't want to wait for the 250ms ticker
		"refresh_bars": func(c *script.Call) error {
			e.host.RefreshBars()
			return nil
		},
	}, nil)
}

// RenderBars asks the Lua bar module to render every active bar at the given
// width. A non-nil empty map is a successful empty snapshot; nil means the
// render pass failed and callers should retain their last good snapshot.
// Must be called from the Session goroutine (single Lua owner).
func (e *Engine) RenderBars(width int) map[string]ui.BarContent {
	result := make(map[string]ui.BarContent)
	err := e.guard(func() error {
		_, callErr := e.vm.CallModuleScoped("rune.bars", "_render_all", 1,
			[]any{width}, func(vals []script.Value) error {
				tbl := vals[0].Table()
				if tbl == nil {
					return nil
				}
				tbl.Each(func(k, v script.Value) bool {
					name := k.Str()
					switch v.Kind() {
					case script.KindString:
						result[name] = ui.BarContent{Left: v.Str()}
					case script.KindTable:
						t := v.Table()
						result[name] = ui.BarContent{
							Left:   t.Field("left").Str(),
							Center: t.Field("center").Str(),
							Right:  t.Field("right").Str(),
						}
					}
					return true
				})
				return nil
			})
		return callErr
	})
	if err != nil {
		e.reportError("bar render", err)
		return nil
	}
	return result
}
