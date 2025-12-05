package widget

import "strings"

// Compile-time check that Separator implements Widget
var _ Widget = (*Separator)(nil)

// Separator renders a horizontal line.
type Separator struct {
	width int
}

// NewSeparator creates a new separator widget.
func NewSeparator() *Separator {
	return &Separator{}
}

// View implements Widget.
func (s *Separator) View() string {
	return "\x1b[90m" + strings.Repeat("─", s.width) + "\x1b[0m"
}

// SetSize implements Widget.
func (s *Separator) SetSize(width, height int) {
	s.width = width
}

// PreferredHeight implements Widget.
func (s *Separator) PreferredHeight() int {
	return 1
}
