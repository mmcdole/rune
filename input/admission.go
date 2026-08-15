package input

import (
	"unicode/utf8"

	"github.com/mmcdole/rune/text"
)

// ValidCommandText reports whether value can safely retain command-mode
// interpretation across submission, history, and the one-line input widget.
func ValidCommandText(value string) bool {
	return utf8.ValidString(value) && !RequiresVerbatim(value)
}

// RequiresVerbatim reports whether value contains text that the ordinary
// single-line command input cannot admit without losing data or rendering
// terminal-active controls. The canonical value remains unchanged; callers
// use this only to choose the lossless verbatim editor/submission mode.
func RequiresVerbatim(value string) bool {
	for _, r := range value {
		if text.RequiresTerminalProjection(r) {
			return true
		}
	}
	return false
}
