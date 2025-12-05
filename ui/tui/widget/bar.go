package widget

import (
	"strings"

	"github.com/drake/rune/ui"
	"github.com/drake/rune/ui/tui/util"
)

// Compile-time check that Bar implements Widget
var _ Widget = (*Bar)(nil)

// Bar renders a Lua-defined bar with left/center/right sections.
type Bar struct {
	name    string
	content ui.BarContent
	width   int
}

// NewBar creates a new bar renderer.
func NewBar(name string) *Bar {
	return &Bar{
		name: name,
	}
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

	if center != "" {
		// Three-part layout
		leftPad := (b.width-centerLen)/2 - leftLen
		if leftPad < 1 {
			leftPad = 1
		}
		rightPad := b.width - leftLen - leftPad - centerLen - rightLen
		if rightPad < 1 {
			rightPad = 1
		}
		return left + strings.Repeat(" ", leftPad) + center + strings.Repeat(" ", rightPad) + right
	}

	// Two-part layout
	pad := b.width - leftLen - rightLen
	if pad < 1 {
		pad = 1
	}
	return left + strings.Repeat(" ", pad) + right
}

// SetSize implements Widget.
func (b *Bar) SetSize(width, height int) {
	b.width = width
	// height is ignored - bars are always 1 line
}

// PreferredHeight implements Widget.
func (b *Bar) PreferredHeight() int {
	if b.content.Left != "" || b.content.Center != "" || b.content.Right != "" {
		return 1
	}
	return 0 // Hidden if no content
}
