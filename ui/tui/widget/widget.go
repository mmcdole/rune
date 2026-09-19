package widget

import "image"

// Widget supplies content and size constraints to layout. Measurement is read-only;
// SetSize applies the allocated content rectangle before interaction or rendering.
// Widgets with Rules leave those decoration cells blank in View; Model
// renders their lines and labels after clipping all content.
type Widget interface {
	MinimumSize() image.Point
	SetSize(width, height int)
	MeasureHeight(width, limit int) int
	View() string
}
