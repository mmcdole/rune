package ui

import (
	"strings"
)

// Pane represents a named buffer that can be shown/hidden
type Pane struct {
	Name    string
	Lines   []string
	Visible bool
	Height  int // Number of lines to show when visible
}

// PaneManager handles multiple named panes
type PaneManager struct {
	panes    map[string]*Pane
	keyBinds map[string]string // key -> pane name
	styles   Styles
	width    int
}

// NewPaneManager creates a new pane manager
func NewPaneManager(styles Styles) *PaneManager {
	return &PaneManager{
		panes:    make(map[string]*Pane),
		keyBinds: make(map[string]string),
		styles:   styles,
	}
}

// SetWidth updates the rendering width
func (pm *PaneManager) SetWidth(w int) {
	pm.width = w
}

// Create creates a new pane with the given name
func (pm *PaneManager) Create(name string) {
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
func (pm *PaneManager) Write(name, text string) {
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
func (pm *PaneManager) Toggle(name string) {
	pane, exists := pm.panes[name]
	if !exists {
		return
	}
	pane.Visible = !pane.Visible
}

// Clear clears the contents of the named pane
func (pm *PaneManager) Clear(name string) {
	pane, exists := pm.panes[name]
	if !exists {
		return
	}
	pane.Lines = pane.Lines[:0]
}

// BindKey binds a key to toggle a pane
func (pm *PaneManager) BindKey(key, name string) {
	pm.keyBinds[key] = name
}

// HandleKey checks if a key is bound and toggles the pane
// Returns true if the key was handled
func (pm *PaneManager) HandleKey(key string) bool {
	name, exists := pm.keyBinds[key]
	if !exists {
		return false
	}
	pm.Toggle(name)
	return true
}

// DebugBindings returns a string showing all key bindings (for debugging)
func (pm *PaneManager) DebugBindings() string {
	if len(pm.keyBinds) == 0 {
		return "No key bindings"
	}
	result := "Key bindings: "
	for k, v := range pm.keyBinds {
		result += "[" + k + "]=" + v + " "
	}
	return result
}

// GetLines returns a copy of the lines from a named pane.
func (pm *PaneManager) GetLines(name string) []string {
	pane, exists := pm.panes[name]
	if !exists {
		return nil
	}
	// Return a copy
	result := make([]string, len(pane.Lines))
	copy(result, pane.Lines)
	return result
}

// HasVisiblePane returns true if any pane is visible
func (pm *PaneManager) HasVisiblePane() bool {
	for _, pane := range pm.panes {
		if pane.Visible {
			return true
		}
	}
	return false
}

// VisibleHeight returns the total height of all visible panes
func (pm *PaneManager) VisibleHeight() int {
	height := 0
	for _, pane := range pm.panes {
		if pane.Visible {
			height += pane.Height + 2 // +1 for header, +1 for bottom border
		}
	}
	return height
}

// View renders all visible panes
func (pm *PaneManager) View() string {
	var parts []string

	for _, pane := range pm.panes {
		if !pane.Visible {
			continue
		}

		// Header line with box drawing
		header := pm.styles.PaneHeader.Render(" " + pane.Name + " ")
		headerPad := pm.width - len(stripAnsi(header))
		if headerPad > 0 {
			header += pm.styles.PaneBorder.Render(strings.Repeat("─", headerPad))
		}
		parts = append(parts, header)

		// Content - show last N lines
		startIdx := 0
		if len(pane.Lines) > pane.Height {
			startIdx = len(pane.Lines) - pane.Height
		}

		linesShown := 0
		for i := startIdx; i < len(pane.Lines); i++ {
			parts = append(parts, pane.Lines[i])
			linesShown++
		}

		// Show "(empty)" if pane has no content
		if len(pane.Lines) == 0 {
			parts = append(parts, pm.styles.Muted.Render("  (empty)"))
			linesShown++
		}

		// Pad if not enough lines
		for i := linesShown; i < pane.Height; i++ {
			parts = append(parts, "")
		}

		// Bottom border
		parts = append(parts, pm.styles.PaneBorder.Render(strings.Repeat("─", pm.width)))
	}

	return strings.Join(parts, "\n")
}
