package input

import "github.com/mmcdole/rune/text"

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
