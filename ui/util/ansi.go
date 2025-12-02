package util

import (
	"strings"

	"github.com/drake/rune/mud"
)

// VisibleLen returns the visible length of a string (excluding ANSI codes).
func VisibleLen(s string) int {
	return len(mud.StripANSI(s))
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
