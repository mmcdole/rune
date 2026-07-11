package tui

import (
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/widget"
)

// InputMode represents the current input handling mode.
type InputMode int

const (
	ModeNormal       InputMode = iota // Standard text input
	ModeCompose                       // Lossless structured-text input
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
	historyRecall bool   // unmodified verbatim entry restored from history

	notify  func(ui.UIEvent)            // outbound events to the session
	submit  func(input.Submission) bool // transfer an immutable draft to the session
	isBound func(key string) bool       // key has a Lua bind
	scroll  func(tea.KeyType) bool      // Go scroll-key fallback; true if handled
}

func newInputController(
	input *widget.Input,
	notify func(ui.UIEvent),
	submit func(input.Submission) bool,
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
// Key policy: Go owns editing mechanics while a UI-internal mode is active
// (picker capture/cancel and lossless composer editing), plus paste safety and
// Enter-to-submit. Application actions remain Lua binds. In normal mode a bound
// non-printable key always goes to Lua; a bound printable key goes to
// Lua only when the input is empty (so "j" can be a hotkey without
// breaking typing). Unbound scroll keys fall back to Go so scrollback
// stays usable even in degraded mode.
func (c *inputController) HandleKey(msg tea.KeyMsg) {
	// Bracketed paste arrives atomically. Intercept it before binding
	// dispatch so even a one-character paste can never fire a printable
	// hotkey, and so structured text never passes through textinput's
	// newline/tab sanitizer.
	if msg.Paste && c.mode != ModePickerModal {
		c.handlePaste(msg)
		return
	}
	if c.mode == ModeCompose && msg.Type != tea.KeyEsc {
		c.input.ContinueCompose()
	}

	// Picker modes capture Ctrl+C/Esc as "cancel". In normal mode they
	// fall through so the Lua binds decide (clear input, double-tap quit,
	// ...). Compose mode owns Escape because it is an internal cancel.
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		if c.mode == ModePickerModal || c.mode == ModePickerInline {
			c.closePicker(false, "")
			return
		}
		if c.mode == ModeCompose && msg.Type == tea.KeyEsc {
			if c.input.ConfirmDiscard() {
				c.cancelCompose()
			}
			return
		}
	}

	switch c.mode {
	case ModeCompose:
		c.handleComposeKey(msg)
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
	// Completion/history pickers are single-line concepts. If a script
	// asks for one while a structured draft is active, settle its callback
	// immediately instead of layering conflicting input modes.
	if c.input.IsComposing() {
		c.notify(ui.PickerSelectMsg{CallbackID: opts.CallbackID, Accepted: false})
		return
	}
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
	c.historyRecall = false
	wasPicker := c.mode == ModePickerInline || c.mode == ModePickerModal
	wasInline := c.mode == ModePickerInline
	c.input.SetValue(text)
	c.input.CursorEnd()
	c.notify(ui.InputChangedMsg{Text: c.input.Value(), Cursor: c.input.Position()})

	if c.input.IsComposing() {
		if wasPicker {
			c.closePicker(false, "")
		}
		c.mode = ModeCompose
		return
	}
	if wasInline {
		c.syncInlineFilter()
		return
	}
	if !wasPicker {
		c.mode = ModeNormal
	}
}

// SetSubmission restores a history entry with explicit interpretation.
// Unlike SetText, an explicit command entry exits sticky compose mode, while
// verbatim is forced even for one safe, non-empty physical line.
func (c *inputController) SetSubmission(submission input.Submission) {
	wasPicker := c.mode == ModePickerInline || c.mode == ModePickerModal
	c.historyRecall = submission.Mode == input.ModeVerbatim

	if submission.Mode == input.ModeVerbatim {
		c.input.BeginCompose(submission.Text, utf8.RuneCountInString(submission.Text))
	} else {
		// Reset first so sticky compose state cannot reinterpret a recalled
		// command entry that happens to have identical text.
		c.input.Reset()
		c.input.SetValue(submission.Text)
		c.input.CursorEnd()
	}
	c.notify(ui.InputChangedMsg{Text: c.input.Value(), Cursor: c.input.Position()})

	if wasPicker {
		c.closePicker(false, "")
	}
	if c.input.IsComposing() {
		c.mode = ModeCompose
	} else {
		c.mode = ModeNormal
	}
}

func (c *inputController) handleNormalKey(msg tea.KeyMsg) {
	// In Rune's terminal stack Ctrl+Enter is delivered as Ctrl+J. It is
	// the explicit way to start a multiline draft without pasting.
	if msg.Type == tea.KeyCtrlJ {
		c.insertComposerText("\n")
		return
	}

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

func (c *inputController) handleComposeKey(msg tea.KeyMsg) {
	if msg.Type == tea.KeyEnter && !msg.Alt {
		c.submitInput()
		return
	}
	if c.historyRecall {
		key := ""
		delta := 0
		switch msg.Type {
		case tea.KeyUp:
			key, delta = "up", -1
		case tea.KeyDown:
			key, delta = "down", 1
		}
		if key != "" && !c.input.CanMoveComposerVertically(delta) && c.isBound(key) {
			c.notify(ui.ExecuteBindMsg(key))
			return
		}
	}

	oldValue := c.input.Value()
	oldCursor := c.input.Position()
	if c.input.UpdateComposer(msg) {
		if c.reportInputUpdate(oldValue, oldCursor) {
			c.historyRecall = false
		}
		if !c.input.IsComposing() {
			c.mode = ModeNormal
		}
		return
	}

	// Non-editing chords remain scriptable in compose mode. In
	// particular, Ctrl+E keeps using the existing external-editor bind.
	if keyStr := keyToString(msg); keyStr != "" && c.isBound(keyStr) {
		c.notify(ui.ExecuteBindMsg(keyStr))
	}
}

func (c *inputController) handlePaste(msg tea.KeyMsg) {
	c.historyRecall = false
	oldValue := c.input.Value()
	oldCursor := c.input.Position()
	wasInline := c.mode == ModePickerInline

	c.input.InsertPaste(string(msg.Runes))
	c.reportInputUpdate(oldValue, oldCursor)

	if c.input.IsComposing() {
		if wasInline {
			// InputChangedMsg is deliberately emitted before the callback so
			// Lua observes the newly pasted draft when cancellation runs.
			c.closePicker(false, "")
		}
		c.mode = ModeCompose
		return
	}
	if wasInline {
		c.syncInlineFilter()
	}
}

func (c *inputController) insertComposerText(text string) {
	c.historyRecall = false
	oldValue := c.input.Value()
	oldCursor := c.input.Position()
	c.input.InsertPaste(text)
	c.mode = ModeCompose
	c.reportInputUpdate(oldValue, oldCursor)
}

func (c *inputController) cancelCompose() {
	c.historyRecall = false
	c.input.Reset()
	c.mode = ModeNormal
	c.notify(ui.InputChangedMsg{Text: "", Cursor: 0})
}

// inlinePickerLocalKeys are navigation keys the inline picker handles
// itself instead of forwarding to Lua binds.
var inlinePickerLocalKeys = map[string]bool{
	"up":   true,
	"down": true,
	"tab":  true,
}

func (c *inputController) handleInlineKey(msg tea.KeyMsg) {
	// Ctrl+Enter transitions from single-line completion into a structured
	// draft. Treat it like a bracketed newline paste so the input update is
	// visible to Lua before the picker callback is cancelled.
	if msg.Type == tea.KeyCtrlJ {
		c.handlePaste(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'\n'}, Paste: true})
		return
	}

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
	return c.reportInputUpdate(oldValue, oldCursor)
}

// reportInputUpdate reports the current value/cursor relative to a snapshot.
// It returns true when text changed.
func (c *inputController) reportInputUpdate(oldValue string, oldCursor int) bool {
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
	submission := input.Command(c.input.Value())
	if c.input.IsComposing() {
		submission = input.Verbatim(c.input.Value())
	}
	if !c.submit(submission) {
		return
	}
	c.input.Reset()
	c.mode = ModeNormal
	c.historyRecall = false
	c.notify(ui.InputChangedMsg{Text: "", Cursor: 0})
}
