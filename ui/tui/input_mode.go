package tui

import (
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/widget"
)

// inputMode represents the current input handling mode.
type inputMode int

const (
	modeNormal       inputMode = iota // Standard text input
	modeCompose                       // Lossless structured-text input
	modePickerModal                   // Modal picker traps all keys
	modePickerInline                  // Inline picker filters based on input
	modeSearch                        // Scrollback-search overlay traps all keys
)

// searchEffects is the viewport half of scrollback search, implemented
// by Model. The controller owns the mode state machine; the viewport
// snapshot, centering, and highlight stay with the widget owner.
type searchEffects interface {
	OpenSearch() widget.SearchScope              // snapshot viewport and choose the search origin
	PreviewSearch(m widget.SearchMatch, ok bool) // center+highlight the match, or restore when none
	CommitSearch()                               // keep the accepted match and its highlight
	CancelSearch()                               // restore the viewport and prior highlight
}

// inputController owns input transitions and Session effects:
// the active picker's Lua callback, and the invariant that every path
// out of a picker mode resets the mode, hides the overlay, and settles
// the callback (exactly one PickerSelectMsg per shown picker). It is
// also the single place that reports input text changes to the session,
// so the session's tracked input (rune.input.get) can never go stale.
type inputController struct {
	input *widget.Input

	// Picker callbacks belong to the Session contract, not the widget.
	pickerCB      string // Lua callback ID to settle on close
	pickerDismiss bool   // close inline picker once input contains a space
	historyRecall bool   // unmodified verbatim entry restored from history
	keepOnSubmit  bool   // keep_input: keep the sent command selected

	notify  func(ui.UIEvent)                // state and actions sent to the session
	submit  func(ui.InputSubmittedMsg) bool // atomically transfer submission and following draft
	isBound func(key string) bool           // key has a Lua bind
	scroll  func(tea.KeyPressMsg) bool      // Go scroll-key fallback; true if handled
	search  searchEffects                   // viewport side of scrollback search
}

func newInputController(
	input *widget.Input,
	notify func(ui.UIEvent),
	submit func(ui.InputSubmittedMsg) bool,
	isBound func(string) bool,
	scroll func(tea.KeyPressMsg) bool,
	search searchEffects,
) *inputController {
	return &inputController{
		input:   input,
		notify:  notify,
		submit:  submit,
		isBound: isBound,
		scroll:  scroll,
		search:  search,
	}
}

