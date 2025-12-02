package widget

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/drake/rune/ui/tui/layout"
	"github.com/drake/rune/ui/tui/style"
	"github.com/drake/rune/ui/tui/util"
)

// Compile-time check that Pane implements layout.Renderer
var _ layout.Renderer = (*Pane)(nil)

// Pane represents a named buffer that can be shown/hidden.
// Implements layout.Renderer for use by the layout engine.
type Pane struct {
	Name    string
	Lines   []string
	Visible bool
	height  int // Number of lines to show when visible (renamed to avoid conflict with Height method)
	styles  style.Styles
	width   int
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

// Init implements tea.Model.
func (p *Pane) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (p *Pane) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return p, nil
}

// View implements tea.Model.
func (p *Pane) View() string {
	if !p.Visible {
		return ""
	}

	var parts []string

	// Header
	title := p.styles.PaneHeader.Render(" " + p.Name + " ")
	titlePad := p.width - util.VisibleLen(title)
	if titlePad > 0 {
		title += p.styles.PaneBorder.Render(strings.Repeat("─", titlePad))
	}
	parts = append(parts, title)

	// Content (last N lines)
	start := 0
	if len(p.Lines) > p.height {
		start = len(p.Lines) - p.height
	}

	for i := 0; i < p.height; i++ {
		lineIdx := start + i
		if lineIdx < len(p.Lines) {
			parts = append(parts, p.Lines[lineIdx])
		} else {
			parts = append(parts, "")
		}
	}

	// Bottom border
	parts = append(parts, p.styles.PaneBorder.Render(strings.Repeat("─", p.width)))

	return strings.Join(parts, "\n")
}

// SetWidth implements layout.Renderer.
func (p *Pane) SetWidth(w int) {
	p.width = w
}

// Height implements layout.Renderer. Returns 0 if hidden.
func (p *Pane) Height() int {
	if !p.Visible {
		return 0
	}
	return p.height + 2 // content + header + border
}

// Write appends a line to the pane.
func (p *Pane) Write(text string) {
	p.Lines = append(p.Lines, text)
	if len(p.Lines) > 1000 {
		p.Lines = p.Lines[len(p.Lines)-500:]
	}
}

// Toggle toggles visibility.
func (p *Pane) Toggle() {
	p.Visible = !p.Visible
}

// Clear empties the pane.
func (p *Pane) Clear() {
	p.Lines = p.Lines[:0]
}

// SetContentHeight sets the display height (number of content lines).
func (p *Pane) SetContentHeight(h int) {
	if h > 0 {
		p.height = h
	}
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

// SetPaneHeight sets the display height for a pane.
func (pm *PaneManager) SetPaneHeight(name string, h int) {
	if pane, ok := pm.panes[name]; ok {
		pane.SetContentHeight(h)
	}
}
