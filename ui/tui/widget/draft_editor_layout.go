package widget

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui/tui/util"
)

type draftGlyph struct {
	text  string
	width int
}

type draftPoint struct {
	offset int
	col    int
}

type draftRow struct {
	line         int
	continuation bool
	glyphs       []draftGlyph
	points       []draftPoint
}

type draftLayout struct {
	rows       []draftRow
	cursorRow  int
	cursorCol  int
	gutterSize int
}

// buildDraftLayout derives safe terminal rows from the canonical buffer.
// Source tabs remain one rune but expand to cells at classic 8-column stops.
// Every source insertion offset is retained on exactly one visual row so
// vertical movement and cursor rendering never need to reverse-map strings.
func buildDraftLayout(content []rune, width, lineCount, rowLimit int) draftLayout {
	gutter := draftGutterSize(lineCount, width)
	contentWidth := width - gutter
	if contentWidth < 1 {
		contentWidth = 1
	}

	layout := draftLayout{
		gutterSize: gutter,
	}

	line := 0
	lineStart := 0
	for {
		lineEnd := lineStart
		for lineEnd < len(content) && content[lineEnd] != '\n' {
			lineEnd++
		}

		layout.rows = append(layout.rows, draftRow{line: line})
		rowIndex := len(layout.rows) - 1
		col := 0
		logicalCol := 0

		newContinuation := func() {
			layout.rows = append(layout.rows, draftRow{line: line, continuation: true})
			rowIndex = len(layout.rows) - 1
			col = 0
		}
		addPoint := func(offset int) {
			layout.rows[rowIndex].points = append(layout.rows[rowIndex].points, draftPoint{offset: offset, col: col})
		}
		appendGlyph := func(g draftGlyph) {
			if g.width > contentWidth {
				g = draftGlyph{text: "�", width: 1}
			}
			if col > 0 && col+g.width > contentWidth {
				newContinuation()
			}
			if g.width == 0 && len(layout.rows[rowIndex].glyphs) > 0 {
				last := len(layout.rows[rowIndex].glyphs) - 1
				layout.rows[rowIndex].glyphs[last].text += g.text
				return
			}
			layout.rows[rowIndex].glyphs = append(layout.rows[rowIndex].glyphs, g)
			col += g.width
			logicalCol += g.width
		}

		remaining := string(content[lineStart:lineEnd])
		for offset := lineStart; offset < lineEnd; {
			if rowLimit > 0 && len(layout.rows) >= rowLimit {
				return layout
			}
			if col >= contentWidth {
				newContinuation()
			}
			r := content[offset]
			if r == '\t' {
				addPoint(offset)
				padding := 8 - logicalCol%8
				for n := 0; n < padding; n++ {
					if col >= contentWidth {
						newContinuation()
					}
					appendGlyph(draftGlyph{text: " ", width: 1})
				}
				offset++
				remaining = remaining[1:]
				continue
			}

			cluster, _ := ansi.FirstGraphemeCluster(remaining, ansi.GraphemeWidth)
			remaining = remaining[len(cluster):]
			display := text.VisualizeTerminalControls(cluster, false)
			glyph := draftGlyph{text: display, width: ansi.StringWidth(display)}
			// A wide glyph that does not fit belongs wholly to the next
			// visual row; its source cursor point must move with it.
			if col > 0 && col+glyph.width > contentWidth {
				newContinuation()
			}
			// Editing offsets remain runes; offsets inside a grapheme share
			// its display cell so cursor motion cannot split its rendering.
			for range utf8.RuneCountInString(cluster) {
				addPoint(offset)
				offset++
			}
			appendGlyph(glyph)
		}

		if col >= contentWidth {
			newContinuation()
		}
		addPoint(lineEnd)

		if lineEnd == len(content) || (rowLimit > 0 && len(layout.rows) >= rowLimit) {
			break
		}
		line++
		lineStart = lineEnd + 1
	}

	return layout
}

// Source offsets are ordered within each row. Tab expansion can leave rows
// without insertion points; skip those when locating the cursor.
func (l draftLayout) withCursor(cursor int) draftLayout {
	for rowIndex, row := range l.rows {
		if len(row.points) == 0 || row.points[len(row.points)-1].offset < cursor {
			continue
		}
		n := sort.Search(len(row.points), func(n int) bool { return row.points[n].offset >= cursor })
		if n < len(row.points) && row.points[n].offset == cursor {
			l.cursorRow, l.cursorCol = rowIndex, row.points[n].col
			return l
		}
	}
	l.cursorRow, l.cursorCol = len(l.rows)-1, 0
	return l
}

func draftGutterSize(lineCount, width int) int {
	digits := lenInt(lineCount)
	size := digits + 3 // number + space + marker + space
	if width-size < 1 {
		return 0
	}
	return size
}

