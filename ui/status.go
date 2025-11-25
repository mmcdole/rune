package ui

import (
	"fmt"
	"strings"
)

// StatusBar displays connection state, scroll mode, and other indicators.
type StatusBar struct {
	// Lua-driven text (if set, overrides connection display)
	luaText string

	// Fallback connection state (used if luaText is empty)
	connState  ConnectionState
	serverAddr string

	scrollMode ScrollMode
	newLines   int
	width      int
	styles     Styles
}

// NewStatusBar creates a new status bar.
func NewStatusBar(styles Styles) StatusBar {
	return StatusBar{
		connState: StateDisconnected,
		styles:    styles,
	}
}

// SetWidth updates the status bar width.
func (s *StatusBar) SetWidth(w int) {
	s.width = w
}

// SetText sets the status bar text from Lua (overrides connection display).
func (s *StatusBar) SetText(text string) {
	s.luaText = text
}

// SetConnectionState updates the connection status (fallback if no Lua text).
func (s *StatusBar) SetConnectionState(state ConnectionState, addr string) {
	s.connState = state
	s.serverAddr = addr
}

// SetScrollMode updates the scroll mode indicator.
func (s *StatusBar) SetScrollMode(mode ScrollMode, newLines int) {
	s.scrollMode = mode
	s.newLines = newLines
}

// View renders the status bar.
func (s *StatusBar) View() string {
	// Left section: Lua text or connection status
	var left string
	if s.luaText != "" {
		left = s.luaText
	} else {
		// Fallback to connection state
		switch s.connState {
		case StateConnected:
			left = s.styles.StatusConnected.Render("● " + s.serverAddr)
		case StateConnecting:
			left = s.styles.StatusConnecting.Render("● Connecting...")
		case StateDisconnected:
			left = s.styles.StatusDisconnected.Render("● Disconnected")
		}
	}

	// Right section: scroll mode
	var right string
	switch s.scrollMode {
	case ModeLive:
		right = s.styles.StatusLive.Render("LIVE")
	case ModeScrolled:
		if s.newLines > 0 {
			right = s.styles.StatusScrolled.Render(fmt.Sprintf("SCROLLED (%d new)", s.newLines))
		} else {
			right = s.styles.StatusScrolled.Render("SCROLLED")
		}
	}

	// Calculate padding
	contentLen := len(stripAnsi(left)) + len(stripAnsi(right))
	padding := s.width - contentLen - 2
	if padding < 1 {
		padding = 1
	}

	return left + strings.Repeat(" ", padding) + right
}

// stripAnsi removes ANSI escape codes for length calculation
func stripAnsi(s string) string {
	var result strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}
