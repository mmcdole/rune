package ui

import "github.com/mmcdole/rune/input"

// UIEvent is implemented by all messages sent from UI to Session. They travel
// one ordered channel (UI.Events()), so accepted events cannot overtake each
// other.
type UIEvent interface {
	uiEvent() // unexported marker method - only this package can implement
}

// PrintLineMsg carries text to append to scrollback. It may contain newlines;
// the TUI splits and wraps it into rows.
type PrintLineMsg string

// EchoLineMsg carries a local echo (submitted input, already styled by the
// Lua "echo" hook) to append to scrollback. Like PrintLineMsg, the
// text may contain newlines.
type EchoLineMsg string

// SetPromptMsg replaces the prompt overlay with a partial line or confirmed prompt.
type SetPromptMsg string

// CommitPromptMsg moves the prompt overlay to scrollback in one update.
type CommitPromptMsg string

// PaneWriteMsg appends a line to a named pane.
type PaneWriteMsg struct {
	Name string
	Text string
}

// PaneToggleMsg toggles visibility of a named pane.
type PaneToggleMsg struct {
	Name string
}

// PaneSetVisibleMsg shows or hides a named pane.
type PaneSetVisibleMsg struct {
	Name    string
	Visible bool
}

// PaneCreateMsg creates a new named pane.
type PaneCreateMsg struct {
	Name string
}

// PaneClearMsg clears the contents of a named pane.
type PaneClearMsg struct {
	Name string
}

// --- Push-based UI Messages (Session -> UI) ---

// UpdateBindsMsg pushes the current set of bound keys from Session to UI.
// UI uses this to check if a key should be sent to Session for execution.
type UpdateBindsMsg map[string]bool

// UpdateBarsMsg pushes rendered bar content from Session to UI.
// Session runs Lua bar renderers and sends the result; UI just displays it.
type UpdateBarsMsg map[string]BarContent

// UpdateLayoutMsg pushes layout configuration from Session to UI.
type UpdateLayoutMsg struct {
	Top    []LayoutEntry
	Bottom []LayoutEntry
}

// Config carries the UI-facing subset of Rune's typed configuration.
type Config struct {
	// KeepInput keeps a submitted command selected in the input line, so Enter
	// resends it and typing replaces it.
	KeepInput bool
	// Numpad enables terminal modes that preserve physical numpad keys. Rune
	// still accepts an already-distinct numpad event when this is false.
	Numpad bool
	// Mouse captures terminal mouse events so the wheel can scroll Rune's
	// viewport. When disabled, the terminal retains native text selection.
	Mouse bool
}

// UpdateConfigMsg pushes UI-facing configuration from Session to UI.
type UpdateConfigMsg Config

// --- Push-based UI Messages (UI -> Session) ---

// InputSubmittedMsg atomically transfers an accepted submission and the
// editable draft that should follow it to Session. Once this event is queued,
// the UI may apply the same transition locally.
type InputSubmittedMsg struct {
	Submission input.Submission
	NextDraft  string // editable text after submit; cursor is at its end
}

func (InputSubmittedMsg) uiEvent() {}

// ExecuteBindMsg requests Session to execute a Lua key binding.
// Sent when UI detects a key that's in the boundKeys map.
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
// Session uses this to update rune.state.scroll_mode/scroll_lines.
type ScrollStateChangedMsg struct {
	Mode     string // "live" or "scrolled"
	NewLines int    // Lines behind live (when scrolled)
}

func (ScrollStateChangedMsg) uiEvent() {}

// SearchStateChangedMsg reports whether the modal scrollback-search
// navigator is active. Search interaction and viewport scroll position are
// independent state dimensions, so this is deliberately not another
// ScrollStateChangedMsg.Mode value.
type SearchStateChangedMsg bool

func (SearchStateChangedMsg) uiEvent() {}

// InputChangedMsg notifies Session of input content changes. Cursor is a
// zero-based rune offset from the input widget.
type InputChangedMsg struct {
	Text   string
	Cursor int
}

func (InputChangedMsg) uiEvent() {}

// CursorMovedMsg notifies Session of cursor position changes without a text
// change. Cursor is a zero-based rune offset from the input widget.
type CursorMovedMsg struct {
	Cursor int
}

func (CursorMovedMsg) uiEvent() {}

// --- Picker Messages (Session -> UI) ---

// ShowPickerMsg requests the UI to display a picker overlay.
// Sent from Session to UI when Lua calls rune.ui.picker.show().
type ShowPickerMsg struct {
	Title      string       // Optional title/header for the picker (modal mode only)
	Items      []PickerItem // Items to display
	CallbackID string       // Opaque ID to track which Lua callback to run
	// Inline mode: picker filters based on input content, doesn't trap keys.
	// Modal mode (default): picker captures keyboard and has its own search field.
	Inline bool
	// DismissOnSpace closes an inline picker as soon as the input contains
	// a space - for pickers over single-token items (slash commands) where
	// a space means the user has committed and is typing arguments.
	DismissOnSpace bool
}

// --- Search Messages (Session -> UI) ---

// ShowSearchMsg requests the UI to open the scrollback-search overlay.
// Sent from Session when Lua calls rune.ui.search(). Fire-and-forget:
// unlike the picker there is no callback to settle - the search is
// self-contained in the UI, and its only session-visible outcome is
// the viewport position, reported through ScrollStateChangedMsg.
type ShowSearchMsg struct {
	Query string // initial query; empty keeps the previous search's query
}

// SetClipboardMsg asks the terminal to set the system clipboard
// (OSC 52). Sent from Session when Lua calls rune.clipboard.set().
type SetClipboardMsg string

// SetInputMsg sets the input line content.
// Sent from Session when Lua calls rune.input.set().
type SetInputMsg string

// SetInputSubmissionMsg restores both draft text and interpretation.
// History recall uses this for one-line verbatim entries that have no
// structural character from which the UI could infer compose mode.
type SetInputSubmissionMsg input.Submission

// --- Picker Messages (UI -> Session) ---

// PickerSelectMsg is sent from UI back to Session when user interacts with picker.
type PickerSelectMsg struct {
	CallbackID string // The callback ID from ShowPickerMsg
	Value      string // The PickerItem.Value of the selection
	Accepted   bool   // True if user pressed Enter, false if Esc/cancel
}

func (PickerSelectMsg) uiEvent() {}

// --- Input Primitive Messages (Session -> UI) ---

// InputSetCursorMsg sets the widget cursor to a zero-based rune offset.
type InputSetCursorMsg int

// --- Pane Scrolling Messages (Session -> UI) ---

// PaneScrollUpMsg scrolls a pane up by N lines.
type PaneScrollUpMsg struct {
	Name  string
	Lines int
}

// PaneScrollDownMsg scrolls a pane down by N lines.
type PaneScrollDownMsg struct {
	Name  string
	Lines int
}

// PaneScrollToTopMsg scrolls a pane to the top.
type PaneScrollToTopMsg struct {
	Name string
}

// PaneScrollToBottomMsg scrolls a pane to the bottom.
type PaneScrollToBottomMsg struct {
	Name string
}
