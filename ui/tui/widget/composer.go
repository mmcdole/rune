package widget

import (
	"strings"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"

	"github.com/mmcdole/rune/text"
)

// maxComposerBodyRows keeps a pasted document useful without allowing the
// input area to take over the terminal. The surrounding Input adds a header
// and footer to these content rows.
const maxComposerBodyRows = 8

// Composer is the lossless editing model used only while the input contains
// physical structure that bubbles/textinput cannot represent (LF or TAB).
// Cursor positions are rune offsets, matching the existing Rune input API.
type Composer struct {
	text    []rune
	cursor  int
	goalCol int // retained display column during vertical movement; -1 = unset
	topRow  int // first visual row shown by the input viewport
}

func newComposer(text string, cursor int) *Composer {
	c := &Composer{goalCol: -1}
	c.Set(text, cursor)
	return c
}

// normalizeComposerText gives the draft one internal newline convention.
// Ordering matters: replacing lone CR first would turn CRLF into two lines.
func normalizeComposerText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

// RequiresComposer reports whether text contains structure or controls the
// normal single-line textinput would destroy. The composer preserves the
// canonical rune while projecting unsafe controls visibly at render time.
func RequiresComposer(value string) bool {
	for _, r := range value {
		if text.VisualizeTerminalRune(r, false) != r {
			return true
		}
	}
	return false
}

func (c *Composer) Value() string {
	return string(c.text)
}

func (c *Composer) Position() int {
	return c.cursor
}

func (c *Composer) Set(text string, cursor int) {
	c.text = []rune(normalizeComposerText(text))
	c.SetCursor(cursor)
	c.goalCol = -1
	c.topRow = 0
}

func (c *Composer) SetCursor(cursor int) {
	c.cursor = clampInt(cursor, 0, len(c.text))
	c.goalCol = -1
}

func (c *Composer) CursorEnd() {
	c.SetCursor(len(c.text))
}

func (c *Composer) Reset() {
	c.text = nil
	c.cursor = 0
	c.goalCol = -1
	c.topRow = 0
}

func (c *Composer) Insert(text string) {
	runes := []rune(normalizeComposerText(text))
	if len(runes) == 0 {
		return
	}

	tail := append([]rune(nil), c.text[c.cursor:]...)
	c.text = append(c.text[:c.cursor], runes...)
	c.cursor += len(runes)
	c.text = append(c.text, tail...)
	c.goalCol = -1
}

func (c *Composer) Backspace() {
	if c.cursor == 0 {
		return
	}
	c.text = append(c.text[:c.cursor-1], c.text[c.cursor:]...)
	c.cursor--
	c.goalCol = -1
}

func (c *Composer) Delete() {
	if c.cursor >= len(c.text) {
		return
	}
	c.text = append(c.text[:c.cursor], c.text[c.cursor+1:]...)
	c.goalCol = -1
}

func (c *Composer) Left() {
	if c.cursor > 0 {
		c.cursor--
	}
	c.goalCol = -1
}

func (c *Composer) Right() {
	if c.cursor < len(c.text) {
		c.cursor++
	}
	c.goalCol = -1
}

func (c *Composer) LineStart() {
	for c.cursor > 0 && c.text[c.cursor-1] != '\n' {
		c.cursor--
	}
	c.goalCol = -1
}

func (c *Composer) LineEnd() {
	for c.cursor < len(c.text) && c.text[c.cursor] != '\n' {
		c.cursor++
	}
	c.goalCol = -1
}

func (c *Composer) DocStart() {
	c.cursor = 0
	c.goalCol = -1
}

func (c *Composer) DocEnd() {
	c.cursor = len(c.text)
	c.goalCol = -1
}

func (c *Composer) WordLeft() {
	for c.cursor > 0 && unicode.IsSpace(c.text[c.cursor-1]) {
		c.cursor--
	}
	for c.cursor > 0 && !unicode.IsSpace(c.text[c.cursor-1]) {
		c.cursor--
	}
	c.goalCol = -1
}

func (c *Composer) WordRight() {
	for c.cursor < len(c.text) && !unicode.IsSpace(c.text[c.cursor]) {
		c.cursor++
	}
	for c.cursor < len(c.text) && unicode.IsSpace(c.text[c.cursor]) {
		c.cursor++
	}
	c.goalCol = -1
}

func (c *Composer) DeleteWordBack() {
	end := c.cursor
	c.WordLeft()
	if c.cursor == end {
		return
	}
	c.text = append(c.text[:c.cursor], c.text[end:]...)
	c.goalCol = -1
}

func (c *Composer) DeleteToLineStart() {
	end := c.cursor
	c.LineStart()
	if c.cursor == end {
		return
	}
	c.text = append(c.text[:c.cursor], c.text[end:]...)
	c.goalCol = -1
}

func (c *Composer) DeleteToLineEnd() {
	start := c.cursor
	c.LineEnd()
	end := c.cursor
	c.cursor = start
	if start == end {
		// Match terminal editor behavior: at EOL, Ctrl+K joins the next
		// physical line instead of becoming a no-op.
		if end < len(c.text) && c.text[end] == '\n' {
			end++
		} else {
			return
		}
	}
	c.text = append(c.text[:start], c.text[end:]...)
	c.goalCol = -1
}