func lenInt(n int) int {
	if n < 10 {
		return 1
	}
	digits := 0
	for n > 0 {
		n /= 10
		digits++
	}
	return digits
}

func (i *Input) draftTopRow(layout draftLayout, bodyHeight int) int {
	maxTop := max(0, len(layout.rows)-bodyHeight)
	top := clampInt(i.draftEditor.topRow, 0, maxTop)
	if layout.cursorRow < top {
		top = layout.cursorRow
	} else if layout.cursorRow >= top+bodyHeight {
		top = layout.cursorRow - bodyHeight + 1
	}
	return clampInt(top, 0, maxTop)
}

func (i *Input) draftRows(bodyHeight int) []string {
	layout := i.draftEditor.layout(i.width)
	top := i.draftTopRow(layout, bodyHeight)

	rows := make([]string, 0, bodyHeight)

	for n := 0; n < bodyHeight; n++ {
		rowIndex := top + n
		if rowIndex >= len(layout.rows) {
			rows = append(rows, strings.Repeat(" ", max(0, i.width)))
			continue
		}
		rows = append(rows, i.renderDraftRow(layout, rowIndex))
	}

	return rows
}

// draftLabels fits complete labels in the available cells. Mode switching
// takes precedence over line count; submit and newline precede secondary actions.
func (i *Input) draftLabels(lines, width int) (header, toggle, footer string) {
	mode, destination, submit := "COMMAND", "verbatim", i.actionHint("submit", "run")
	if i.SubmissionMode() == input.ModeVerbatim {
		mode, destination, submit = "VERBATIM", "command", i.actionHint("submit", "send")
	}
	word := "lines"
	if lines == 1 {
		word = "line"
	}
	title := fmt.Sprintf("%s · %d %s", mode, lines, word)
	toggle = i.actionHint("toggle_mode", destination)
	header = title
	if ansi.StringWidth(header)+3+ansi.StringWidth(toggle) > width {
		header = mode
	}
	if ansi.StringWidth(header)+3+ansi.StringWidth(toggle) > width {
		toggle = ""
		header = fitDraftHints(width, title)
		if header == "" {
			header = fitDraftHints(width, mode)
		}
	}
	hints := []string{submit, i.actionHint("newline", "newline")}
	cancel := i.keys.Hint("cancel")
	if cancel != "" {
		hints = append(hints, cancel+"×2 discard")
	}
	hints = append(hints, i.actionHint("open_editor", "editor"))
	footer = fitDraftHints(width, hints...)
	if i.discardPending && cancel != "" {
		footer = fitDraftHints(width, cancel+" again to discard")
		if footer == "" {
			footer = fitDraftHints(width, cancel+" to discard")
		}
	}

	return header, toggle, footer
}

// fitDraftHints keeps hints in priority order without cutting a key or label.
func fitDraftHints(width int, hints ...string) string {
	var fitted string
	for _, hint := range hints {
		if hint == "" {
			continue
		}
		candidate := hint
		if fitted != "" {
			candidate = fitted + " · " + hint
		}
		if ansi.StringWidth(candidate) > width {
			break
		}
		fitted = candidate
	}
	return fitted
}

func (i *Input) renderDraftRow(layout draftLayout, rowIndex int) string {
	row := layout.rows[rowIndex]
	var b strings.Builder

	if layout.gutterSize > 0 {
		digits := layout.gutterSize - 3
		if row.continuation {
			b.WriteString(i.styles.Muted.Render(strings.Repeat(" ", digits) + " ↳ "))
		} else {
			b.WriteString(i.styles.Muted.Render(fmt.Sprintf("%*d │ ", digits, row.line+1)))
		}
	}

	col := 0
	cursorDrawn := false
	for _, glyph := range row.glyphs {
		if i.selected {
			b.WriteString(i.styles.InputSelected.Render(glyph.text))
		} else if rowIndex == layout.cursorRow && col == layout.cursorCol && !cursorDrawn {
			b.WriteString(i.styles.InputCursor.Render(glyph.text))
			cursorDrawn = true
		} else {
			b.WriteString(i.styles.InputText.Render(glyph.text))
		}
		col += glyph.width
	}
	if rowIndex == layout.cursorRow && !cursorDrawn && !i.selected {
		b.WriteString(i.styles.InputCursor.Render(" "))
	}

	view := b.String()
	if padding := i.width - ansi.StringWidth(view); padding > 0 {
		view += strings.Repeat(" ", padding)
	}
	return util.ClipRow(view, i.width)
}

func (i *Input) actionHint(action, label string) string {
	key := i.keys.Hint(action)
	if key == "" {
		return ""
	}
	return key + " " + label
}
