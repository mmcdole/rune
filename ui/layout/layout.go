package layout

// Config declares which components go in each dock.
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
type Config struct {
	Top    []string // Components for top dock (rendered above viewport)
	Bottom []string // Components for bottom dock (rendered below viewport)
}

// DefaultConfig returns the default layout with just input and status.
func DefaultConfig() Config {
	return Config{
		Bottom: []string{"input", "status"},
	}
}
