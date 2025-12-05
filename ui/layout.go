package ui

// LayoutEntry represents a single component in a layout dock.
type LayoutEntry struct {
	Name   string // Component name (e.g., "input", "status", pane name)
	Height int    // Explicit height in lines (0 = intrinsic/auto)
}

// LayoutConfig declares which components go in each dock.
// Both docks can contain any mix of bars, panes, and built-in components.
//
// Built-in component names:
//   - "input": the input line (1 line)
//   - "status": the status bar (1 line, fallback if no Lua bar registered)
//   - "separator": a horizontal line (1 line)
//
// If no layout provider is set, the default layout is:
//
//	Bottom: []LayoutEntry{{Name: "input"}, {Name: "status"}}
type LayoutConfig struct {
	Top    []LayoutEntry // Components for top dock (rendered above viewport)
	Bottom []LayoutEntry // Components for bottom dock (rendered below viewport)
}

// DefaultLayoutConfig returns the default layout with just input and status.
func DefaultLayoutConfig() LayoutConfig {
	return LayoutConfig{
		Bottom: []LayoutEntry{{Name: "input"}, {Name: "status"}},
	}
}
