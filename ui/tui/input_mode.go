package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/widget"
)

// InputMode represents the current input handling mode.
type InputMode int

const (
	ModeNormal       InputMode = iota // Standard text input
	ModePickerModal                   // Modal picker traps all keys
	ModePickerInline                  // Inline picker filters based on input
)

// inputController owns the input-mode state machine: the current mode,
// the active picker's Lua callback, and the invariant that every path
// out of a picker mode resets the mode, hides the overlay, and settles
// the callback (exactly one PickerSelectMsg per shown picker). It is
// also the single place that reports input text changes to the session,
// so the session's tracked input (rune.input.get) can never go stale.
type inputController struct {
	input *widget.Input
	mode  InputMode

	// Active picker state. This lives here rather than on the Input
	// widget: the widget is pure view, the callback contract with the
	// session is the controller's.
	pickerCB      string // Lua callback ID to settle on close
	pickerDismiss bool   // close inline picker once input contains a space

	notify  func(ui.UIEvent)       // outbound events to the session
	submit  func(line string)      // deliver a submitted input line
	isBound func(key string) bool  // key has a Lua bind
	scroll  func(tea.KeyType) bool // Go scroll-key fallback; true if handled
}

func newInputController(
	input *widget.Input,
	notify func(ui.UIEvent),
	submit func(string),
	isBound func(string) bool,
	scroll func(tea.KeyType) bool,
) *inputController {
	return &inputController{
		input:   input,
		notify:  notify,
		submit:  submit,
		isBound: isBound,
		scroll:  scroll,
	}
}

// HandleKey routes key presses.
//
// Key policy: Go owns keys only while a UI-internal mode is active
// (picker capture/cancel) plus Enter-to-submit; all other editing and
// navigation policy lives in Lua binds. In normal mode a bound
// non-printable key always goes to Lua; a bound printable key goes to
// Lua only when the input is empty (so "j" can be a hotkey without
// breaking typing). Unbound scroll keys fall back to Go so scrollback
// stays usable even in degraded mode.
func (c *inputController) HandleKey(msg tea.KeyMsg) {
	// Picker modes capture Ctrl+C/Esc as "cancel". In normal mode they
	// fall through so the Lua binds decide (clear input, double-tap
	// quit, ...).
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		if c.mode != ModeNormal {
			c.closePicker(false, "")
			return
		}
	}

	switch c.mode {
	case ModePickerModal:
		c.handleModalKey(msg)
	case ModePickerInline:
		c.handleInlineKey(msg)
	default:
		c.handleNormalKey(msg)
	}
}

// ShowPicker enters the requested picker mode and records the callback
// to settle when the picker closes.
func (c *inputController) ShowPicker(opts ui.ShowPickerMsg) {
	if opts.Inline {
		c.mode = ModePickerInline
	} else {
		c.mode = ModePickerModal
	}
	c.pickerCB = opts.CallbackID
	c.pickerDismiss = opts.DismissOnSpace
	c.input.ShowPicker(opts)
}

// SetText replaces the input content (rune.input.set). Lua editing
// binds (ctrl+u, ctrl+w) change input while the inline picker is open;
// keep its filter in sync, and close the picker (cancelling its
// callback) when the input is cleared.
func (c *inputController) SetText(text string) {
	c.input.SetValue(text)
	c.input.CursorEnd()
	c.notify(ui.InputChangedMsg{Text: text, Cursor: c.input.Position()})
	if c.mode == ModePickerInline {
		c.syncInlineFilter()
	}
}

func (c *inputController) handleNormalKey(msg tea.KeyMsg) {
	keyStr := keyToString(msg)
	if keyStr != "" && c.isBound(keyStr) {
		// Alt-modified runes are chords, not typing: they never reach
		// the input widget, so the empty-input guard doesn't apply.
		isPrintable := msg.Type == tea.KeyRunes && !msg.Alt
		if !isPrintable || c.input.Value() == "" {
			c.notify(ui.ExecuteBindMsg(keyStr))
			return
		}
	}

	if msg.Type == tea.KeyEnter {
		c.submitInput()
		return
	}

	// Unbound scroll keys: Go fallback (keeps degraded mode scrollable)
	if c.scroll(msg.Type) {
		return
	}

	c.forwardToInput(msg)
}

