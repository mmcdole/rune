package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ServerLineMsg represents a line from the MUD server.
type ServerLineMsg string

// DisplayLineMsg represents a line to append to scrollback (server output or prompt commit).
type DisplayLineMsg string

// EchoLineMsg represents a local echo (user input) to append to scrollback.
type EchoLineMsg string

// BatchedLinesMsg carries multiple lines in one message for efficient batching.
type BatchedLinesMsg struct {
	Lines []string
}

// PromptMsg represents a server prompt (partial line without newline).
type PromptMsg string

// ConnectionStateMsg notifies the TUI of connection state changes.
type ConnectionStateMsg struct {
	State   ConnectionState
	Address string
}

// ConnectionState represents the network connection status.
type ConnectionState int

const (
	StateDisconnected ConnectionState = iota
	StateConnecting
	StateConnected
)

func (s ConnectionState) String() string {
	switch s {
	case StateDisconnected:
		return "Disconnected"
	case StateConnecting:
		return "Connecting"
	case StateConnected:
		return "Connected"
	default:
		return "Unknown"
	}
}

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

// StatusTextMsg updates the status bar text from Lua.
type StatusTextMsg string

// InfobarMsg updates the info bar (above input line) from Lua.
type InfobarMsg string

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

// PaneBindMsg binds a key to toggle a pane.
type PaneBindMsg struct {
	Key  string
	Name string
}
