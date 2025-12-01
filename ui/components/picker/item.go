package picker

import "github.com/drake/rune/ui/style"

// Item represents a row in the picker.
// Implement this interface for custom item types.
type Item interface {
	// FilterValue returns the string to be used for fuzzy matching.
	FilterValue() string

	// Render returns the styled string for a single row.
	// matches contains the indices of characters to highlight.
	// selected indicates whether this item is currently selected.
	Render(width int, selected bool, matches []int, styles style.Styles) string
}
