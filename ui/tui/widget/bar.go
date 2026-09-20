package widget

import (
	"image"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/util"
)

// Bar renders a Lua-defined bar with left/center/right sections.
type Bar struct {
	content ui.BarContent
	width   int
}

// SetContent reports changed content and changed visibility separately.
func (b *Bar) SetContent(content ui.BarContent) (changed, layout bool) {
	if b.content == content {
		return false, false
	}
	before := b.MeasureHeight(0, 1)
	b.content = content
	return true, before != b.MeasureHeight(0, 1)
}

// View renders the bar at its allocated width.
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

// SetSize applies the allocated content size.
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
