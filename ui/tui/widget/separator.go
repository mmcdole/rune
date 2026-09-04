package widget

import (
	"image"

	"github.com/mattn/go-runewidth"
	"github.com/mmcdole/rune/ui/tui/style"
)

// Compile-time check that Separator implements Widget.
var _ Widget = (*Separator)(nil)

// Separator renders a horizontal line.
type Separator struct {
	width int
	char  string
}

// NewSeparator creates a new separator widget.
func NewSeparator() *Separator {
	return &Separator{}
}

// SetChar selects the rule character. Anything but a single-cell value is
// dropped defensively so direct Go callers cannot break the width math.
func (s *Separator) SetChar(char string) {
	if runewidth.StringWidth(char) != 1 {
		char = ""
	}
	s.char = char
}

// View implements Widget.
func (s *Separator) View() string {
	return style.RenderBorder(s.width, s.char)
}

// SetSize implements Widget.
func (s *Separator) SetSize(width, height int) {
	s.width = width
}

func (s *Separator) MeasureHeight(width, limit int) int { return min(1, limit) }

func (s *Separator) MinimumSize() image.Point { return image.Point{} }
