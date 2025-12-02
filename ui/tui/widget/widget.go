package widget

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/drake/rune/ui/tui/layout"
)

// Widget layers tea.Model on top of layout.Renderer for interactive components.
// Layout code operates on Renderer; Bubble Tea code uses Widget.
type Widget interface {
	tea.Model       // Init, Update, View
	layout.Renderer // SetWidth, Height, View (View satisfies both)
}
