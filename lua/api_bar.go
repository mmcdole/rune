package lua

import (
	glua "github.com/yuin/gopher-lua"

	"github.com/drake/rune/ui"
)

// registerBarFuncs registers the rune.ui.bar API.
func (e *Engine) registerBarFuncs() {
	// Create rune.ui table if it doesn't exist
	uiTable := e.L.GetField(e.runeTable, "ui")
	if uiTable == glua.LNil {
		uiTable = e.L.NewTable()
		e.L.SetField(e.runeTable, "ui", uiTable)
	}
	ui := uiTable.(*glua.LTable)

	// rune.ui.bar(name, render_fn) - Register a bar renderer
	// render_fn receives (width) and returns string or {left, center, right}
	e.L.SetField(ui, "bar", e.L.NewFunction(func(L *glua.LState) int {
		name := L.CheckString(1)
		fn := L.CheckFunction(2)
		e.barFuncs[name] = fn
		return 0
	}))

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

// RenderBar calls a Lua bar render function and returns the content.
// Called from Session on tick to update bar cache.
func (e *Engine) RenderBar(name string, width int) (ui.BarContent, bool) {
	if e.L == nil {
		return ui.BarContent{}, false
	}
	fn, ok := e.barFuncs[name]
	if !ok {
		return ui.BarContent{}, false
	}

	// Call the Lua function with width
	e.L.Push(fn)
	e.L.Push(glua.LNumber(width))
	if err := e.L.PCall(1, 1, nil); err != nil {
		e.CallHook("error", "bar render: "+err.Error())
		return ui.BarContent{}, false
	}

	result := e.L.Get(-1)
	e.L.Pop(1)

	// Handle return value - can be string or table {left, center, right}
	switch v := result.(type) {
	case glua.LString:
		return ui.BarContent{Left: string(v)}, true
	case *glua.LTable:
		return ui.BarContent{
			Left:   luaStringOrEmpty(e.L.GetField(v, "left")),
			Center: luaStringOrEmpty(e.L.GetField(v, "center")),
			Right:  luaStringOrEmpty(e.L.GetField(v, "right")),
		}, true
	default:
		return ui.BarContent{}, false
	}
}

// GetBarNames returns the names of all registered bars.
func (e *Engine) GetBarNames() []string {
	names := make([]string, 0, len(e.barFuncs))
	for name := range e.barFuncs {
		names = append(names, name)
	}
	return names
}

// HasBar returns true if a bar with the given name is registered.
func (e *Engine) HasBar(name string) bool {
	_, ok := e.barFuncs[name]
	return ok
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
