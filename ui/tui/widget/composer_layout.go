package widget

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui/tui/util"
)

type composerGlyph struct {
	text  string
	width int
}

type composerPoint struct {
	offset int
	col    int
}

type composerRow struct {
	line         int
	continuation bool
	glyphs       []composerGlyph
	points       []composerPoint
}

type composerLayout struct {
	rows       []composerRow
	cursorRow  int
	cursorCol  int
	lineCount  int
	gutterSize int
}

// buildComposerLayout derives safe terminal rows from the canonical buffer.
// Source tabs remain one rune but expand to cells at classic 8-column stops.
// Every source insertion offset is retained on exactly one visual row so
// vertical movement and cursor rendering never need to reverse-map strings.
func buildComposerLayout(content []rune, cursor, width int) composerLayout {
	lineCount := 1
	for _, r := range content {
		if r == '\n' {
			lineCount++
		}
	}

	gutter := composerGutterSize(lineCount, width)
	contentWidth := width - gutter
	if contentWidth < 1 {
		contentWidth = 1
	}

	layout := composerLayout{
		lineCount:  lineCount,
		gutterSize: gutter,
		cursorRow:  -1,
	}

	line := 0
	lineStart := 0
	for {
		lineEnd := lineStart
		for lineEnd < len(content) && content[lineEnd] != '\n' {
			lineEnd++
		}

		layout.rows = append(layout.rows, composerRow{line: line})
		rowIndex := len(layout.rows) - 1
		col := 0
		logicalCol := 0

		newContinuation := func() {
			layout.rows = append(layout.rows, composerRow{line: line, continuation: true})
			rowIndex = len(layout.rows) - 1
			col = 0
		}
		addPoint := func(offset int) {
			layout.rows[rowIndex].points = append(layout.rows[rowIndex].points, composerPoint{offset: offset, col: col})
			if offset == cursor {
				layout.cursorRow = rowIndex
				layout.cursorCol = col
			}
		}
		appendGlyph := func(g composerGlyph) {
			if g.width > contentWidth {
				g = composerGlyph{text: "�", width: 1}
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
					appendGlyph(composerGlyph{text: " ", width: 1})
				}
				offset++
				remaining = remaining[1:]
				continue
			}

			cluster, _ := ansi.FirstGraphemeCluster(remaining, ansi.GraphemeWidth)
			remaining = remaining[len(cluster):]
			display := text.VisualizeTerminalControls(cluster, false)
			glyph := composerGlyph{text: display, width: util.VisibleLen(display)}
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

		if lineEnd == len(content) {
			break
		}
		line++
		lineStart = lineEnd + 1
	}

	if layout.cursorRow < 0 {
		layout.cursorRow = len(layout.rows) - 1
		layout.cursorCol = 0
	}
	return layout
}

func composerGutterSize(lineCount, width int) int {
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

func (i *Input) composerTopRow(layout composerLayout, bodyHeight int) int {
	maxTop := max(0, len(layout.rows)-bodyHeight)
	top := clampInt(i.composer.topRow, 0, maxTop)
	if layout.cursorRow < top {
		top = layout.cursorRow
	} else if layout.cursorRow >= top+bodyHeight {
		top = layout.cursorRow - bodyHeight + 1
	}
	return clampInt(top, 0, maxTop)
}

func (i *Input) composerRows(bodyHeight int) []string {
	layout := buildComposerLayout(i.composer.text, i.composer.cursor, i.width)
	top := i.composerTopRow(layout, bodyHeight)

	rows := make([]string, 0, bodyHeight)

	for n := 0; n < bodyHeight; n++ {
		rowIndex := top + n
		if rowIndex >= len(layout.rows) {
			rows = append(rows, strings.Repeat(" ", max(0, i.width)))
			continue
		}
		rows = append(rows, i.renderComposerRow(layout, rowIndex))
	}

	return rows
}

// composeLabels fits complete labels in the available cells. Mode switching
// takes precedence over line count; submit and newline precede secondary actions.
func (i *Input) composeLabels(lines, width int) (header, toggle, footer string) {
	mode, destination, submit := "COMMAND", "verbatim", "Enter run"
	if i.SubmissionMode() == input.ModeVerbatim {
		mode, destination, submit = "VERBATIM", "command", "Enter send"
	}
	word := "lines"
	if lines == 1 {
		word = "line"
	}
	title := fmt.Sprintf("%s · %d %s", mode, lines, word)
	toggle = "Alt+V " + destination
	header = title
	if ansi.StringWidth(header)+3+ansi.StringWidth(toggle) > width {
		header = mode
	}
	if ansi.StringWidth(header)+3+ansi.StringWidth(toggle) > width {
		toggle = ""
		header = fitComposerHints(width, title)
		if header == "" {
			header = fitComposerHints(width, mode)
		}
	}
	hints := []string{submit, "Ctrl+J newline"}
	if i.SubmissionMode() == input.ModeVerbatim {
		hints = append(hints, "Alt+Enter run")
	}
	hints = append(hints, "Esc×2 discard")
	if i.editorAvailable {
		hints = append(hints, "Ctrl+E editor")
	}
	footer = fitComposerHints(width, hints...)
	if i.discardPending {
		footer = fitComposerHints(width, "Esc again to discard")
		if footer == "" {
			footer = fitComposerHints(width, "Esc to discard")
		}
	}
	return header, toggle, footer
}

// fitComposerHints keeps hints in priority order without cutting a key or label.
func fitComposerHints(width int, hints ...string) string {
	var fitted string
	for _, hint := range hints {
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

func (i *Input) renderComposerRow(layout composerLayout, rowIndex int) string {
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
	if padding := i.width - util.VisibleLen(view); padding > 0 {
		view += strings.Repeat(" ", padding)
	}
	return clipRow(view, i.width)
}
