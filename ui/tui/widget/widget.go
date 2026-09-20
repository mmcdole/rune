package widget

import "image"

// Widget supplies content and size constraints to layout. Measurement is read-only;
// SetSize applies the allocated content rectangle before interaction or rendering.
// Rules supplies border geometry; LabeledRules supplies the final decorations.
// Their cells stay blank in View; Model renders them after clipping content.
type Widget interface {
	MinimumSize() image.Point
	SetSize(width, height int)
	MeasureHeight(width, limit int) int
	View() string
}
