package ui

import "github.com/mmcdole/rune/input"

// UIEvent is implemented by all messages sent from UI to Session. They travel
// one ordered channel (UI.Events()), so accepted events cannot overtake each
// other.
type UIEvent interface {
	uiEvent() // unexported marker method - only this package can implement
}

// InputSubmittedMsg atomically transfers an accepted submission and the
// editable draft that should follow it to Session. Once this event is queued,
// the UI may apply the same transition locally.
type InputSubmittedMsg struct {
	Submission input.Submission
	NextDraft  string // editable text after submit; cursor is at its end
}

func (InputSubmittedMsg) uiEvent() {}

// OpenEditorMsg asks Session to edit this draft while the TUI is suspended.
// It carries the UI's text snapshot instead of relying on Lua input state.
type OpenEditorMsg struct{ Text string }

func (OpenEditorMsg) uiEvent() {}

// ExecuteBindMsg requests Session to execute a Lua key binding.
// Sent when the UI routes a registry binding to Lua.
type ExecuteBindMsg string

func (ExecuteBindMsg) uiEvent() {}

// WindowSizeChangedMsg notifies Session of window size changes.
// Session uses this to update rune.state.width/height.
type WindowSizeChangedMsg struct {
	Width  int
	Height int
}

func (WindowSizeChangedMsg) uiEvent() {}

// ScrollStateChangedMsg notifies Session of scroll state changes.
// Session uses this to update rune.state.scroll_mode/scroll_lines. It is
// state, not an event: the UI posts it only when the value changed.
type ScrollStateChangedMsg struct {
	Scrolled bool // output is scrolled back from live tailing
	NewLines int  // Lines behind live (when scrolled)
}

func (ScrollStateChangedMsg) uiEvent() {}

// SearchStateChangedMsg reports whether the modal scrollback-search
// navigator is active. Search interaction and viewport scroll position are
// independent state dimensions, so this is deliberately separate from
// ScrollStateChangedMsg.
type SearchStateChangedMsg bool

func (SearchStateChangedMsg) uiEvent() {}

// InputChangedMsg notifies Session of input content changes. Cursor is a
// zero-based rune offset from the input widget. Rev, here and in the other
// draft reports, is the latest Session edit the UI had applied; Session
// ignores reports made before the UI saw its newest edit, since that edit
// replaces them.
type InputChangedMsg struct {
	Text   string
	Cursor int
	Rev    uint64
}

func (InputChangedMsg) uiEvent() {}

// DraftAppliedMsg acknowledges a Session-requested editor update. It reconciles
// the mirror after any older user edits already queued in Events. Unlike
// InputChangedMsg it never runs Lua observers: the request already did that.
type DraftAppliedMsg struct {
	Text   string
	Cursor int // zero-based rune offset, like InputChangedMsg
	Rev    uint64
}

func (DraftAppliedMsg) uiEvent() {}

// CursorMovedMsg notifies Session of cursor position changes without a text
// change. Cursor is a zero-based rune offset from the input widget.
type CursorMovedMsg struct {
	Cursor int
	Rev    uint64
}

func (CursorMovedMsg) uiEvent() {}

// PickerSelectMsg is sent from UI back to Session when user interacts with picker.
type PickerSelectMsg struct {
	CallbackID int    // The callback ID from PickerOptions
	Value      string // The PickerItem.Value of the selection
	Accepted   bool   // True if user pressed Enter, false if Esc/cancel
}

func (PickerSelectMsg) uiEvent() {}
