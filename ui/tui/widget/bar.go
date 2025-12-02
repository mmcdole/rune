package widget

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/drake/rune/ui"
	"github.com/drake/rune/ui/tui/layout"
	"github.com/drake/rune/ui/tui/util"
)

// Compile-time check that Bar implements layout.Renderer
var _ layout.Renderer = (*Bar)(nil)

// Bar renders a Lua-defined bar with left/center/right sections.
// Implements layout.Renderer for use by the layout engine.
type Bar struct {
	name    string
	content *map[string]ui.BarContent
	width   int
}

// NewBar creates a new bar renderer.
func NewBar(name string, content *map[string]ui.BarContent) *Bar {
	return &Bar{
		name:    name,
		content: content,
	}
}

// Init implements tea.Model.
func (b *Bar) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (b *Bar) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return b, nil
}

// View implements layout.Renderer.
func (b *Bar) View() string {
	content, ok := (*b.content)[b.name]
	if !ok {
		return "" // Safe guard if content removed between frames
	}

	left := content.Left
	center := content.Center
	right := content.Right

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

// SetWidth implements layout.Renderer.
func (b *Bar) SetWidth(w int) {
	b.width = w
}

// Height implements layout.Renderer.
func (b *Bar) Height() int {
	if _, ok := (*b.content)[b.name]; ok {
		return 1
	}
	return 0 // Hidden if no content
}
