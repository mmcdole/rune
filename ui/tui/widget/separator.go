package widget

import (
	"github.com/mattn/go-runewidth"

	"github.com/mmcdole/rune/ui/tui/style"
)

// Compile-time checks that Separator implements Widget and Configurable
var (
	_ Widget       = (*Separator)(nil)
	_ Configurable = (*Separator)(nil)
)

// Separator renders a horizontal line.
type Separator struct {
	width int
	char  string
}

// NewSeparator creates a new separator widget.
func NewSeparator() *Separator {
	return &Separator{}
}

// SetOptions implements Configurable. "char" sets the rule character;
// anything but a single-cell value is dropped so the rule can never
// break the width math.
func (s *Separator) SetOptions(opts map[string]string) {
	char := opts["char"]
	if runewidth.StringWidth(char) != 1 {
		char = ""
	}
	s.char = char
}

// View implements Widget.
func (s *Separator) View() string {
	return style.RenderBorder(s.width, s.char)
}

// SetSize implements Widget.
func (s *Separator) SetSize(width, height int) {
	s.width = width
}

// PreferredHeight implements Widget.
func (s *Separator) PreferredHeight() int {
	return 1
}
