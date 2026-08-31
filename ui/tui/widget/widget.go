package widget

// Widget is the interface for layout-aware UI elements.
type Widget interface {
	SetSize(width, height int)
	PreferredHeight() int
	View() string
}
