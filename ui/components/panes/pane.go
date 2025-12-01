package panes

// Pane represents a named buffer that can be shown/hidden
type Pane struct {
	Name    string
	Lines   []string
	Visible bool
	Height  int // Number of lines to show when visible
}
