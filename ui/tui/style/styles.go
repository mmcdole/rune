package style

import (
	"github.com/charmbracelet/lipgloss"
)

// Styles holds all the lipgloss styles for the TUI.
type Styles struct {
	// Layout
	App        lipgloss.Style
	Scrollback lipgloss.Style
	PromptArea lipgloss.Style
	InputArea  lipgloss.Style
	StatusBar  lipgloss.Style

	// Status indicators
	StatusConnected    lipgloss.Style
	StatusDisconnected lipgloss.Style
	StatusConnecting   lipgloss.Style
	StatusLive         lipgloss.Style
	StatusScrolled     lipgloss.Style

	// Input
	InputPrompt lipgloss.Style
	InputText   lipgloss.Style
	InputCursor lipgloss.Style

	// Overlay
	OverlayBorder        lipgloss.Style
	OverlaySelected      lipgloss.Style
	OverlayNormal        lipgloss.Style
	OverlayMatch         lipgloss.Style
	OverlayMatchSelected lipgloss.Style // Match highlighting on selected row

	// Panes
	PaneHeader lipgloss.Style
	PaneBorder lipgloss.Style

	// Misc
	Muted   lipgloss.Style
	Error   lipgloss.Style
	Warning lipgloss.Style
}

// DefaultStyles returns the default style configuration.
func DefaultStyles() Styles {
	return Styles{
		// Layout - minimal borders, let content breathe
		App: lipgloss.NewStyle(),
		Scrollback: lipgloss.NewStyle().
			Padding(0),
		PromptArea: lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")),
		InputArea: lipgloss.NewStyle(),
		StatusBar: lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")),

		// Status indicators - subtle colors
		StatusConnected: lipgloss.NewStyle().
			Foreground(lipgloss.Color("71")), // Muted green
		StatusDisconnected: lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")), // Gray (subtle)
		StatusConnecting: lipgloss.NewStyle().
			Foreground(lipgloss.Color("179")), // Muted yellow
		StatusLive: lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")), // Gray
		StatusScrolled: lipgloss.NewStyle().
			Foreground(lipgloss.Color("179")), // Muted yellow

		// Input
		InputPrompt: lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")),
		InputText: lipgloss.NewStyle(),
		InputCursor: lipgloss.NewStyle().
			Background(lipgloss.Color("255")).
			Foreground(lipgloss.Color("0")),

		// Overlay (slash picker, fuzzy search)
		OverlayBorder: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1),
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
		Error: lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")),
		Warning: lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")),
	}
}
