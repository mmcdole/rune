package widget

import (
	"fmt"

	"github.com/mmcdole/rune/ui/tui/util"
)

// Pane represents a named buffer that can be shown/hidden.
//
// Lines are stored as written (logical lines) and soft-wrapped to the
// requested width when ContentRows is called, so a resize re-fits
// everything. Scrolling is tracked as a logical-line offset from the
// newest line; while scrolled the view stays anchored on the same
// history and new writes are counted for the title indicator.
type Pane struct {
	Name     string
	Lines    []string
	Visible  bool
	offset   int // logical lines scrolled back from the newest (0 = live)
	newLines int // writes that arrived while scrolled
}

// NewPane creates a new pane.
func NewPane(name string) *Pane {
	return &Pane{
		Name:    name,
		Lines:   make([]string, 0, 100),
		Visible: false,
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
	panes map[string]*Pane
}

// NewPaneManager creates a new pane manager.
func NewPaneManager() *PaneManager {
	return &PaneManager{
		panes: make(map[string]*Pane),
	}
}

// Create creates a new pane.
func (pm *PaneManager) Create(name string) {
	if _, exists := pm.panes[name]; exists {
		return
	}
	pm.panes[name] = NewPane(name)
}

// Get returns a pane by name, creating it if needed.
func (pm *PaneManager) Get(name string) *Pane {
	if _, exists := pm.panes[name]; !exists {
		pm.Create(name)
	}
	return pm.panes[name]
}

// Lookup returns an existing pane without creating it.
func (pm *PaneManager) Lookup(name string) (*Pane, bool) {
	pane, ok := pm.panes[name]
	return pane, ok
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