// mode derives routing from the active widget, never a second mutable state.
func (c *inputController) mode() inputMode {
	switch {
	case c.input.SearchActive():
		return modeSearch
	case c.input.PickerInline():
		return modePickerInline
	case c.input.PickerActive():
		return modePickerModal
	case c.input.IsComposing():
		return modeCompose
	default:
		return modeNormal
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
func (c *inputController) HandleKey(msg tea.KeyPressMsg) {
	msg = normalizeNumpadText(msg)
	isEscape := matchesKey(msg, tea.KeyEsc, 0)
	if c.mode() == modeCompose && !isEscape {
		c.input.ContinueCompose()
	}

	// Picker modes capture Ctrl+C/Esc as "cancel". In normal mode they
	// fall through so the Lua binds decide (clear input, double-tap quit,
	// ...). Compose mode owns Escape because it is an internal cancel.
	isCancel := isEscape || matchesKey(msg, 'c', tea.ModCtrl)
	if isCancel {
		if c.mode() == modePickerModal || c.mode() == modePickerInline {
			c.closePicker(false, "")
			return
		}
		if c.mode() == modeSearch {
			c.closeSearch(false)
			return
		}
		if c.mode() == modeCompose && isEscape {
			if c.input.ConfirmDiscard() {
				c.cancelCompose()
			}
			return
		}
	}

	switch c.mode() {
	case modeCompose:
		c.handleComposeKey(msg)
	case modePickerModal:
		c.handleModalKey(msg)
	case modePickerInline:
		c.handleInlineKey(msg)
	case modeSearch:
		c.handleSearchKey(msg)
	default:
		c.handleNormalKey(msg)
	}
}

// SetText replaces the input content (rune.input.set). Lua editing
// binds (ctrl+u, ctrl+w) change input while the inline picker is open;
// keep its filter in sync, and close the picker (cancelling its
// callback) when the input is cleared.
func (c *inputController) SetText(text string) {
	if c.mode() == modeSearch {
		c.closeSearch(false)
	}
	c.historyRecall = false
	wasPicker := c.mode() == modePickerInline || c.mode() == modePickerModal
	wasInline := c.mode() == modePickerInline
	c.input.SetValue(text)
	c.input.CursorEnd()
	c.notify(ui.InputChangedMsg{Text: c.input.Value(), Cursor: c.input.Position()})

	if c.input.IsComposing() {
		if wasPicker {
			c.closePicker(false, "")
		}
		return
	}
	if wasInline {
		c.syncInlineFilter()
		return
	}

}

// SetSubmission restores a history entry with explicit interpretation.
// Unlike SetText, an explicit command entry exits sticky compose mode, while
// verbatim is forced even for one safe, non-empty physical line.
func (c *inputController) SetSubmission(submission input.Submission) {
	if c.mode() == modeSearch {
		c.closeSearch(false)
	}
	wasPicker := c.mode() == modePickerInline || c.mode() == modePickerModal
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

}

func (c *inputController) dispatchNumpadEnterBind(msg tea.KeyPressMsg) bool {
	info, ok := numpadKey(msg)
	if !ok || info.name != "numpad_enter" || msg.Mod&keyModifiers != 0 || !c.isBound(info.name) {
		return false
	}
	c.notify(ui.ExecuteBindMsg(info.name))
	return true
}

// tryNormalBind applies normal mode's typing-safety rule. Text-bearing
// keys dispatch only when the draft is empty or selected; non-text keys can
// remain useful as movement binds while a command is being composed.
func (c *inputController) tryNormalBind(msg tea.KeyPressMsg) bool {
	key := keyToString(msg)
	if key == "" || !c.isBound(key) {
		return false
	}
	if msg.Text != "" && c.input.Value() != "" && !c.input.Selected() {
		return false
	}
	c.notify(ui.ExecuteBindMsg(key))
	return true
}

func (c *inputController) handleNormalKey(msg tea.KeyPressMsg) {
	if c.dispatchNumpadEnterBind(msg) {
		return
	}
	// Accept both portable Ctrl+J and disambiguated Ctrl+Enter as the explicit
	// way to start a multiline draft without pasting.
	if isComposerNewline(msg) {
		c.insertComposerText("\n")
		return
	}
	if matchesEnterKey(msg, 0) {
		c.submitInput()
		return
	}

	// A NumLock-off physical bind gets first refusal. If there is no such
	// bind, route the key through its ordinary navigation meaning, including
	// any Lua bind on that base key.
	if info, ok := numpadNavigation(msg); ok {
		if c.tryNormalBind(msg) {
			return
		}
		msg = info.navigationFallback(msg)
	}
	if c.tryNormalBind(msg) {
		return
	}
	// Unbound scroll keys: Go fallback (keeps degraded mode scrollable)
	if c.scroll(msg) {
		return
	}

	c.forwardToInput(msg)
}

func (c *inputController) handleComposeKey(msg tea.KeyPressMsg) {
	if c.dispatchNumpadEnterBind(msg) {
		return
	}
	if matchesEnterKey(msg, 0) {
		c.submitInput()
		return
	}

	// Compose owns editing mechanics. Give NumLock-off keypad keys their
	// semantic navigation meaning first, but retain the physical name so an
	// unconsumed modified chord can still be delegated to Lua.
	physicalKey := ""
	physicalModified := false
	if info, ok := numpadNavigation(msg); ok {
		physicalKey = keyToString(msg)
		physicalModified = msg.Mod&keyModifiers != 0
		msg = info.navigationFallback(msg)
	}
	if c.historyRecall {
		key := ""
		delta := 0
		switch {
		case matchesKey(msg, tea.KeyUp, 0):
			key, delta = "up", -1
		case matchesKey(msg, tea.KeyDown, 0):
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

		return
	}

	if physicalModified && physicalKey != "" && c.isBound(physicalKey) {
		c.notify(ui.ExecuteBindMsg(physicalKey))
		return
	}

	// Non-editing chords remain scriptable in compose mode. In
	// particular, Ctrl+E keeps using the existing external-editor bind.
	if keyStr := keyToString(msg); keyStr != "" && c.isBound(keyStr) {
		c.notify(ui.ExecuteBindMsg(keyStr))
	}
}

// HandlePaste routes one atomic bracketed-paste payload. It bypasses bind
// dispatch even when the payload is a single printable character.
func (c *inputController) HandlePaste(text string) {
	switch c.mode() {
	case modePickerModal:
		c.input.Picker().Filter(c.input.Picker().Query() + text)
		return
	case modeSearch:
		c.input.Search().TypeRunes([]rune(text))
		c.previewSearch()
		return
	}
	c.handlePaste(text)
}

func (c *inputController) handlePaste(text string) {
	c.historyRecall = false
	oldValue := c.input.Value()
	oldCursor := c.input.Position()
	wasInline := c.mode() == modePickerInline

	c.input.InsertPaste(text)
	c.reportInputUpdate(oldValue, oldCursor)

	if c.input.IsComposing() {
		if wasInline {
			// InputChangedMsg is deliberately emitted before the callback so
			// Lua observes the newly pasted draft when cancellation runs.
			c.closePicker(false, "")
		}
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
	c.reportInputUpdate(oldValue, oldCursor)
}

func (c *inputController) cancelCompose() {
	c.historyRecall = false
	c.input.Reset()
	c.notify(ui.InputChangedMsg{Text: "", Cursor: 0})
}

// isComposerNewline accepts the portable Ctrl+J representation as well as the
// distinct Ctrl+Enter event reported by terminals with key disambiguation.
func isComposerNewline(msg tea.KeyPressMsg) bool {
	return matchesKey(msg, 'j', tea.ModCtrl) || matchesEnterKey(msg, tea.ModCtrl)
}

// forwardToInput passes an editing key to the text input and reports
// the resulting text or cursor change to the session. Returns true if
// the text changed.
func (c *inputController) forwardToInput(msg tea.KeyPressMsg) bool {
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

// SetKeepOnSubmit applies the pushed keep_input preference.
// Turning it off releases the keep-input selection without clearing the draft.
func (c *inputController) SetKeepOnSubmit(on bool) {
	c.keepOnSubmit = on
	if !on {
		c.input.Deselect()
	}
}

// submitInput transfers the current submission and its following draft as one
// event, then applies that same transition locally only after Session accepts
// ownership.
func (c *inputController) submitInput() {
	submission := input.Command(c.input.Value())
	if c.input.IsComposing() {
		submission = input.Verbatim(c.input.Value())
	}
	keep := c.keepOnSubmit && submission.Mode == input.ModeCommand && submission.Text != ""
	nextDraft := ""
	if keep {
		nextDraft = submission.Text
	}
	if !c.submit(ui.InputSubmittedMsg{Submission: submission, NextDraft: nextDraft}) {
		return
	}
	c.historyRecall = false
	if keep {
		// zMUD-style keep: the sent command stays in the line, selected,
		// so Enter resends it and typing replaces it. The accepted submission
		// already carries this post-submit draft to Session.
		c.input.SelectAll()
		c.input.CursorEnd()
		return
	}
	c.input.Reset()
}
