package tui

// Component represents any UI element that can be placed in a dock.
// This interface unifies bars, panes, and built-in components for
// the layout engine, allowing them to be rendered generically.
type Component interface {
	// Render returns the string representation for the given width.
	Render(width int) string

	// Height returns how many lines this component currently occupies.
	Height() int
}
