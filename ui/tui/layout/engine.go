package layout

import "strings"

// Dock represents renderers in a dock (top or bottom).
type Dock struct {
	Renderers []Renderer
}

// Height returns the total height of all renderers in the dock.
func (d *Dock) Height() int {
	h := 0
	for _, r := range d.Renderers {
		h += r.Height()
	}
	return h
}

// SetWidth sets the width on all renderers in the dock.
func (d *Dock) SetWidth(w int) {
	for _, r := range d.Renderers {
		r.SetWidth(w)
	}
}

// View returns the rendered view of all visible renderers concatenated.
func (d *Dock) View() string {
	var parts []string
	for _, r := range d.Renderers {
		if r.Height() > 0 {
			parts = append(parts, r.View())
		}
	}
	return strings.Join(parts, "\n")
}

// Engine calculates layout for top dock, viewport, and bottom dock.
type Engine struct {
	width  int
	height int
}

// NewEngine creates a new layout engine.
func NewEngine() *Engine {
	return &Engine{}
}

// SetSize sets the total available size.
func (e *Engine) SetSize(width, height int) {
	e.width = width
	e.height = height
}

// Width returns the current width.
func (e *Engine) Width() int {
	return e.width
}

// Calculate computes layout given top and bottom docks.
// Sets width on all renderers and returns viewport height.
func (e *Engine) Calculate(top, bottom *Dock) int {
	top.SetWidth(e.width)
	bottom.SetWidth(e.width)

	topHeight := top.Height()
	bottomHeight := bottom.Height()

	viewportHeight := e.height - topHeight - bottomHeight
	if viewportHeight < 1 {
		viewportHeight = 1
	}

	return viewportHeight
}
