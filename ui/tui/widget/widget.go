package widget

import "image"

// Widget supplies content and size constraints to layout. Measurement is read-only;
// SetSize applies the allocated content rectangle before interaction or rendering.
// Input additionally supplies RuleRows for proposed geometry and Labels for its
// current applied size. Rule cells stay blank in View; Model draws borders and
// current labels after clipping content.
type Widget interface {
	MinimumSize() image.Point
	SetSize(width, height int)
	MeasureHeight(width, limit int) int
	View() string
}
