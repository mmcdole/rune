package widget

import (
	"image"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"
)

// Compile-time check that Separator implements Widget.
var _ Widget = (*Separator)(nil)

// Separator renders a horizontal line.
type Separator struct {
	width       int
	char        string
	borderStyle lipgloss.Style
}

// NewSeparator creates a separator with a fixed character and border style.
// Anything but a single-cell value falls back to the default rule.
func NewSeparator(char string, borderStyle lipgloss.Style) *Separator {
	if runewidth.StringWidth(char) != 1 {
		char = "─"
	}
	return &Separator{char: char, borderStyle: borderStyle}
}

// View implements Widget.
func (s *Separator) View() string {
	return s.borderStyle.Render(strings.Repeat(s.char, s.width))
}

// SetSize implements Widget.
func (s *Separator) SetSize(width, height int) {
	s.width = width
}

func (s *Separator) MeasureHeight(width, limit int) int { return min(1, limit) }

func (s *Separator) MinimumSize() image.Point { return image.Point{} }
