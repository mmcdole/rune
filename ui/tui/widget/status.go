package widget

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/drake/rune/ui"
	"github.com/drake/rune/ui/tui/style"
	"github.com/drake/rune/ui/tui/util"
)

// ScrollMode represents viewport scroll state.
type ScrollMode int

const (
	ModeLive ScrollMode = iota
	ModeScrolled
)

// Status displays connection state, scroll mode, and other indicators.
type Status struct {
	connState  ui.ConnectionState
	serverAddr string
	scrollMode ScrollMode
	newLines   int
	width      int
	styles     style.Styles
}

// NewStatus creates a new status widget.
func NewStatus(styles style.Styles) *Status {
	return &Status{
		connState: ui.StateDisconnected,
		styles:    styles,
	}
}

// Init implements tea.Model.
func (s *Status) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (s *Status) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return s, nil
}

// View implements tea.Model.
func (s *Status) View() string {
	// Left: Connection state
	var left string
	switch s.connState {
	case ui.StateConnected:
		left = s.styles.StatusConnected.Render("● " + s.serverAddr)
	case ui.StateConnecting:
		left = s.styles.StatusConnecting.Render("● Connecting...")
	case ui.StateDisconnected:
		left = s.styles.StatusDisconnected.Render("● Disconnected")
	}

	// Right: Scroll mode
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

	// Padding
	contentLen := util.VisibleLen(left) + util.VisibleLen(right)
	padding := s.width - contentLen - 2
	if padding < 1 {
		padding = 1
	}

	return left + strings.Repeat(" ", padding) + right
}

// SetWidth implements Widget.
func (s *Status) SetWidth(w int) {
	s.width = w
}

// Height implements Widget.
func (s *Status) Height() int {
	return 1
}

// SetConnectionState updates the connection status.
func (s *Status) SetConnectionState(state ui.ConnectionState, addr string) {
	s.connState = state
	s.serverAddr = addr
}

// SetScrollMode updates the scroll mode indicator.
func (s *Status) SetScrollMode(mode ScrollMode, newLines int) {
	s.scrollMode = mode
	s.newLines = newLines
}
