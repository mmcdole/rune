package widget

import (
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/mmcdole/rune/input"
)

// maxEditorBodyRows keeps a pasted document useful without allowing the
// input area to take over the terminal. The surrounding Input adds a header
// and footer to these content rows.
const maxEditorBodyRows = 8

// editor is the lossless editing model used for verbatim drafts, including
// physical structure that bubbles/textinput cannot represent (LF or TAB).
// Cursor positions are rune offsets, matching the existing Rune input API.
type editor struct {
	text    []rune
	cursor  int
	goalCol int // retained display column during vertical movement; -1 = unset
	topRow  int // first visual row shown by the input window

	cached      editorLayout
	layoutWidth int
	lineCount   int
}

func newEditor(text string, cursor int) *editor {
	c := &editor{goalCol: -1}
	c.Set(text, cursor)
	return c
}

func (c *editor) Value() string {
	return string(c.text)
}

func (c *editor) Position() int {
	return c.cursor
}

func (c *editor) Set(text string, cursor int) {
	c.text = []rune(input.NormalizeDraftText(text))
	c.invalidate()
	c.SetCursor(cursor)
	c.goalCol = -1
	c.topRow = 0
}

func (c *editor) SetCursor(cursor int) {
	c.cursor = clampInt(cursor, 0, len(c.text))
	c.goalCol = -1
}

func (c *editor) CursorEnd() {
	c.SetCursor(len(c.text))
}

func (c *editor) Insert(text string) {
	runes := []rune(input.NormalizeDraftText(text))
	if len(runes) == 0 {
		return
	}

	tail := append([]rune(nil), c.text[c.cursor:]...)
	c.invalidate()
	c.text = append(c.text[:c.cursor], runes...)
	c.cursor += len(runes)
	c.text = append(c.text, tail...)
	c.goalCol = -1
}

func (c *editor) Backspace() {
	if c.cursor == 0 {
		return
	}
	c.invalidate()
	c.text = append(c.text[:c.cursor-1], c.text[c.cursor:]...)
	c.cursor--
	c.goalCol = -1
}

func (c *editor) Delete() {
	if c.cursor >= len(c.text) {
		return
	}
	c.invalidate()
	c.text = append(c.text[:c.cursor], c.text[c.cursor+1:]...)
	c.goalCol = -1
}

func (c *editor) Left() {
	if c.cursor > 0 {
		c.cursor--
	}
	c.goalCol = -1
}

func (c *editor) Right() {
	if c.cursor < len(c.text) {
		c.cursor++
	}
	c.goalCol = -1
}

func (c *editor) LineStart() {
	for c.cursor > 0 && c.text[c.cursor-1] != '\n' {
		c.cursor--
	}
	c.goalCol = -1
}

func (c *editor) LineEnd() {
	for c.cursor < len(c.text) && c.text[c.cursor] != '\n' {
		c.cursor++
	}
	c.goalCol = -1
}

func (c *editor) DocStart() {
	c.cursor = 0
	c.goalCol = -1
}

func (c *editor) DocEnd() {
	c.cursor = len(c.text)
	c.goalCol = -1
}

func (c *editor) WordLeft() {
	for c.cursor > 0 && unicode.IsSpace(c.text[c.cursor-1]) {
		c.cursor--
	}
	for c.cursor > 0 && !unicode.IsSpace(c.text[c.cursor-1]) {
		c.cursor--
	}
	c.goalCol = -1
}

func (c *editor) WordRight() {
	for c.cursor < len(c.text) && !unicode.IsSpace(c.text[c.cursor]) {
		c.cursor++
	}
	for c.cursor < len(c.text) && unicode.IsSpace(c.text[c.cursor]) {
		c.cursor++
	}
	c.goalCol = -1
}

func (c *editor) DeleteWordBack() {
	end := c.cursor
	c.WordLeft()
	if c.cursor == end {
		return
	}
	c.invalidate()
	c.text = append(c.text[:c.cursor], c.text[end:]...)
	c.goalCol = -1
}

func (c *editor) DeleteToLineStart() {
	end := c.cursor
	c.LineStart()
	if c.cursor == end {
		return
	}
	c.invalidate()
	c.text = append(c.text[:c.cursor], c.text[end:]...)
	c.goalCol = -1
}

func (c *editor) DeleteToLineEnd() {
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
	c.invalidate()
	c.text = append(c.text[:start], c.text[end:]...)
	c.goalCol = -1
}

