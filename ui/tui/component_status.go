package tui

import "github.com/drake/rune/ui/tui/components/status"

// StatusComponent adapts the default status bar for the Component interface.
type StatusComponent struct {
	status *status.Bar
}

// Height returns 1 since the status bar is always single-line.
func (s *StatusComponent) Height() int {
	return 1
}

// Render returns the status bar content.
func (s *StatusComponent) Render(width int) string {
	return s.status.View()
}
