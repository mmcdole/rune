package widget

import (
	"fmt"

	"github.com/mmcdole/rune/ui/tui/util"
)

// Pane is a named text buffer. Whether and where it appears on screen is
// placement state owned by the layout tree, never by the buffer.
//
// Lines are stored as written (logical lines) and soft-wrapped to the
// requested width when ContentRows is called, so a resize re-fits
// everything. Scrolling is tracked as a logical-line offset from the
// newest line; while scrolled the view stays anchored on the same
// history and new writes are counted for the title indicator.
type Pane struct {
	Name     string
	Lines    []string
	offset   int // logical lines scrolled back from the newest (0 = live)
	newLines int // writes that arrived while scrolled
}

// NewPane creates a new pane.
func NewPane(name string) *Pane {
	return &Pane{
		Name:  name,
		Lines: make([]string, 0, 100),
	}
}

// Title returns the unstyled pane title for its current scroll state.
func (p *Pane) Title() string {
	if p.offset == 0 {
		return p.Name
	}
	if p.newLines > 0 {
		return fmt.Sprintf("%s · scroll +%d", p.Name, p.newLines)
	}
	return p.Name + " · scroll"
}

// ContentRows returns exactly height rows of wrapped content for the current
// scroll position. The window is anchored at the logical line end =
// len(Lines)-offset; when a deep scroll leaves it underfull, it extends
// forward so the pane stays full whenever the buffer allows. Visibility is a
// layout concern and does not affect the returned content.
func (p *Pane) ContentRows(width, height int) []string {
	if height <= 0 {
		return nil
	}

	end := len(p.Lines) - p.offset
	if end < 0 {
		end = 0
	}

	var rows []string
	for i := end - 1; i >= 0 && len(rows) < height; i-- {
		rows = append(util.WrapLine(p.Lines[i], width), rows...)
	}

	if len(rows) >= height {
		rows = rows[len(rows)-height:]
	} else {
		for i := end; i < len(p.Lines) && len(rows) < height; i++ {
			rows = append(rows, util.WrapLine(p.Lines[i], width)...)
		}
		if len(rows) > height {
			rows = rows[:height]
		}
	}

	for len(rows) < height {
		rows = append(rows, "")
	}
	return rows
}

// Write appends text as logical lines, one per line break. While
// scrolled, the view stays anchored on the same history (the offset
// grows with the buffer) and new lines are counted for the header
// indicator.
func (p *Pane) Write(text string) {
	for _, line := range util.SplitLines(text) {
		p.Lines = append(p.Lines, util.ExpandTabs(line))
		if p.offset > 0 {
			p.offset++
			p.newLines++
		}
	}
	if len(p.Lines) > 1000 {
		p.Lines = p.Lines[len(p.Lines)-500:]
		p.clampOffset()
	}
}

func (p *Pane) clampOffset() {
	max := len(p.Lines) - 1
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
	maxOffset := max(0, len(p.Lines)-1)
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
	p.offset = len(p.Lines) - 1
	p.clampOffset()
}

// ScrollToBottom returns to live tailing.
func (p *Pane) ScrollToBottom() {
	p.offset = 0
	p.newLines = 0
}

// Clear empties the pane.
func (p *Pane) Clear() {
	p.Lines = p.Lines[:0]
	p.offset = 0
	p.newLines = 0
}
