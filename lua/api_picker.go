package lua

import (
	"github.com/mmcdole/rune/ui"
	glua "github.com/yuin/gopher-lua"
)

// registerPickerFuncs registers the rune._ui.picker_show primitive.
// The public rune.ui.picker.show API is defined in Lua (00_init.lua);
// opts parsing stays here because it marshals Lua tables into Go
// types for the UI.
func (e *Engine) registerPickerFuncs() {
	internal := e.L.GetField(e.runeTable, "_ui").(*glua.LTable)

	// rune._ui.picker_show(opts) - Show a picker overlay
	// opts = {
	//   title = "History",                  -- optional title (modal mode only)
	//   items = {"item1", "item2"} or {{text="...", value="...", desc="..."}},
	//   on_select = function(value) end     -- called with selected value
	//   mode = "inline"                     -- optional: "inline" or "modal" (default)
	//   match_description = true            -- optional: include description in fuzzy matching
	// }
	// Modal mode: picker captures keyboard and has its own search field.
	// Inline mode: user types in main input, picker filters based on input content.
	e.L.SetField(internal, "picker_show", e.L.NewFunction(func(L *glua.LState) int {
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

		// Register callback in Engine (cleared on reload to prevent stale references)
		callbackID := e.RegisterPickerCallback(onSelectFn)

		// Call host to show the picker
		e.host.ShowPicker(title, items, callbackID, inline)
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
