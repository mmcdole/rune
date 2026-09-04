package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/mmcdole/rune/ui"
)

// ShowPicker enters the requested picker mode and records the callback
// to settle when the picker closes.
func (c *inputController) ShowPicker(opts ui.ShowPickerMsg) {
	// Completion/history pickers are single-line concepts. If a script
	// asks for one while a structured draft is active, settle its callback
	// immediately instead of layering conflicting input modes.
	if c.input.IsComposing() {
		c.notify(ui.PickerSelectMsg{CallbackID: opts.CallbackID, Accepted: false})
		return
	}
	// Picker and search overlays are mutually exclusive; the newcomer
	// wins, the open search settles as cancelled.
	if c.mode() == modeSearch {
		c.closeSearch(false)
	}
	if c.mode() == modePickerModal || c.mode() == modePickerInline {
		c.closePicker(false, "")
	}

	c.pickerCB = opts.CallbackID
	c.pickerDismiss = opts.DismissOnSpace
	c.input.ShowPicker(opts)
}

// closePicker is the single exit path from either picker mode: resets
// the mode, hides the overlay, and settles the Lua callback - fired
// when accepted, cancelled otherwise. Every shown picker must end here
// or its callback is stranded on the session side.
func (c *inputController) closePicker(accepted bool, value string) {
	c.input.HidePicker()
	c.notify(ui.PickerSelectMsg{CallbackID: c.pickerCB, Value: value, Accepted: accepted})
	c.pickerCB = ""
	c.pickerDismiss = false
}

// inlinePickerLocalKeys are navigation keys the inline picker handles
// itself instead of forwarding to Lua binds.
var inlinePickerLocalKeys = map[string]bool{
	"up":    true,
	"down":  true,
	"tab":   true,
	"enter": true,
}

func (c *inputController) handleInlineKey(msg tea.KeyPressMsg) {
	// Ctrl+Enter transitions from single-line completion into a structured
	// draft. Treat it like a bracketed newline paste so the input update is
	// visible to Lua before the picker callback is cancelled.
	if isComposerNewline(msg) {
		c.handlePaste("\n")
		return
	}
	// An exact physical numpad bind wins in the inline picker. Without one,
	// the NumLock-off form becomes the local navigation key engraved on it.
	if info, ok := numpadNavigation(msg); ok {
		if key := keyToString(msg); key != "" && c.isBound(key) {
			c.notify(ui.ExecuteBindMsg(key))
			return
		}
		msg = info.navigationFallback(msg)
	}
	keyStr := keyToString(msg)
	// Don't send picker navigation keys to Lua - handle them locally
	if keyStr != "" && c.isBound(keyStr) && !inlinePickerLocalKeys[keyStr] {
		c.notify(ui.ExecuteBindMsg(keyStr))
		return
	}

	switch {
	case matchesKey(msg, tea.KeyUp, 0):
		c.input.Picker().SelectUp()
		return

	case matchesKey(msg, tea.KeyDown, 0):
		c.input.Picker().SelectDown()
		return

	case matchesKey(msg, tea.KeyTab, 0):
		if item, ok := c.input.Picker().Selected(); ok {
			c.input.SetValue(item.GetValue() + " ")
			c.input.CursorEnd()
			// Report the completed text before the selection fires so
			// the session's input state is fresh inside the callback.
			c.notify(ui.InputChangedMsg{Text: c.input.Value(), Cursor: c.input.Position()})
			c.closePicker(true, item.GetValue())
		} else {
			c.closePicker(false, "")
		}
		return

	case matchesEnterKey(msg, 0):
		if item, ok := c.input.Picker().Selected(); ok {
			c.closePicker(true, item.GetValue())
		} else {
			c.closePicker(false, "")
		}
		c.submitInput()
		return
	}

	if c.scroll(msg) {
		return
	}

	if c.forwardToInput(msg) {
		c.syncInlineFilter()
	}
}

func (c *inputController) handleModalKey(msg tea.KeyPressMsg) {
	if info, ok := numpadNavigation(msg); ok {
		msg = info.navigationFallback(msg)
	}
	switch {
	case matchesKey(msg, tea.KeyUp, 0):
		c.input.Picker().SelectUp()

	case matchesKey(msg, tea.KeyDown, 0):
		c.input.Picker().SelectDown()

	case matchesEnterKey(msg, 0), matchesKey(msg, tea.KeyTab, 0):
		if item, ok := c.input.Picker().Selected(); ok {
			c.closePicker(true, item.GetValue())
		} else {
			c.closePicker(false, "")
		}

	case matchesKey(msg, tea.KeyBackspace, 0):
		query := []rune(c.input.Picker().Query())
		if len(query) > 0 {
			c.input.Picker().Filter(string(query[:len(query)-1]))
		}

	default:
		if msg.Text != "" {
			c.input.Picker().Filter(c.input.Picker().Query() + msg.Text)
		}
	}
}

// syncInlineFilter re-filters the inline picker after the input
// changed; closes it when the input is cleared, or - for pickers that
// opted in via dismiss_on_space - once the user types a space and moves
// on to arguments.
func (c *inputController) syncInlineFilter() {
	val := c.input.Value()
	if val == "" || (c.pickerDismiss && strings.ContainsRune(val, ' ')) {
		c.closePicker(false, "")
		return
	}
	c.input.Picker().Filter(c.input.Value())
}
