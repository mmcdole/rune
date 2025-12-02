package tui

import "github.com/drake/rune/ui/tui/components/panes"

// PaneComponent adapts a Pane for the Component interface.
// It delegates to the panes.Manager for rendering.
type PaneComponent struct {
	manager *panes.Manager
	name    string
}

// Height returns the render height of the pane (0 if hidden).
func (p *PaneComponent) Height() int {
	return p.manager.GetHeight(p.name)
}

// Render returns the pane content with header and borders.
func (p *PaneComponent) Render(width int) string {
	return p.manager.RenderPane(p.name, width)
}
