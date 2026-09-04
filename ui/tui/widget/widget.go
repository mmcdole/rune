package widget

import "image"

// Widget is the layout-facing contract of a surface. Measurement is read-only;
// SetSize applies the allocated content rectangle before interaction or painting.
// Surfaces with Rules leave those decoration cells blank in View; the compositor
// paints their lines and labels after clipping all content.
type Widget interface {
	MinimumSize() image.Point
	SetSize(width, height int)
	MeasureHeight(width, limit int) int
	View() string
}
