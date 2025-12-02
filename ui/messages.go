package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/drake/rune/mud"
	"github.com/drake/rune/ui/components/status"
)

// PrintLineMsg represents a line to append to scrollback.
// Used for all output: server lines, Lua prints, etc.
type PrintLineMsg string

// EchoLineMsg represents a local echo (user input) to append to scrollback.
type EchoLineMsg string

// PromptMsg represents a server prompt (partial line without newline).
type PromptMsg string

// ConnectionStateMsg notifies the TUI of connection state changes.
type ConnectionStateMsg struct {
	State   status.ConnectionState
	Address string
}

// ConnectionState type aliases for external use.
type ConnectionState = status.ConnectionState

const (
	StateDisconnected = status.StateDisconnected
	StateConnecting   = status.StateConnecting
	StateConnected    = status.StateConnected
)

// tickMsg is used for periodic updates (line batching, clock refresh).
type tickMsg time.Time

// flushLinesMsg signals the model to flush pending lines.
type flushLinesMsg struct {
	Lines []string
}

// doTick returns a command that sends a tickMsg after the given duration.
func doTick() tea.Cmd {
	return tea.Tick(16*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

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
type UpdateBarsMsg map[string]mud.BarContent

// UpdateLayoutMsg pushes layout configuration from Session to UI.
type UpdateLayoutMsg struct {
	Top    []string
	Bottom []string
}

// --- Push-based UI Messages (UI -> Session) ---

// ExecuteBindMsg requests Session to execute a Lua key binding.
// Sent when UI detects a key that's in the boundKeys map.
type ExecuteBindMsg string

// WindowSizeMsg notifies Session of window size changes.
// Session uses this to update rune.state.width/height.
type WindowSizeChangedMsg struct {
	Width  int
	Height int
}

// ScrollStateChangedMsg notifies Session of scroll state changes.
// Session uses this to update rune.state.scroll_mode/scroll_lines.
type ScrollStateChangedMsg struct {
	Mode     string // "live" or "scrolled"
	NewLines int    // Lines behind live (when scrolled)
}

// InputChangedMsg notifies Session of input content changes.
// Session tracks this so Lua can query current input via rune.input.get().
type InputChangedMsg string

// BarContent is an alias for mud.BarContent for convenience.
type BarContent = mud.BarContent

// --- Picker Messages (Session -> UI) ---

// ShowPickerMsg requests the UI to display a picker overlay.
// Sent from Session to UI when Lua calls rune.ui.picker.show().
type ShowPickerMsg struct {
	Title      string           // Optional title/header for the picker (modal mode only)
	Items      []mud.PickerItem // Items to display
	CallbackID string           // Opaque ID to track which Lua callback to run
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
