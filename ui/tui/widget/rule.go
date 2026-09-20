package widget

import "image"

// Rule describes a joinable line in cell coordinates. From is inclusive and To
// is exclusive. Widgets describe geometry; the renderer owns joins.
type Rule struct {
	Vertical     bool
	At, From, To int
}

func (r Rule) Translate(origin image.Point) Rule {
	if r.Vertical {
		r.At += origin.X
		r.From += origin.Y
		r.To += origin.Y
	} else {
		r.At += origin.Y
		r.From += origin.X
		r.To += origin.X
	}
	return r
}
