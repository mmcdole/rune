package widget

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/mmcdole/rune/ui/tui/util"
)

// clipRow hard-truncates one row to the given width (ANSI-aware).
// Every widget row must fit the terminal width: an overlong row wraps
// at the terminal, adds a phantom physical line, and scrolls the whole
// frame - corrupting the layout for everything above it.
func clipRow(s string, width int) string {
	if width < 1 || util.VisibleLen(s) <= width {
		return s
	}
	return ansi.Truncate(s, width, "")
}
