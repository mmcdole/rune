package ui

import "github.com/drake/rune/mud"

// UIMode specifies which UI implementation to use.
type UIMode int

const (
	// ModeTUI uses the full Bubble Tea TUI.
	ModeTUI UIMode = iota
	// ModeConsole uses the simple stdin/stdout console.
	ModeConsole
)

// New creates a new UI based on the specified mode.
func New(mode UIMode) mud.UI {
	switch mode {
	case ModeConsole:
		return NewConsoleUI()
	case ModeTUI:
		fallthrough
	default:
		return NewBubbleTeaUI()
	}
}

// NewWithDefault creates a TUI by default, with fallback to console
// if the terminal doesn't support alt screen.
func NewWithDefault() mud.UI {
	return NewBubbleTeaUI()
}
