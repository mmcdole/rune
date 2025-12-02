package tui

import "strings"

// SeparatorComponent adapts a horizontal separator line for the Component interface.
type SeparatorComponent struct {
	width *int
}

// Height returns 1 since the separator is always single-line.
func (s *SeparatorComponent) Height() int {
	return 1
}

// Render returns a dim horizontal line spanning the full width.
func (s *SeparatorComponent) Render(width int) string {
	return "\x1b[90m" + strings.Repeat("─", width) + "\x1b[0m"
}