// Update applies keys that have local editing meaning in compose mode. It
// deliberately leaves plain Enter, Escape, Ctrl+C, and Ctrl+E unhandled so
// the controller can submit/cancel/delegate to the external-editor binding.
func (c *Composer) Update(msg tea.KeyMsg, widgetWidth int) bool {
	switch msg.Type {
	case tea.KeyCtrlJ:
		c.Insert("\n")
		return true
	case tea.KeyEnter:
		// Plain Enter submits; Alt+Enter is intentionally left to the
		// controller as the reserved alternate-submit chord.
		return false
	case tea.KeyTab:
		c.Insert("\t")
		return true
	case tea.KeyLeft:
		if msg.Alt {
			c.WordLeft()
		} else {
			c.Left()
		}
		return true
	case tea.KeyRight:
		if msg.Alt {
			c.WordRight()
		} else {
			c.Right()
		}
		return true
	case tea.KeyCtrlB:
		c.Left()
		return true
	case tea.KeyCtrlF:
		c.Right()
		return true
	case tea.KeyUp, tea.KeyCtrlP:
		c.moveVertical(-1, widgetWidth)
		return true
	case tea.KeyDown, tea.KeyCtrlN:
		c.moveVertical(1, widgetWidth)
		return true
	case tea.KeyHome, tea.KeyCtrlA:
		c.LineStart()
		return true
	case tea.KeyEnd:
		c.LineEnd()
		return true
	case tea.KeyCtrlHome:
		c.DocStart()
		return true
	case tea.KeyCtrlEnd:
		c.DocEnd()
		return true
	case tea.KeyBackspace, tea.KeyCtrlH:
		if msg.Alt {
			c.DeleteWordBack()
		} else {
			c.Backspace()
		}
		return true
	case tea.KeyDelete, tea.KeyCtrlD:
		c.Delete()
		return true
	case tea.KeyCtrlW:
		c.DeleteWordBack()
		return true
	case tea.KeyCtrlU:
		c.DeleteToLineStart()
		return true
	case tea.KeyCtrlK:
		c.DeleteToLineEnd()
		return true
	case tea.KeyCtrlLeft:
		c.WordLeft()
		return true
	case tea.KeyCtrlRight:
		c.WordRight()
		return true
	case tea.KeyPgUp:
		c.moveVertical(-maxComposerBodyRows, widgetWidth)
		return true
	case tea.KeyPgDown:
		c.moveVertical(maxComposerBodyRows, widgetWidth)
		return true
	case tea.KeySpace:
		c.Insert(" ")
		return true
	case tea.KeyRunes:
		if msg.Alt {
			switch string(msg.Runes) {
			case "b":
				c.WordLeft()
				return true
			case "f":
				c.WordRight()
				return true
			}
			return false
		}
		c.Insert(string(msg.Runes))
		return true
	}

	return false
}

func (c *Composer) moveVertical(delta, widgetWidth int) {
	layout := buildComposerLayout(c.text, c.cursor, widgetWidth)
	if len(layout.rows) == 0 {
		return
	}
	if c.goalCol < 0 {
		c.goalCol = layout.cursorCol
	}
	target := clampInt(layout.cursorRow+delta, 0, len(layout.rows)-1)
	points := layout.rows[target].points
	if len(points) == 0 {
		return
	}

	best := points[0]
	bestDistance := absInt(best.col - c.goalCol)
	for _, point := range points[1:] {
		distance := absInt(point.col - c.goalCol)
		if distance < bestDistance || (distance == bestDistance && point.col > best.col) {
			best = point
			bestDistance = distance
		}
	}
	c.cursor = best.offset
}

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
func buildComposerLayout(text []rune, cursor, width int) composerLayout {
	lineCount := 1
	for _, r := range text {
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
		for lineEnd < len(text) && text[lineEnd] != '\n' {
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

		for offset := lineStart; offset < lineEnd; offset++ {
			if col >= contentWidth {
				newContinuation()
			}
			r := text[offset]
			if r == '\t' {
				addPoint(offset)
				padding := 8 - logicalCol%8
				for n := 0; n < padding; n++ {
					if col >= contentWidth {
						newContinuation()
					}
					appendGlyph(composerGlyph{text: " ", width: 1})
				}
				continue
			}

			glyph := safeComposerGlyph(r)
			// A wide glyph that does not fit belongs wholly to the next
			// visual row; its source cursor point must move with it.
			if col > 0 && col+glyph.width > contentWidth {
				newContinuation()
			}
			addPoint(offset)
			appendGlyph(glyph)
		}

		if col >= contentWidth {
			newContinuation()
		}
		addPoint(lineEnd)

		if lineEnd == len(text) {
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

func safeComposerGlyph(r rune) composerGlyph {
	if r == utf8.RuneError {
		return composerGlyph{text: "�", width: 1}
	}
	displayRune := text.VisualizeTerminalRune(r, false)
	if displayRune != r {
		display := string(displayRune)
		return composerGlyph{text: display, width: runewidth.RuneWidth(displayRune)}
	}

	width := runewidth.RuneWidth(r)
	if width < 0 {
		width = 1
	}
	return composerGlyph{text: string(r), width: width}
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

func clampInt(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
