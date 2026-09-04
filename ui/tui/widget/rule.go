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

func boxRules(width, height int) []Rule {
	return []Rule{
		{At: 0, To: width}, {At: height - 1, To: width},
		{Vertical: true, At: 0, To: height}, {Vertical: true, At: width - 1, To: height},
	}
}
