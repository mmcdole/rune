package widget

import (
	"image"

	"charm.land/lipgloss/v2"
)

// Rule describes a joinable line in cell coordinates. From is inclusive and To
// is exclusive. Widgets describe their decoration; the compositor owns joins.
type Rule struct {
	Vertical     bool
	At, From, To int
	// Label occupies only its own cells on a horizontal rule.
	Label      string
	LabelAt    int
	LabelStyle lipgloss.Style
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
		r.LabelAt += origin.X
	}
	return r
}