// Update applies keys that have local editing meaning in editor mode. The
// controller handles configured editor actions first. Escape, Ctrl+C, and
// Ctrl+E remain available for cancellation and Lua bindings.
func (c *editor) Update(msg tea.KeyPressMsg, widgetWidth int) bool {
	if msg.Text != "" {
		c.Insert(msg.Text)
		return true
	}

	switch {
	case matchesEnterKey(msg, 0):
		// Enter has no editing meaning unless handled by a configured action.
		return false
	case matchesKey(msg, tea.KeyTab, 0):
		c.Insert("\t")
		return true
	case matchesKey(msg, tea.KeyLeft, 0), matchesKey(msg, 'b', tea.ModCtrl):
		c.Left()
		return true
	case matchesKey(msg, tea.KeyLeft, tea.ModAlt),
		matchesKey(msg, tea.KeyLeft, tea.ModCtrl),
		matchesKey(msg, 'b', tea.ModAlt):
		c.WordLeft()
		return true
	case matchesKey(msg, tea.KeyRight, 0), matchesKey(msg, 'f', tea.ModCtrl):
		c.Right()
		return true
	case matchesKey(msg, tea.KeyRight, tea.ModAlt),
		matchesKey(msg, tea.KeyRight, tea.ModCtrl),
		matchesKey(msg, 'f', tea.ModAlt):
		c.WordRight()
		return true
	case matchesKey(msg, tea.KeyUp, 0), matchesKey(msg, 'p', tea.ModCtrl):
		c.moveVertical(-1, widgetWidth)
		return true
	case matchesKey(msg, tea.KeyDown, 0), matchesKey(msg, 'n', tea.ModCtrl):
		c.moveVertical(1, widgetWidth)
		return true
	case matchesKey(msg, tea.KeyHome, 0), matchesKey(msg, 'a', tea.ModCtrl):
		c.LineStart()
		return true
	case matchesKey(msg, tea.KeyEnd, 0):
		c.LineEnd()
		return true
	case matchesKey(msg, tea.KeyHome, tea.ModCtrl):
		c.DocStart()
		return true
	case matchesKey(msg, tea.KeyEnd, tea.ModCtrl):
		c.DocEnd()
		return true
	case matchesKey(msg, tea.KeyBackspace, 0), matchesKey(msg, tea.KeyBackspace, tea.ModShift),
		matchesKey(msg, 'h', tea.ModCtrl):
		c.Backspace()
		return true
	case matchesKey(msg, tea.KeyBackspace, tea.ModAlt),
		matchesKey(msg, 'h', tea.ModCtrl|tea.ModAlt),
		matchesKey(msg, 'w', tea.ModCtrl):
		c.DeleteWordBack()
		return true
	case matchesKey(msg, tea.KeyDelete, 0), matchesKey(msg, 'd', tea.ModCtrl):
		c.Delete()
		return true
	case matchesKey(msg, 'u', tea.ModCtrl):
		c.DeleteToLineStart()
		return true
	case matchesKey(msg, 'k', tea.ModCtrl):
		c.DeleteToLineEnd()
		return true
	case matchesKey(msg, tea.KeyPgUp, 0):
		c.moveVertical(-maxEditorBodyRows, widgetWidth)
		return true
	case matchesKey(msg, tea.KeyPgDown, 0):
		c.moveVertical(maxEditorBodyRows, widgetWidth)
		return true
	}

	return false
}

const keyModifiers = tea.ModShift | tea.ModAlt | tea.ModCtrl |
	tea.ModMeta | tea.ModHyper | tea.ModSuper

func matchesKey(msg tea.KeyPressMsg, code rune, modifiers tea.KeyMod) bool {
	return msg.Code == code && msg.Mod&keyModifiers == modifiers
}

func matchesEnterKey(msg tea.KeyPressMsg, modifiers tea.KeyMod) bool {
	return (msg.Code == tea.KeyEnter || msg.Code == tea.KeyKpEnter) &&
		msg.Mod&keyModifiers == modifiers
}

func (c *editor) moveVertical(delta, widgetWidth int) {
	layout := c.layout(widgetWidth)
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

// layout shares the same shaped draft across measurement, rendering, and
// navigation. Text edits invalidate it; an unchanged draft needs no reshaping.
func (c *editor) layout(width int) editorLayout {
	if c.cached.rows == nil || c.layoutWidth != width {
		c.cached = buildEditorLayout(c.text, width, c.lines(), 0)
		c.layoutWidth = width
	}
	return c.cached.withCursor(c.cursor)
}

func (c *editor) invalidate() {
	c.cached.rows = nil
	c.lineCount = 0
}

func (c *editor) lines() int {
	if c.lineCount == 0 {
		c.lineCount = 1
		for _, r := range c.text {
			if r == '\n' {
				c.lineCount++
			}
		}
	}
	return c.lineCount
}

// Measurement must not evict the layout at the actual editing width. Most
// large drafts already reach the height cap without examining their wrapping.
func (c *editor) measureRows(width int) int {
	if c.lines() >= maxEditorBodyRows {
		return maxEditorBodyRows
	}
	if c.cached.rows != nil && c.layoutWidth == width {
		return min(len(c.cached.rows), maxEditorBodyRows)
	}
	return min(len(buildEditorLayout(c.text, width, c.lines(), maxEditorBodyRows).rows), maxEditorBodyRows)
}
