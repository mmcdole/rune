package tui

import "github.com/mmcdole/rune/ui/tui/widget"

// pane is a named widget with text and scrolling. Output also owns its prompt
// and searchable scrollback.
type pane interface {
	widget.Widget
	Name() string
	Write(string)
	Clear()
	ScrollUp(int)
	ScrollDown(int)
	ScrollToTop()
	ScrollToBottom()
	Title() string
}

var (
	_ pane = (*widget.Output)(nil)
	_ pane = (*widget.Pane)(nil)
)

// pane creates a named pane on first use. The main output is installed in
// NewModel, so looking it up always returns the same Output widget.
func (m *Model) pane(name string) pane {
	if p, ok := m.panes[name]; ok {
		return p
	}
	p := widget.NewPane(name)
	m.panes[name] = p
	return p
}
