package widget

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Separator renders a horizontal line.
type Separator struct {
	width int
}

// NewSeparator creates a new separator widget.
func NewSeparator() *Separator {
	return &Separator{}
}

// Init implements tea.Model.
func (s *Separator) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (s *Separator) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return s, nil
}

// View implements tea.Model.
func (s *Separator) View() string {
	return "\x1b[90m" + strings.Repeat("─", s.width) + "\x1b[0m"
}

// SetWidth implements Widget.
func (s *Separator) SetWidth(w int) {
	s.width = w
}

// Height implements Widget.
func (s *Separator) Height() int {
	return 1
}
