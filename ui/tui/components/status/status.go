package status

import (
	"fmt"
	"strings"

	"github.com/drake/rune/ui"
	"github.com/drake/rune/ui/tui/components/viewport"
	"github.com/drake/rune/ui/tui/style"
	"github.com/drake/rune/ui/tui/util"
)

// Bar displays connection state, scroll mode, and other indicators.
// This is the default system status bar, used when Lua doesn't define
// a bar named "status" via rune.ui.bar("status", fn).
type Bar struct {
	connState  ui.ConnectionState
	serverAddr string
	scrollMode viewport.ScrollMode
	newLines   int
	width      int
	styles     style.Styles
}

// New creates a new status bar.
func New(styles style.Styles) Bar {
	return Bar{
		connState: ui.StateDisconnected,
		styles:    styles,
	}
}

// SetWidth updates the status bar width.
func (s *Bar) SetWidth(w int) {
	s.width = w
}

// SetConnectionState updates the connection status.
func (s *Bar) SetConnectionState(state ui.ConnectionState, addr string) {
	s.connState = state
	s.serverAddr = addr
}

// SetScrollMode updates the scroll mode indicator.
func (s *Bar) SetScrollMode(mode viewport.ScrollMode, newLines int) {
	s.scrollMode = mode
	s.newLines = newLines
}

// View renders the default system status bar.
func (s *Bar) View() string {
	// Left section: Connection state
	var left string
	switch s.connState {
	case ui.StateConnected:
		left = s.styles.StatusConnected.Render("● " + s.serverAddr)
	case ui.StateConnecting:
		left = s.styles.StatusConnecting.Render("● Connecting...")
	case ui.StateDisconnected:
		left = s.styles.StatusDisconnected.Render("● Disconnected")
	}

	// Right section: scroll mode
	var right string
	switch s.scrollMode {
	case viewport.ModeLive:
		right = s.styles.StatusLive.Render("LIVE")
	case viewport.ModeScrolled:
		if s.newLines > 0 {
			right = s.styles.StatusScrolled.Render(fmt.Sprintf("SCROLLED (%d new)", s.newLines))
		} else {
			right = s.styles.StatusScrolled.Render("SCROLLED")
		}
	}

	// Calculate padding
	contentLen := util.VisibleLen(left) + util.VisibleLen(right)
	padding := s.width - contentLen - 2
	if padding < 1 {
		padding = 1
	}

	return left + strings.Repeat(" ", padding) + right
}
