package input

import (
	"unicode/utf8"

	"github.com/mmcdole/rune/text"
)

// ValidCommandText accepts command lines separated by newlines, with tabs
// allowed in arguments. Invalid UTF-8 and terminal controls are rejected.
func ValidCommandText(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		if text.RequiresTerminalProjection(r) {
			return false
		}
	}
	return true
}

// RequiresStructuredEditor reports whether value contains text that the ordinary
// single-line command input cannot admit without losing data or rendering
// terminal-active controls. The canonical value remains unchanged; callers
// use this to choose the lossless editor, independently of submission mode.
func RequiresStructuredEditor(value string) bool {
	for _, r := range value {
		if text.RequiresTerminalProjection(r) {
			return true
		}
	}
	return false
}
