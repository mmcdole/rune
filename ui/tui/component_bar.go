package tui

import (
	"strings"

	"github.com/drake/rune/ui"
	"github.com/drake/rune/ui/tui/util"
)

// BarComponent adapts a Lua-defined bar for the Component interface.
// It looks up rendered content from the barContent map.
type BarComponent struct {
	name    string
	content *map[string]ui.BarContent
	width   *int
}

// Height returns 1 since bars are always single-line.
func (b *BarComponent) Height() int {
	return 1
}

// Render returns the bar content with left/center/right alignment.
func (b *BarComponent) Render(width int) string {
	content, ok := (*b.content)[b.name]
	if !ok {
		return ""
	}

	left := content.Left
	center := content.Center
	right := content.Right

	leftLen := util.VisibleLen(left)
	centerLen := util.VisibleLen(center)
	rightLen := util.VisibleLen(right)

	// Calculate spacing
	if center != "" {
		// Three-part layout: left ... center ... right
		leftPad := (width-centerLen)/2 - leftLen
		if leftPad < 1 {
			leftPad = 1
		}
		rightPad := width - leftLen - leftPad - centerLen - rightLen
		if rightPad < 1 {
			rightPad = 1
		}
		return left + strings.Repeat(" ", leftPad) + center + strings.Repeat(" ", rightPad) + right
	}

	// Two-part layout: left ... right
	pad := width - leftLen - rightLen
	if pad < 1 {
		pad = 1
	}
	return left + strings.Repeat(" ", pad) + right
}
