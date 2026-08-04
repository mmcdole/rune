package util

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/mmcdole/rune/text"
)

// ColRange is a half-open range of visible columns within a row.
type ColRange struct {
	Start, End int
}

// HighlightRange re-styles the visible columns [startCol, endCol) of an
// SGR-bearing row: the ambient style is reset before the highlighted
// text and restored after it. render receives the stripped match text.
//
// ansi.Cut with a left edge > 0 re-emits the SGR sequences seen before
// the cut point, so the suffix restores the row's ambient style without
// a hand-rolled SGR state machine. (CutWc in the pinned x/ansi
// truncates the right edge twice; stay on the grapheme-based Cut.)
// Columns must be measured with ansi.StringWidth, the same method Cut
// slices by, or wide runes misalign the splice.
func HighlightRange(row string, startCol, endCol int, render func(string) string) string {
	width := ansi.StringWidth(row)
	if startCol < 0 {
		startCol = 0
	}
	if endCol > width {
		endCol = width
	}
	if startCol >= endCol {
		return row
	}
	prefix := ansi.Cut(row, 0, startCol)
	mid := text.StripANSI(ansi.Cut(row, startCol, endCol))
	suffix := ansi.Cut(row, endCol, width)
	return prefix + "\x1b[0m" + render(mid) + suffix
}

// HighlightRanges applies HighlightRange to each of several sorted,
// non-overlapping ranges, splicing rightmost-first so earlier column
// offsets stay valid as the string grows.
func HighlightRanges(row string, ranges []ColRange, render func(string) string) string {
	for i := len(ranges) - 1; i >= 0; i-- {
		row = HighlightRange(row, ranges[i].Start, ranges[i].End, render)
	}
	return row
}
