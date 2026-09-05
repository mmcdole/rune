package widget

import (
	"image"
	"slices"

	"charm.land/lipgloss/v2"
)

// Rule describes a joinable line in cell coordinates. From is inclusive and To
// is exclusive. Widgets describe their decoration; the compositor owns joins.
type Rule struct {
	Vertical     bool
	At, From, To int
	// Labels occupy only their own cells on a horizontal rule.
	Labels []RuleLabel
}

// RuleLabel is independently positioned decoration on a horizontal rule.
type RuleLabel struct {
	Text  string
	At    int
	Style lipgloss.Style
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
		r.Labels = slices.Clone(r.Labels)
		for n := range r.Labels {
			r.Labels[n].At += origin.X
		}
	}
	return r
}
