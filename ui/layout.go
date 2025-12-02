package ui

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
//	Bottom: []string{"input", "status"}
type LayoutConfig struct {
	Top    []string // Components for top dock (rendered above viewport)
	Bottom []string // Components for bottom dock (rendered below viewport)
}

// DefaultLayoutConfig returns the default layout with just input and status.
func DefaultLayoutConfig() LayoutConfig {
	return LayoutConfig{
		Bottom: []string{"input", "status"},
	}
}
