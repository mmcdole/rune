package layout

// Config declares which components go in each dock.
// Both docks can contain any mix of bars, panes, and built-in components.
//
// Built-in component names:
//   - "input": the input line (1 line)
//   - "status": the status bar (1 line)
//   - "separator": a horizontal line (1 line)
//   - "infobar": Lua-controlled info line, only shown if set (0-1 lines)
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

// BarDef defines a single-line bar component.
type BarDef struct {
	Name   string
	Border Border
	Render func(state ClientState, width int) BarContent
}

// BarContent holds the rendered content of a bar.
type BarContent struct {
	Left   string
	Center string
	Right  string
}

// PaneDef defines a multi-line pane component.
type PaneDef struct {
	Name       string
	Height     int    // Fixed height in lines
	Visible    bool   // Can be toggled
	BufferSize int    // Max lines retained in buffer
	Border     Border // Decorative borders
	Title      bool   // Show name as header
}

// Border specifies decorative borders around a component.
type Border string

const (
	BorderNone   Border = ""
	BorderTop    Border = "top"
	BorderBottom Border = "bottom"
	BorderBoth   Border = "both"
)

// ClientState holds state available to bar render functions.
type ClientState struct {
	Connected   bool
	Address     string
	ScrollMode  string // "live" or "scroll"
	ScrollLines int    // Lines behind live (when scrolled)
}

// Provider is the interface the UI uses to get layout information.
type Provider interface {
	// Layout returns the current layout configuration.
	Layout() Config

	// Bar returns the bar definition for a name, or nil if not found.
	Bar(name string) *BarDef

	// Pane returns the pane definition for a name, or nil if not found.
	Pane(name string) *PaneDef

	// PaneLines returns the current buffer contents for a pane.
	PaneLines(name string) []string

	// State returns the current client state for bar rendering.
	State() ClientState
}
