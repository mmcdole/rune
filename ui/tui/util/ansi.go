package util

import (
	"strings"

	"github.com/drake/rune/text"
	"github.com/mattn/go-runewidth"
)

// VisibleLen returns the visible display width of a string (excluding ANSI codes).
func VisibleLen(s string) int {
	return runewidth.StringWidth(text.StripANSI(s))
}

// FilterClearSequences removes ANSI sequences that would clear the screen.
// MUD clients typically ignore these to prevent server-side screen wipes.
func FilterClearSequences(line string) string {
	// Filter clear screen sequences
	line = strings.ReplaceAll(line, "\x1b[2J", "")   // Clear entire screen
	line = strings.ReplaceAll(line, "\x1b[H", "")    // Move cursor to home
	line = strings.ReplaceAll(line, "\x1b[0;0H", "") // Move cursor to 0,0
	line = strings.ReplaceAll(line, "\x1b[1;1H", "") // Move cursor to 1,1
	return line
}
