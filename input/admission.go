package input

import (
	"strings"
	"unicode/utf8"

	"github.com/mmcdole/rune/text"
)

// ValidCommandText admits a single command. Local /commands may carry multiline,
// tab-indented arguments (for example Lua source); game input stays one line.
// Neither form admits invalid UTF-8 or terminal-active controls.
func ValidCommandText(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	// Match the dispatcher's ^/(%S+) prefix. Lua's whitespace class is ASCII.
	local := len(value) > 1 && value[0] == '/' && !strings.ContainsRune(" \t\r\n\v\f", rune(value[1]))
	for _, r := range value {
		if local && (r == '\n' || r == '\r' || r == '\t') {
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
