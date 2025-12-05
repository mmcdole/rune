package lua

import (
	"github.com/drake/rune/ui"
	glua "github.com/yuin/gopher-lua"
)

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
	//   title = "History",                  -- optional title (modal mode only)
	//   items = {"item1", "item2"} or {{text="...", value="...", desc="..."}},
	//   on_select = function(value) end     -- called with selected value
	//   mode = "inline"                     -- optional: "inline" or "modal" (default)
	//   match_description = true            -- optional: include description in fuzzy matching
	// }
	// Modal mode: picker captures keyboard and has its own search field.
	// Inline mode: user types in main input, picker filters based on input content.
	e.L.SetField(picker, "show", e.L.NewFunction(func(L *glua.LState) int {
		opts := L.CheckTable(1)

		// Parse title (optional - used in modal mode header)
		title := ""
		if titleVal := L.GetField(opts, "title"); titleVal != glua.LNil {
			title = titleVal.String()
		}

		// Parse mode (optional - "inline" or "modal", default "modal")
		inline := false
		if modeVal := L.GetField(opts, "mode"); modeVal != glua.LNil {
			inline = modeVal.String() == "inline"
		}

		// Parse match_description (optional - include description in fuzzy matching)
		matchDesc := false
		if mdVal := L.GetField(opts, "match_description"); mdVal != glua.LNil {
			matchDesc = glua.LVAsBool(mdVal)
		}

		// Parse items
		itemsVal := L.GetField(opts, "items")
		itemsTbl, ok := itemsVal.(*glua.LTable)
		if !ok {
			L.RaiseError("picker: items must be a table")
			return 0
		}
		items := parsePickerItems(L, itemsTbl, matchDesc)

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
		e.host.ShowPicker(title, items, onSelect, inline)
		return 0
	}))
}

// parsePickerItems parses a Lua table into []ui.PickerItem.
// Supports both simple strings and tables with text/value/desc fields.
func parsePickerItems(L *glua.LState, tbl *glua.LTable, matchDesc bool) []ui.PickerItem {
	var items []ui.PickerItem
	tbl.ForEach(func(k, v glua.LValue) {
		switch item := v.(type) {
		case glua.LString:
			// Simple string: text and value are the same
			s := string(item)
			items = append(items, ui.PickerItem{Text: s, Value: s, MatchDesc: matchDesc})
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
			items = append(items, ui.PickerItem{Text: text, Description: desc, Value: value, MatchDesc: matchDesc})
		}
	})
	return items
}
