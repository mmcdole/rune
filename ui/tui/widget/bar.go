package widget

import (
	"image"
	"strings"

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

// NewBar creates a new bar renderer.
func NewBar() *Bar {
	return &Bar{}
}

// SetContent updates the bar's content.
func (b *Bar) SetContent(content ui.BarContent) {
	b.content = content
}

// View implements Widget.
func (b *Bar) View() string {
	left := b.content.Left
	center := b.content.Center
	right := b.content.Right

	leftLen := util.VisibleLen(left)
	centerLen := util.VisibleLen(center)
	rightLen := util.VisibleLen(right)

	if centerLen > 0 {
		// Three-part layout
		leftPad := (b.width-centerLen)/2 - leftLen
		if leftPad < 1 {
			leftPad = 1
		}
		rightPad := b.width - leftLen - leftPad - centerLen - rightLen
		if rightPad < 1 {
			rightPad = 1
		}
		return clipRow(left+strings.Repeat(" ", leftPad)+center+strings.Repeat(" ", rightPad)+right, b.width)
	}

	// Two-part layout
	pad := b.width - leftLen - rightLen
	if pad < 1 {
		pad = 1
	}
	return clipRow(left+strings.Repeat(" ", pad)+right, b.width)
}

// SetSize implements Widget.
func (b *Bar) SetSize(width, height int) {
	b.width = width
	// height is ignored - bars are always 1 line
}

func (b *Bar) MeasureHeight(width, limit int) int {
	if util.VisibleLen(b.content.Left) > 0 ||
		util.VisibleLen(b.content.Center) > 0 ||
		util.VisibleLen(b.content.Right) > 0 {
		return min(1, limit)
	}
	return 0 // Hidden if no content
}

func (b *Bar) MinimumSize() image.Point { return image.Point{} }
