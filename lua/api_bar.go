package lua

import glua "github.com/yuin/gopher-lua"

// BarData represents the raw strings returned by a Lua bar renderer.
// Pure data with no UI dependencies - Session converts this to layout.BarContent.
type BarData struct {
	Left   string
	Center string
	Right  string
}

// LayoutConfig holds the current layout configuration set by Lua.
type LayoutConfig struct {
	Top    []string
	Bottom []string
}

// barRegistry holds registered Lua bar render functions and layout config.
type barRegistry struct {
	funcs  map[string]*glua.LFunction
	layout LayoutConfig
}

func newBarRegistry() *barRegistry {
	return &barRegistry{
		funcs: make(map[string]*glua.LFunction),
		layout: LayoutConfig{
			Bottom: []string{"input", "status"}, // Default layout
		},
	}
}

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
		e.bars.funcs[name] = fn
		return 0
	}))

	// rune.ui.layout(config) - Set the layout configuration
	// config = { top = {"bar1"}, bottom = {"input", "status"} }
	e.L.SetField(ui, "layout", e.L.NewFunction(func(L *glua.LState) int {
		cfg := L.CheckTable(1)

		// Parse top array
		topVal := L.GetField(cfg, "top")
		if topTbl, ok := topVal.(*glua.LTable); ok {
			e.bars.layout.Top = tableToStrings(L, topTbl)
		} else {
			e.bars.layout.Top = nil
		}

		// Parse bottom array
		bottomVal := L.GetField(cfg, "bottom")
		if bottomTbl, ok := bottomVal.(*glua.LTable); ok {
			e.bars.layout.Bottom = tableToStrings(L, bottomTbl)
		} else {
			e.bars.layout.Bottom = nil
		}

		e.host.OnConfigChange() // Notify Session to push layout update to UI
		return 0
	}))
}

// tableToStrings converts a Lua array table to a Go string slice.
func tableToStrings(L *glua.LState, tbl *glua.LTable) []string {
	var result []string
	tbl.ForEach(func(k, v glua.LValue) {
		if s, ok := v.(glua.LString); ok {
			result = append(result, string(s))
		}
	})
	return result
}

// RenderBar calls a Lua bar render function and returns the content.
// Called from Session on tick to update bar cache.
func (e *Engine) RenderBar(name string, width int) (BarData, bool) {
	fn, ok := e.bars.funcs[name]
	if !ok {
		return BarData{}, false
	}

	// Call the Lua function with width
	e.L.Push(fn)
	e.L.Push(glua.LNumber(width))
	if err := e.L.PCall(1, 1, nil); err != nil {
		e.CallHook("error", "bar render: "+err.Error())
		return BarData{}, false
	}

	result := e.L.Get(-1)
	e.L.Pop(1)

	// Handle return value - can be string or table {left, center, right}
	switch v := result.(type) {
	case glua.LString:
		return BarData{Left: string(v)}, true
	case *glua.LTable:
		return BarData{
			Left:   luaStringOrEmpty(e.L.GetField(v, "left")),
			Center: luaStringOrEmpty(e.L.GetField(v, "center")),
			Right:  luaStringOrEmpty(e.L.GetField(v, "right")),
		}, true
	default:
		return BarData{}, false
	}
}

// GetBarNames returns the names of all registered bars.
func (e *Engine) GetBarNames() []string {
	names := make([]string, 0, len(e.bars.funcs))
	for name := range e.bars.funcs {
		names = append(names, name)
	}
	return names
}

// HasBar returns true if a bar with the given name is registered.
func (e *Engine) HasBar(name string) bool {
	_, ok := e.bars.funcs[name]
	return ok
}

// GetLayout returns the current Lua-defined layout configuration.
func (e *Engine) GetLayout() LayoutConfig {
	return e.bars.layout
}

// luaStringOrEmpty returns the string value of a Lua value, or empty string if nil.
func luaStringOrEmpty(v glua.LValue) string {
	if v == glua.LNil {
		return ""
	}
	return v.String()
}
