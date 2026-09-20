package widget

import (
	"fmt"
	"image"
	"strings"

	"github.com/mmcdole/rune/ui/tui/util"
)

// Pane is a named text buffer. Whether and where it appears on screen is
// placement state owned by the layout tree, never by the buffer.
//
// Lines are stored as written (logical lines) and soft-wrapped to the
// allocated width when View is called, so a resize re-fits
// everything. Scrolling is tracked as a logical-line offset from the
// newest line; while scrolled the view stays anchored on the same
// history and new writes are counted for the title indicator.
type Pane struct {
	name          string
	lines         []string
	offset        int // logical lines scrolled back from the newest (0 = live)
	newLines      int // writes that arrived while scrolled
	width, height int
}

// NewPane creates a new pane.
func NewPane(name string) *Pane {
	return &Pane{
		name:  name,
		lines: make([]string, 0, 100),
	}
}

func (p *Pane) SetSize(width, height int) { p.width, p.height = width, height }

func (p *Pane) MinimumSize() image.Point { return image.Point{} }

// MeasureHeight counts wrapped rows only until the layout's height budget is met.
func (p *Pane) MeasureHeight(width, limit int) int {
	if limit <= 0 {
		return 0
	}
	rows := 0
	for _, line := range p.lines {
		rows += len(util.WrapLine(line, max(1, width)))
		if rows >= limit {
			return limit
		}
	}
	return rows
}

// Title returns the unstyled pane title for its current scroll state.
func (p *Pane) Title() string {
	if p.offset == 0 {
		return p.name
	}
	if p.newLines > 0 {
		return fmt.Sprintf("%s · scroll +%d", p.name, p.newLines)
	}
	return p.name + " · scroll"
}

// View returns exactly the allocated height of wrapped content for the current
// scroll position. The window is anchored at the logical line end =
// len(lines)-offset; when a deep scroll leaves it underfull, it extends
// forward so the pane stays full whenever the buffer allows. Visibility is a
// layout concern and does not affect the returned content.
func (p *Pane) View() string {
	width, height := p.width, p.height
	if height <= 0 {
		return ""
	}

	end := len(p.lines) - p.offset
	if end < 0 {
		end = 0
	}

	rows := make([]string, height)
	start := height
	for i := end - 1; i >= 0 && start > 0; i-- {
		wrapped := util.WrapLine(p.lines[i], width)
		n := min(start, len(wrapped))
		start -= n
		copy(rows[start:], wrapped[len(wrapped)-n:])
	}
	if start > 0 {
		// Deep scrolls extend forward to fill the window; sparse panes pad below.
		used := copy(rows, rows[start:])
		clear(rows[used:])
		for i := end; i < len(p.lines) && used < height; i++ {
			used += copy(rows[used:], util.WrapLine(p.lines[i], width))
		}
	}

	return strings.Join(rows, "\n")
}

// Write appends text as logical lines, one per line break. While
// scrolled, the view stays anchored on the same history (the offset
// grows with the buffer) and new lines are counted for the header
// indicator.
func (p *Pane) Write(text string) {
	for _, line := range util.SplitLines(text) {
		p.lines = append(p.lines, util.ExpandTabs(line))
		if p.offset > 0 {
			p.offset++
			p.newLines++
		}
	}
	if len(p.lines) > 1000 {
		p.lines = p.lines[len(p.lines)-500:]
		p.clampOffset()
	}
}

func (p *Pane) clampOffset() {
	max := len(p.lines) - 1
	if max < 0 {
		max = 0
	}
	if p.offset > max {
		p.offset = max
	}
	if p.offset <= 0 {
		p.offset = 0
		p.newLines = 0
	}
}

// ScrollUp scrolls back by n logical lines.
func (p *Pane) ScrollUp(n int) {
	if n <= 0 {
		return
	}
	maxOffset := max(0, len(p.lines)-1)
	if n >= maxOffset-p.offset {
		p.offset = maxOffset
	} else {
		p.offset += n
	}
	p.clampOffset()
}

// ScrollDown scrolls forward by n logical lines; reaching the newest
// line returns the pane to live tailing.
func (p *Pane) ScrollDown(n int) {
	if n <= 0 {
		return
	}
	if n >= p.offset {
		p.offset = 0
	} else {
		p.offset -= n
	}
	p.clampOffset()
}

// ScrollToTop jumps to the oldest line.
func (p *Pane) ScrollToTop() {
	p.offset = len(p.lines) - 1
	p.clampOffset()
}

// ScrollToBottom returns to live tailing.
func (p *Pane) ScrollToBottom() {
	p.offset = 0
	p.newLines = 0
}

// Clear empties the pane.
func (p *Pane) Clear() {
	p.lines = p.lines[:0]
	p.offset = 0
	p.newLines = 0
}