// inlinePickerLocalKeys are navigation keys the inline picker handles
// itself instead of forwarding to Lua binds.
var inlinePickerLocalKeys = map[string]bool{
	"up":   true,
	"down": true,
	"tab":  true,
}

func (c *inputController) handleInlineKey(msg tea.KeyMsg) {
	keyStr := keyToString(msg)
	// Don't send picker navigation keys to Lua - handle them locally
	if keyStr != "" && c.isBound(keyStr) && !inlinePickerLocalKeys[keyStr] {
		c.notify(ui.ExecuteBindMsg(keyStr))
		return
	}

	switch msg.Type {
	case tea.KeyUp:
		c.input.PickerSelectUp()
		return

	case tea.KeyDown:
		c.input.PickerSelectDown()
		return

	case tea.KeyTab:
		if item, ok := c.input.PickerSelected(); ok {
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

	case tea.KeyEnter:
		if item, ok := c.input.PickerSelected(); ok {
			c.closePicker(true, item.GetValue())
		} else {
			c.closePicker(false, "")
		}
		c.submitInput()
		return
	}

	if c.scroll(msg.Type) {
		return
	}

	if c.forwardToInput(msg) {
		c.syncInlineFilter()
	}
}

func (c *inputController) handleModalKey(msg tea.KeyMsg) {
	switch msg.Type {
	case tea.KeyUp:
		c.input.PickerSelectUp()

	case tea.KeyDown:
		c.input.PickerSelectDown()

	case tea.KeyEnter, tea.KeyTab:
		if item, ok := c.input.PickerSelected(); ok {
			c.closePicker(true, item.GetValue())
		} else {
			c.closePicker(false, "")
		}

	case tea.KeyRunes:
		c.input.PickerFilter(c.input.PickerQuery() + string(msg.Runes))

	case tea.KeySpace:
		c.input.PickerFilter(c.input.PickerQuery() + " ")

	case tea.KeyBackspace:
		query := []rune(c.input.PickerQuery())
		if len(query) > 0 {
			c.input.PickerFilter(string(query[:len(query)-1]))
		}
	}
}

// forwardToInput passes an editing key to the text input and reports
// the resulting text or cursor change to the session. Returns true if
// the text changed.
func (c *inputController) forwardToInput(msg tea.KeyMsg) bool {
	oldValue := c.input.Value()
	oldCursor := c.input.Position()
	c.input.UpdateTextInput(msg)

	newValue := c.input.Value()
	newCursor := c.input.Position()
	if newValue != oldValue {
		c.notify(ui.InputChangedMsg{Text: newValue, Cursor: newCursor})
		return true
	}
	if newCursor != oldCursor {
		c.notify(ui.CursorMovedMsg{Cursor: newCursor})
	}
	return false
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
	c.input.UpdatePickerFilter()
}

// closePicker is the single exit path from either picker mode: resets
// the mode, hides the overlay, and settles the Lua callback - fired
// when accepted, cancelled otherwise. Every shown picker must end here
// or its callback is stranded on the session side.
func (c *inputController) closePicker(accepted bool, value string) {
	c.mode = ModeNormal
	c.input.HidePicker()
	c.notify(ui.PickerSelectMsg{CallbackID: c.pickerCB, Value: value, Accepted: accepted})
	c.pickerCB = ""
	c.pickerDismiss = false
}

// submitInput delivers the current input line and clears it, reporting
// the cleared state so the session's tracked input cannot go stale.
func (c *inputController) submitInput() {
	c.submit(c.input.Value())
	c.input.Reset()
	c.notify(ui.InputChangedMsg{Text: "", Cursor: 0})
}
