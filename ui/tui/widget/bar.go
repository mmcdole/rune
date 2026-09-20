package widget

import (
	"image"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/util"
)

// Compile-time check that Bar implements Widget
var _ Widget = (*Bar)(nil)

// Bar renders a Lua-defined bar with left/center/right sections.
type Bar struct {
	content ui.BarContent
	width   int
}

// SetContent updates the bar's content.
func (b *Bar) SetContent(content ui.BarContent) bool {
	if b.content == content {
		return false
	}
	b.content = content
	return true
}

// View implements Widget.
func (b *Bar) View() string {
	left := b.content.Left
	center := b.content.Center
	right := b.content.Right

	leftWidth := ansi.StringWidth(left)
	centerWidth := ansi.StringWidth(center)
	rightWidth := ansi.StringWidth(right)

	centerPad := 0
	if centerWidth > 0 {
		centerPad = max(1, (b.width-centerWidth)/2-leftWidth)
	} else {
		center = ""
	}

	rightPad := max(1, b.width-leftWidth-centerPad-centerWidth-rightWidth)
	return util.ClipRow(left+strings.Repeat(" ", centerPad)+center+strings.Repeat(" ", rightPad)+right, b.width)
}

// SetSize implements Widget.
func (b *Bar) SetSize(width, height int) {
	b.width = width
	// height is ignored - bars are always 1 line
}

func (b *Bar) MeasureHeight(width, limit int) int {
	if ansi.StringWidth(b.content.Left) > 0 ||
		ansi.StringWidth(b.content.Center) > 0 ||
		ansi.StringWidth(b.content.Right) > 0 {
		return min(1, limit)
	}
	return 0 // Hidden if no content
}

func (b *Bar) MinimumSize() image.Point { return image.Point{} }
