package lua

import (
	"github.com/mmcdole/rune/script"
	"github.com/mmcdole/rune/ui"
)

// registerPickerFuncs registers the rune._ui.picker_show primitive.
// The public rune.ui.picker.show API is defined in Lua (00_init.lua);
// opts parsing stays here because it marshals Lua tables into Go
// types for the UI.
func (e *Engine) registerPickerFuncs() {
	e.vm.RegisterModule("rune._ui", map[string]script.GoFunc{
		// rune._ui.picker_show(opts) - Show a picker overlay
		// opts = {
		//   title = "History",                  -- optional title (modal mode only)
		//   items = {"item1", "item2"} or {{text="...", value="...", desc="..."}},
		//   on_select = function(value) end     -- called with selected value
		//   mode = "inline"                     -- optional: "inline" or "modal" (default)
		//   match_description = true            -- optional: include description in fuzzy matching
		//   dismiss_on_space = true             -- optional (inline): close once input contains a space
		// }
		// Modal mode: picker captures keyboard and has its own search field.
		// Inline mode: user types in main input, picker filters based on input content.
		"picker_show": func(c *script.Call) error {
			opts := c.Table(1)

			title := ""
			if v := opts.Field("title"); !v.IsNil() {
				title = v.String()
			}
			inline := false
			if v := opts.Field("mode"); !v.IsNil() {
				inline = v.String() == "inline"
			}
			matchDesc := opts.Field("match_description").Truthy()
			dismissOnSpace := opts.Field("dismiss_on_space").Truthy()

			items := opts.Field("items").Table()
			if items == nil {
				return c.Errorf("picker: items must be a table")
			}

			// Pin on_select for execution when the selection lands.
			// Cleared on reload to prevent stale references.
			onSelect, ok := c.PinValue(opts.Field("on_select"))
			if !ok {
				return c.Errorf("picker: on_select must be a function")
			}
			callbackID := e.RegisterPickerCallback(onSelect)

			e.host.ShowPicker(ui.ShowPickerMsg{
				Title:          title,
				Items:          parsePickerItems(items, matchDesc),
				CallbackID:     callbackID,
				Inline:         inline,
				DismissOnSpace: dismissOnSpace,
			})
			return nil
		},
	}, nil)
}

// parsePickerItems parses a script table into []ui.PickerItem.
// Supports both simple strings and tables with text/value/desc fields.
func parsePickerItems(tbl script.TableView, matchDesc bool) []ui.PickerItem {
	var items []ui.PickerItem
	tbl.Each(func(k, v script.Value) bool {
		switch v.Kind() {
		case script.KindString:
			// Simple string: text and value are the same
			s := v.Str()
			items = append(items, ui.PickerItem{Text: s, Value: s, MatchDesc: matchDesc})
		case script.KindTable:
			t := v.Table()
			text := t.Field("text").String()
			value := t.Field("value").String()
			desc := ""
			if d := t.Field("desc"); !d.IsNil() {
				desc = d.String()
			}
			// Default value to text if not specified
			if value == "" {
				value = text
			}
			items = append(items, ui.PickerItem{Text: text, Description: desc, Value: value, MatchDesc: matchDesc})
		}
		return true
	})
	return items
}
