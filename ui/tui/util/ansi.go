package util

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
	"github.com/mmcdole/rune/text"
)

// VisibleLen returns the visible display width of a string (excluding ANSI codes).
func VisibleLen(s string) int {
	return runewidth.StringWidth(text.StripANSI(s))
}

// SplitLines splits text into lines, treating lone CR and CRLF as
// line breaks.
func SplitLines(s string) []string {
	if !strings.ContainsAny(s, "\r\n") {
		return []string{s}
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Split(s, "\n")
}

// WrapLine soft-wraps one line into rows of at most width columns,
// returning at least one row. ANSI codes and wide runes are handled. A
// line that fits, or a width below 1, passes through unchanged. The
// byte-length check is a fast bound: a rune's display width never
// exceeds its byte count.
func WrapLine(line string, width int) []string {
	if width < 1 || len(line) <= width || VisibleLen(line) <= width {
		return []string{line}
	}
	return strings.Split(ansi.Wrap(line, width, ""), "\n")
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

// tabStop is the classic terminal tab width.
const tabStop = 8

// ExpandTabs replaces each tab with spaces up to the next 8-column tab
// stop, measured in visible cells (ANSI sequences are zero-width). A raw
// \t must never reach the renderer: bubbletea repaints only rows that
// changed, and a tab makes the terminal skip cells without erasing them,
// resurrecting content from the previous frame as ghost columns.
func ExpandTabs(line string) string {
	if !strings.Contains(line, "\t") {
		return line
	}
	var b strings.Builder
	col := 0
	for {
		i := strings.IndexByte(line, '\t')
		if i < 0 {
			b.WriteString(line)
			return b.String()
		}
		seg := line[:i]
		b.WriteString(seg)
		col += VisibleLen(seg)
		pad := tabStop - col%tabStop
		b.WriteString(strings.Repeat(" ", pad))
		col += pad
		line = line[i+1:]
	}
}
