package ui

// UIEvent is implemented by all messages sent from UI to Session.
// This provides compile-time type safety for the outbound channel.
type UIEvent interface {
	uiEvent() // unexported marker method - only this package can implement
}

// PrintLineMsg represents a line to append to scrollback.
// Used for all output: server lines, Lua prints, etc.
type PrintLineMsg string

// EchoLineMsg represents a local echo (user input) to append to scrollback.
type EchoLineMsg string

// PromptMsg represents a server prompt (partial line without newline).
type PromptMsg string

// PaneWriteMsg appends a line to a named pane.
type PaneWriteMsg struct {
	Name string
	Text string
}

// PaneToggleMsg toggles visibility of a named pane.
type PaneToggleMsg struct {
	Name string
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

// --- Push-based UI Messages (UI -> Session) ---

// ExecuteBindMsg requests Session to execute a Lua key binding.
// Sent when UI detects a key that's in the boundKeys map.
type ExecuteBindMsg string

func (ExecuteBindMsg) uiEvent() {}

// WindowSizeMsg notifies Session of window size changes.
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

// InputChangedMsg notifies Session of input content changes.
// Session tracks this so Lua can query current input via rune.input.get().
type InputChangedMsg struct {
	Text   string
	Cursor int
}

func (InputChangedMsg) uiEvent() {}

// CursorMovedMsg notifies Session of cursor position changes (without text change).
// This allows tracking cursor for Lua without triggering input_changed hooks.
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
}

// SetInputMsg sets the input line content.
// Sent from Session when Lua calls rune.input.set().
type SetInputMsg string

// --- Picker Messages (UI -> Session) ---

// PickerSelectMsg is sent from UI back to Session when user interacts with picker.
type PickerSelectMsg struct {
	CallbackID string // The callback ID from ShowPickerMsg
	Value      string // The PickerItem.Value of the selection
	Accepted   bool   // True if user pressed Enter, false if Esc/cancel
}

func (PickerSelectMsg) uiEvent() {}

// --- Input Primitive Messages (Session -> UI) ---

// InputSetCursorMsg sets the cursor position.
type InputSetCursorMsg int

// SetGhostMsg sets the ghost text for command-level suggestions.
// Go just renders this as dim text if it prefix-matches the input.
// Lua is the source of truth for what to suggest.
type SetGhostMsg string

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
