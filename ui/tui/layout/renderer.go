package layout

// Renderer is the minimal interface for layout-aware components.
// Used by the layout engine to calculate sizes and render output.
type Renderer interface {
	SetWidth(w int)
	Height() int
	View() string
}
