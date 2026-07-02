package lua

import (
	glua "github.com/yuin/gopher-lua"

	"github.com/drake/rune/ui"
)

// Bar renderers are owned by the Lua bar module (18_bars.lua), which
// also applies the standard failure quarantine. Go's role is calling
// rune.bars._render_all on the tick and marshaling the result.

// registerBarFuncs registers layout/refresh primitives on rune.ui.
func (e *Engine) registerBarFuncs() {
	// Create rune.ui table if it doesn't exist
	uiTable := e.L.GetField(e.runeTable, "ui")
	if uiTable == glua.LNil {
		uiTable = e.L.NewTable()
		e.L.SetField(e.runeTable, "ui", uiTable)
	}
	ui := uiTable.(*glua.LTable)

	// rune.ui.refresh_bars() - Force immediate bar refresh
	// Use when bar state changes and you don't want to wait for the 250ms ticker
	e.L.SetField(ui, "refresh_bars", e.L.NewFunction(func(L *glua.LState) int {
		e.host.RefreshBars()
		return 0
	}))

	// rune.ui.layout(config) - Set the layout configuration
	// config = { top = {"bar1", {name="pane", height=10}}, bottom = {"input", "status"} }
	e.L.SetField(ui, "layout", e.L.NewFunction(func(L *glua.LState) int {
		cfg := L.CheckTable(1)

		// Parse top array
		topVal := L.GetField(cfg, "top")
		if topTbl, ok := topVal.(*glua.LTable); ok {
			e.barLayout.Top = parseLayoutArray(L, topTbl)
		} else {
			e.barLayout.Top = nil
		}

		// Parse bottom array
		bottomVal := L.GetField(cfg, "bottom")
		if bottomTbl, ok := bottomVal.(*glua.LTable); ok {
			e.barLayout.Bottom = parseLayoutArray(L, bottomTbl)
		} else {
			e.barLayout.Bottom = nil
		}

		e.host.OnConfigChange() // Notify Session to push layout update to UI
		return 0
	}))
}

// parseLayoutArray converts a Lua array table to LayoutEntry slice.
// Supports both strings ("name") and tables ({name="name", height=10}).
func parseLayoutArray(L *glua.LState, tbl *glua.LTable) []ui.LayoutEntry {
	var result []ui.LayoutEntry
	tbl.ForEach(func(k, v glua.LValue) {
		switch val := v.(type) {
		case glua.LString:
			// Simple string: "component_name"
			result = append(result, ui.LayoutEntry{Name: string(val)})
		case *glua.LTable:
			// Table: {name="component_name", height=10}
			entry := ui.LayoutEntry{}
			if name := L.GetField(val, "name"); name != glua.LNil {
				entry.Name = name.String()
			}
			if height := L.GetField(val, "height"); height != glua.LNil {
				if h, ok := height.(glua.LNumber); ok {
					entry.Height = int(h)
				}
			}
			if entry.Name != "" {
				result = append(result, entry)
			}
		}
	})
	return result
}

// RenderBars asks the Lua bar module to render every active bar at
// the given width. Returns nil when no bars produced content or the
// module is unavailable (degraded mode).
// Must be called from the Session goroutine (single Lua owner).
func (e *Engine) RenderBars(width int) map[string]ui.BarContent {
	if e.L == nil {
		return nil
	}
	render, ok := e.getRuneFunc("bars", "_render_all")
	if !ok {
		return nil
	}

	if err := e.guard(func() error {
		return e.L.CallByParam(glua.P{
			Fn:      render,
			NRet:    1,
			Protect: true,
		}, glua.LNumber(width))
	}); err != nil {
		e.reportError("bar render", err)
		return nil
	}

	ret := e.L.Get(-1)
	e.L.Pop(1)

	tbl, ok := ret.(*glua.LTable)
	if !ok {
		return nil
	}

	result := make(map[string]ui.BarContent)
	tbl.ForEach(func(k, v glua.LValue) {
		name := k.String()
		switch val := v.(type) {
		case glua.LString:
			result[name] = ui.BarContent{Left: string(val)}
		case *glua.LTable:
			result[name] = ui.BarContent{
				Left:   luaStringOrEmpty(e.L.GetField(val, "left")),
				Center: luaStringOrEmpty(e.L.GetField(val, "center")),
				Right:  luaStringOrEmpty(e.L.GetField(val, "right")),
			}
		}
	})
	if len(result) == 0 {
		return nil
	}
	return result
}

// GetLayout returns the current Lua-defined layout configuration.
func (e *Engine) GetLayout() ui.LayoutConfig {
	return e.barLayout
}

// luaStringOrEmpty returns the string value of a Lua value, or empty string if nil.
func luaStringOrEmpty(v glua.LValue) string {
	if v == glua.LNil {
		return ""
	}
	return v.String()
}
