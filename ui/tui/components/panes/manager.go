package panes

import (
	"strings"

	"github.com/drake/rune/ui/tui/style"
	"github.com/drake/rune/ui/tui/util"
)

// Manager handles multiple named panes
type Manager struct {
	panes  map[string]*Pane
	styles style.Styles
}

// NewManager creates a new pane manager
func NewManager(styles style.Styles) *Manager {
	return &Manager{
		panes:  make(map[string]*Pane),
		styles: styles,
	}
}

// Create creates a new pane with the given name
func (pm *Manager) Create(name string) {
	if _, exists := pm.panes[name]; exists {
		return // Already exists
	}
	pm.panes[name] = &Pane{
		Name:    name,
		Lines:   make([]string, 0, 100),
		Visible: false,
		Height:  10, // Default height
	}
}

// Write appends a line to the named pane
func (pm *Manager) Write(name, text string) {
	pane, exists := pm.panes[name]
	if !exists {
		// Auto-create if doesn't exist
		pm.Create(name)
		pane = pm.panes[name]
	}

	// Append line, limit buffer size
	pane.Lines = append(pane.Lines, text)
	if len(pane.Lines) > 1000 {
		// Keep last 500 lines when buffer exceeds 1000
		pane.Lines = pane.Lines[len(pane.Lines)-500:]
	}
}

// Toggle toggles visibility of the named pane
func (pm *Manager) Toggle(name string) {
	pane, exists := pm.panes[name]
	if !exists {
		return
	}
	pane.Visible = !pane.Visible
}

// Clear clears the contents of the named pane
func (pm *Manager) Clear(name string) {
	pane, exists := pm.panes[name]
	if !exists {
		return
	}
	pane.Lines = pane.Lines[:0]
}

// GetHeight returns the render height of a specific pane (if visible).
// Returns 0 if pane doesn't exist or is hidden.
func (pm *Manager) GetHeight(name string) int {
	pane, ok := pm.panes[name]
	if !ok || !pane.Visible {
		return 0
	}
	// Height + 1 (Header) + 1 (Bottom Border)
	return pane.Height + 2
}

// RenderPane returns the rendered string for a single pane.
// Returns empty string if pane doesn't exist or is hidden.
func (pm *Manager) RenderPane(name string, width int) string {
	pane, ok := pm.panes[name]
	if !ok || !pane.Visible {
		return ""
	}

	var parts []string

	// 1. Header
	title := pm.styles.PaneHeader.Render(" " + pane.Name + " ")
	titlePad := width - util.VisibleLen(title)
	if titlePad > 0 {
		title += pm.styles.PaneBorder.Render(strings.Repeat("─", titlePad))
	}
	parts = append(parts, title)

	// 2. Content (last N lines)
	lines := pane.Lines
	height := pane.Height

	start := 0
	if len(lines) > height {
		start = len(lines) - height
	}

	for i := 0; i < height; i++ {
		lineIdx := start + i
		if lineIdx < len(lines) {
			parts = append(parts, lines[lineIdx])
		} else {
			parts = append(parts, "") // Empty padding
		}
	}

	// 3. Bottom border
	parts = append(parts, pm.styles.PaneBorder.Render(strings.Repeat("─", width)))

	return strings.Join(parts, "\n")
}
