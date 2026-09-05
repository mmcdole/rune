package style

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// RenderBorder returns a horizontal border line with dim styling.
// An empty char draws the default rule.
func RenderBorder(width int, char string) string {
	if char == "" {
		char = "─"
	}
	return "\x1b[90m" + strings.Repeat(char, width) + "\x1b[0m"
}

// Styles holds the lipgloss styles the widgets render with. Server
// output and bar/status text arrive pre-styled from Lua (rune.style);
// only chrome the TUI draws itself is styled here.
type Styles struct {
	// Input
	InputText     lipgloss.Style
	InputCursor   lipgloss.Style
	InputSelected lipgloss.Style // whole-line keep-input selection

	// Picker overlay
	OverlaySelected      lipgloss.Style
	OverlayNormal        lipgloss.Style
	OverlayMatch         lipgloss.Style
	OverlayMatchSelected lipgloss.Style // Match highlighting on selected row

	// Panes
	PaneHeader lipgloss.Style
	PaneBorder lipgloss.Style

	// Misc
	Muted   lipgloss.Style
	Warning lipgloss.Style
}

// DefaultStyles returns the default style configuration.
func DefaultStyles() Styles {
	return Styles{
		// Input
		InputText: lipgloss.NewStyle(),
		InputCursor: lipgloss.NewStyle().
			Background(lipgloss.Color("255")).
			Foreground(lipgloss.Color("0")),
		InputSelected: lipgloss.NewStyle().Reverse(true),

		// Picker overlay (slash picker, fuzzy search)
		OverlaySelected: lipgloss.NewStyle().
			Background(lipgloss.Color("62")).
			Foreground(lipgloss.Color("230")),
		OverlayNormal: lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")),
		OverlayMatch: lipgloss.NewStyle().
			Foreground(lipgloss.Color("212")). // Magenta for matched chars
			Bold(true),
		OverlayMatchSelected: lipgloss.NewStyle().
			Foreground(lipgloss.Color("212")). // Magenta for matched chars
			Background(lipgloss.Color("62")).  // Same background as OverlaySelected
			Bold(true),

		// Panes
		PaneHeader: lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("62")),
		PaneBorder: lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")),

		// Misc
		Muted: lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")),
		Warning: lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")),
	}
}
