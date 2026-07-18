package widget

import (
	"fmt"
	"strings"

	"github.com/mmcdole/rune/ui/tui/style"
	"github.com/mmcdole/rune/ui/tui/util"
)

// Compile-time check that Pane implements Widget
var _ Widget = (*Pane)(nil)

// Pane represents a named buffer that can be shown/hidden.
//
// Lines are stored as written (logical lines) and soft-wrapped to the
// pane width at render time, so a resize re-fits everything. Scrolling
// is tracked as a logical-line offset from the newest line; while
// scrolled the view stays anchored on the same history, new writes are
// counted, and the header shows a scroll indicator.
type Pane struct {
	Name     string
	Lines    []string
	Visible  bool
	height   int // Number of content lines to show when visible
	styles   style.Styles
	width    int
	offset   int // logical lines scrolled back from the newest (0 = live)
	newLines int // writes that arrived while scrolled
}

// NewPane creates a new pane widget.
func NewPane(name string, styles style.Styles) *Pane {
	return &Pane{
		Name:    name,
		Lines:   make([]string, 0, 100),
		Visible: false,
		height:  10,
		styles:  styles,
	}
}

// visibleRows renders exactly p.height rows of wrapped content for the
// current scroll position. The window is anchored at the logical line
// end = len(Lines)-offset; when a deep scroll leaves it underfull, it
// extends forward so the pane stays full whenever the buffer allows.
func (p *Pane) visibleRows() []string {
	end := len(p.Lines) - p.offset
	if end < 0 {
		end = 0
	}

	var rows []string
	for i := end - 1; i >= 0 && len(rows) < p.height; i-- {
		rows = append(util.WrapLine(p.Lines[i], p.width), rows...)
	}

	if len(rows) >= p.height {
		rows = rows[len(rows)-p.height:]
	} else {
		for i := end; i < len(p.Lines) && len(rows) < p.height; i++ {
			rows = append(rows, util.WrapLine(p.Lines[i], p.width)...)
		}
		if len(rows) > p.height {
			rows = rows[:p.height]
		}
	}

	for len(rows) < p.height {
		rows = append(rows, "")
	}
	return rows
}

// View implements Widget.
func (p *Pane) View() string {
	if !p.Visible {
		return ""
	}

	var parts []string

	// Header, with a scroll indicator while off the live tail
	// (mirrors the status bar's SCROLL/LIVE vocabulary).
	label := " " + p.Name + " "
	if p.offset > 0 {
		if p.newLines > 0 {
			label = fmt.Sprintf(" %s · scroll +%d ", p.Name, p.newLines)
		} else {
			label = " " + p.Name + " · scroll "
		}
	}
	title := p.styles.PaneHeader.Render(label)
	titlePad := p.width - util.VisibleLen(title)
	if titlePad > 0 {
		title += p.styles.PaneBorder.Render(strings.Repeat("─", titlePad))
	}
	parts = append(parts, title)
	parts = append(parts, p.visibleRows()...)

	// Bottom border
	parts = append(parts, p.styles.PaneBorder.Render(strings.Repeat("─", p.width)))

	return strings.Join(parts, "\n")
}

// SetSize implements Widget.
func (p *Pane) SetSize(width, height int) {
	p.width = width
	// Height includes header (1) + border (1), so content height = height - 2
	if height > 2 {
		p.height = height - 2
	} else if height > 0 {
		p.height = height
	}
}

// PreferredHeight implements Widget. Returns 0 if hidden.
func (p *Pane) PreferredHeight() int {
	if !p.Visible {
		return 0
	}
	return p.height + 2 // content + header + border
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
	p.offset += n
	p.clampOffset()
}

// ScrollDown scrolls forward by n logical lines; reaching the newest
// line returns the pane to live tailing.
func (p *Pane) ScrollDown(n int) {
	p.offset -= n
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

// SetVisible shows or hides the pane. Visibility never touches scroll
// state: a pane hidden on the live tail reopens live, a scrolled pane
// reopens anchored where it was (Write keeps the anchor as the buffer
// grows, and clampOffset pins it to the oldest line if trimming
// removes the history it pointed at).
func (p *Pane) SetVisible(visible bool) {
	p.Visible = visible
}

// Toggle toggles visibility.
func (p *Pane) Toggle() {
	p.Visible = !p.Visible
}

// Clear empties the pane.
func (p *Pane) Clear() {
	p.Lines = p.Lines[:0]
	p.offset = 0
	p.newLines = 0
}

// PaneManager handles multiple named panes.
type PaneManager struct {
	panes  map[string]*Pane
	styles style.Styles
}

// NewPaneManager creates a new pane manager.
func NewPaneManager(styles style.Styles) *PaneManager {
	return &PaneManager{
		panes:  make(map[string]*Pane),
		styles: styles,
	}
}

// Create creates a new pane.
func (pm *PaneManager) Create(name string) {
	if _, exists := pm.panes[name]; exists {
		return
	}
	pm.panes[name] = NewPane(name, pm.styles)
}

// Get returns a pane by name, creating it if needed.
func (pm *PaneManager) Get(name string) *Pane {
	if _, exists := pm.panes[name]; !exists {
		pm.Create(name)
	}
	return pm.panes[name]
}

// Write appends a line to a pane (auto-creates if missing).
func (pm *PaneManager) Write(name, text string) {
	pm.Get(name).Write(text)
}

// Toggle toggles pane visibility.
func (pm *PaneManager) Toggle(name string) {
	if pane, exists := pm.panes[name]; exists {
		pane.Toggle()
	}
}

// SetVisible shows or hides a pane.
func (pm *PaneManager) SetVisible(name string, visible bool) {
	if pane, exists := pm.panes[name]; exists {
		pane.SetVisible(visible)
	}
}

// Clear clears a pane.
func (pm *PaneManager) Clear(name string) {
	if pane, exists := pm.panes[name]; exists {
		pane.Clear()
	}
}

// Exists returns true if a pane exists.
func (pm *PaneManager) Exists(name string) bool {
	_, ok := pm.panes[name]
	return ok
}
