package lua

import (
	"github.com/mmcdole/rune/script"
	"github.com/mmcdole/rune/ui"
)

// registerPickerFuncs registers the rune._ui.picker_show primitive.
// The public rune.ui.picker.show API is defined in Lua (00_init.lua),
// which keeps each on_select callback in a pending table under the id
// returned here. Opts parsing stays in Go because it marshals Lua
// tables into Go types for the UI.
func (e *Engine) registerPickerFuncs() {
	e.vm.RegisterModule("rune._ui", map[string]script.GoFunc{
		// rune._ui.picker_show(opts) -> id - Show a picker overlay; its
		// result settles the callback stored under the returned id.
		// opts = {
		//   title = "History",                  -- optional title (modal mode only)
		//   items = {"item1", "item2"} or {{text="...", value="...", desc="..."}},
		//   mode = "inline"                     -- optional: "inline" or "modal" (default)
		//   match_description = true            -- optional: include description in fuzzy matching
		//   dismiss_on_space = true             -- optional (inline): close once input contains a space
		// }
		// Modal mode: picker captures keyboard and has its own search field.
		// Inline mode: user types in main input, picker filters based on input content.
		"picker_show": func(c *script.Call) error {
			opts := c.Table(1)

			title := opts.Field("title").Str()
			inline := opts.Field("mode").Str() == "inline"
			matchDesc := opts.Field("match_description").Truthy()
			dismissOnSpace := opts.Field("dismiss_on_space").Truthy()

			items := opts.Field("items").Table()
			if items == nil {
				return c.Errorf("picker: items must be a table")
			}

			e.pickerNextID++
			callbackID := e.pickerNextID
			e.host.ShowPicker(ui.PickerOptions{
				Title:          title,
				Items:          parsePickerItems(items, matchDesc),
				CallbackID:     callbackID,
				Inline:         inline,
				DismissOnSpace: dismissOnSpace,
			})
			c.Return(callbackID)
			return nil
		},
	}, nil)
}

// OnPickerResult settles the Lua callback for a closed picker: on_select
// runs with value when accepted, and the callback is dropped otherwise.
func (e *Engine) OnPickerResult(id int, value string, accepted bool) {
	if err := e.guard(func() error {
		// found=false means the picker module is unavailable (core failed
		// to load); the result is dropped.
		_, _, err := e.vm.CallModule("rune.ui.picker", "_settle", 0, id, accepted, value)
		return err
	}); err != nil {
		e.reportError("picker callback", err)
	}
}

// parsePickerItems parses a script table into []ui.PickerItem.
// Supports both simple strings and tables with text/value/desc fields.
// Item order is semantic, so read the array part by index (Each
// visits in engine-defined order).
func parsePickerItems(tbl script.TableView, matchDesc bool) []ui.PickerItem {
	var items []ui.PickerItem
	for i := 1; i <= tbl.Len(); i++ {
		v := tbl.Index(i)
		switch v.Kind() {
		case script.KindString:
			// Simple string: text and value are the same
			s := v.Str()
			items = append(items, ui.PickerItem{Text: s, Value: s, MatchDesc: matchDesc})
		case script.KindTable:
			t := v.Table()
			text := t.Field("text").Str()
			value := t.Field("value").Str()
			desc := t.Field("desc").Str()
			// Default value to text if not specified
			if value == "" {
				value = text
			}
			items = append(items, ui.PickerItem{Text: text, Description: desc, Value: value, MatchDesc: matchDesc})
		}
	}
	return items
}
