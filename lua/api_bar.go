package lua

import (
	"github.com/mmcdole/rune/script"
	"github.com/mmcdole/rune/ui"
)

// Bar renderers are owned by the Lua bar module (35_bars.lua), which
// also applies the standard failure quarantine. Go's role is calling
// rune.bars._render_all on the tick and marshaling the result.

// registerBarFuncs registers layout/refresh primitives on rune._ui.
// The public rune.ui wrappers are defined in Lua (00_init.lua).
func (e *Engine) registerBarFuncs() {
	e.vm.RegisterModule("rune._ui", map[string]script.GoFunc{
		// rune._ui.refresh_bars() - Force immediate bar refresh
		// Use when bar state changes and you don't want to wait for the 250ms ticker
		"refresh_bars": func(c *script.Call) error {
			e.host.RefreshBars()
			return nil
		},

		// rune._ui.layout(config) - Set the layout configuration
		// config = { top = {"bar1", {name="pane", height=10}}, bottom = {"input", "status"} }
		"layout": func(c *script.Call) error {
			cfg := c.Table(1)

			if top := cfg.Field("top").Table(); top != nil {
				e.barLayout.Top = parseLayoutArray(top)
			} else {
				e.barLayout.Top = nil
			}
			if bottom := cfg.Field("bottom").Table(); bottom != nil {
				e.barLayout.Bottom = parseLayoutArray(bottom)
			} else {
				e.barLayout.Bottom = nil
			}

			e.host.OnConfigChange() // Notify Session to push layout update to UI
			return nil
		},
	}, nil)
}

// parseLayoutArray converts a script array table to LayoutEntry slice.
// Supports both strings ("name") and tables ({name="name", height=10}).
func parseLayoutArray(tbl script.TableView) []ui.LayoutEntry {
	var result []ui.LayoutEntry
	tbl.Each(func(k, v script.Value) bool {
		switch v.Kind() {
		case script.KindString:
			// Simple string: "component_name"
			result = append(result, ui.LayoutEntry{Name: v.Str()})
		case script.KindTable:
			// Table: {name="component_name", height=10}
			t := v.Table()
			entry := ui.LayoutEntry{Name: t.Field("name").String()}
			if entry.Name == "nil" {
				entry.Name = ""
			}
			if height := t.Field("height"); height.Kind() == script.KindNumber {
				entry.Height = int(height.Num())
			}
			if entry.Name != "" {
				result = append(result, entry)
			}
		}
		return true
	})
	return result
}

// RenderBars asks the Lua bar module to render every active bar at
// the given width. Returns nil when no bars produced content or the
// module is unavailable (degraded mode).
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
					name := k.String()
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
	if len(result) == 0 {
		return nil
	}
	return result
}

// GetLayout returns the current Lua-defined layout configuration.
func (e *Engine) GetLayout() ui.LayoutConfig {
	return e.barLayout
}
