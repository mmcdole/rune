package lua

import glua "github.com/yuin/gopher-lua"

// registerPickerFuncs registers the rune.ui.picker API.
func (e *Engine) registerPickerFuncs() {
	// Ensure rune.ui table exists
	uiTable := e.L.GetField(e.runeTable, "ui")
	if uiTable == glua.LNil {
		uiTable = e.L.NewTable()
		e.L.SetField(e.runeTable, "ui", uiTable)
	}
	ui := uiTable.(*glua.LTable)

	// Create rune.ui.picker namespace
	picker := e.L.NewTable()
	e.L.SetField(ui, "picker", picker)

	// rune.ui.picker.show(opts) - Show a picker overlay
	// opts = {
	//   title = "History",                -- optional title
	//   items = {"item1", "item2"} or {{text="...", value="...", desc="..."}},
	//   on_select = function(value) end   -- called with selected value
	//   filter_prefix = "/"               -- optional: enables "linked" mode
	// }
	// In linked mode, the picker filters based on input line content minus the prefix.
	// Keys pass through to the input field instead of being trapped by the picker.
	e.L.SetField(picker, "show", e.L.NewFunction(func(L *glua.LState) int {
		opts := L.CheckTable(1)

		// Parse title (optional)
		title := ""
		if titleVal := L.GetField(opts, "title"); titleVal != glua.LNil {
			title = titleVal.String()
		}

		// Parse filter_prefix (optional - enables linked mode)
		filterPrefix := ""
		if prefixVal := L.GetField(opts, "filter_prefix"); prefixVal != glua.LNil {
			filterPrefix = prefixVal.String()
		}

		// Parse items
		itemsVal := L.GetField(opts, "items")
		itemsTbl, ok := itemsVal.(*glua.LTable)
		if !ok {
			L.RaiseError("picker: items must be a table")
			return 0
		}
		items := parsePickerItems(L, itemsTbl)

		// Parse on_select callback
		onSelectVal := L.GetField(opts, "on_select")
		onSelectFn, ok := onSelectVal.(*glua.LFunction)
		if !ok {
			L.RaiseError("picker: on_select must be a function")
			return 0
		}

		// Create Go callback that will invoke the Lua function
		// This is called synchronously from Session when UI returns selection
		onSelect := func(value string) {
			L.Push(onSelectFn)
			L.Push(glua.LString(value))
			if err := L.PCall(1, 0, nil); err != nil {
				e.CallHook("error", "picker callback: "+err.Error())
			}
		}

		// Call host to show the picker
		e.host.ShowPicker(title, items, onSelect, filterPrefix)
		return 0
	}))
}

// parsePickerItems parses a Lua table into []PickerItem.
// Supports both simple strings and tables with text/value/desc fields.
func parsePickerItems(L *glua.LState, tbl *glua.LTable) []PickerItem {
	var items []PickerItem
	tbl.ForEach(func(k, v glua.LValue) {
		switch item := v.(type) {
		case glua.LString:
			// Simple string: text and value are the same
			s := string(item)
			items = append(items, PickerItem{Text: s, Value: s})
		case *glua.LTable:
			// Table with text, value, desc fields
			text := L.GetField(item, "text").String()
			value := L.GetField(item, "value").String()
			desc := ""
			if descVal := L.GetField(item, "desc"); descVal != glua.LNil {
				desc = descVal.String()
			}
			// Default value to text if not specified
			if value == "" {
				value = text
			}
			items = append(items, PickerItem{Text: text, Description: desc, Value: value})
		}
	})
	return items
}
