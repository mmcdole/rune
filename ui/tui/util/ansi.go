package util

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// VisibleLen returns the visible display width of a string (excluding ANSI codes).
func VisibleLen(s string) int {
	return ansi.StringWidth(s)
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
	var rows []string
	for _, row := range strings.Split(ansi.Wrap(line, width, ""), "\n") {
		if VisibleLen(row) <= width {
			rows = append(rows, row)
		} else {
			// The ANSI word wrapper can split ASCII-led graphemes (keycap
			// emoji). Enforce the compositor's cell budget on those rows.
			rows = append(rows, wrapCells(row, width)...)
		}
	}
	return rows
}

func wrapCells(s string, width int) []string {
	var rows []string
	var row strings.Builder
	column := 0
	var state byte
	for len(s) > 0 {
		var token string
		cells, n := 0, 0
		if state == 0 && s[0] >= ' ' && s[0] != 0x7f {
			token, cells = ansi.FirstGraphemeCluster(s, ansi.GraphemeWidth)
			n = len(token)
		} else {
			token, cells, n, state = ansi.DecodeSequence(s, state, nil)
		}
		s = s[n:]
		if cells > width {
			token, cells = "�", 1
		}
		if cells > 0 && column+cells > width {
			rows = append(rows, row.String())
			row.Reset()
			column = 0
		}
		row.WriteString(token)
		column += cells
	}
	return append(rows, row.String())
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
