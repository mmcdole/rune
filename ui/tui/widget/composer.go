package widget

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/mmcdole/rune/input"
)

// maxComposerBodyRows keeps a pasted document useful without allowing the
// input area to take over the terminal. The surrounding Input adds a header
// and footer to these content rows.
const maxComposerBodyRows = 8

// composer is the lossless editing model used for verbatim drafts, including
// physical structure that bubbles/textinput cannot represent (LF or TAB).
// Cursor positions are rune offsets, matching the existing Rune input API.
type composer struct {
	text    []rune
	cursor  int
	goalCol int // retained display column during vertical movement; -1 = unset
	topRow  int // first visual row shown by the input viewport
}

func newComposer(text string, cursor int) *composer {
	c := &composer{goalCol: -1}
	c.Set(text, cursor)
	return c
}

// normalizeComposerText gives the draft one internal newline convention.
// Ordering matters: replacing lone CR first would turn CRLF into two lines.
func normalizeComposerText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

// RequiresComposer reports whether the neutral input policy requires the
// lossless editor. Kept as a widget-level name for the local call sites; the
// admission rule itself belongs to the input package.
func RequiresComposer(value string) bool {
	return input.RequiresVerbatim(value)
}

func (c *composer) Value() string {
	return string(c.text)
}

func (c *composer) Position() int {
	return c.cursor
}

func (c *composer) Set(text string, cursor int) {
	c.text = []rune(normalizeComposerText(text))
	c.SetCursor(cursor)
	c.goalCol = -1
	c.topRow = 0
}

func (c *composer) SetCursor(cursor int) {
	c.cursor = clampInt(cursor, 0, len(c.text))
	c.goalCol = -1
}

func (c *composer) CursorEnd() {
	c.SetCursor(len(c.text))
}

func (c *composer) Insert(text string) {
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

func (c *composer) Backspace() {
	if c.cursor == 0 {
		return
	}
	c.text = append(c.text[:c.cursor-1], c.text[c.cursor:]...)
	c.cursor--
	c.goalCol = -1
}

func (c *composer) Delete() {
	if c.cursor >= len(c.text) {
		return
	}
	c.text = append(c.text[:c.cursor], c.text[c.cursor+1:]...)
	c.goalCol = -1
}

func (c *composer) Left() {
	if c.cursor > 0 {
		c.cursor--
	}
	c.goalCol = -1
}

func (c *composer) Right() {
	if c.cursor < len(c.text) {
		c.cursor++
	}
	c.goalCol = -1
}

func (c *composer) LineStart() {
	for c.cursor > 0 && c.text[c.cursor-1] != '\n' {
		c.cursor--
	}
	c.goalCol = -1
}

func (c *composer) LineEnd() {
	for c.cursor < len(c.text) && c.text[c.cursor] != '\n' {
		c.cursor++
	}
	c.goalCol = -1
}

func (c *composer) DocStart() {
	c.cursor = 0
	c.goalCol = -1
}

func (c *composer) DocEnd() {
	c.cursor = len(c.text)
	c.goalCol = -1
}

func (c *composer) WordLeft() {
	for c.cursor > 0 && unicode.IsSpace(c.text[c.cursor-1]) {
		c.cursor--
	}
	for c.cursor > 0 && !unicode.IsSpace(c.text[c.cursor-1]) {
		c.cursor--
	}
	c.goalCol = -1
}

func (c *composer) WordRight() {
	for c.cursor < len(c.text) && !unicode.IsSpace(c.text[c.cursor]) {
		c.cursor++
	}
	for c.cursor < len(c.text) && unicode.IsSpace(c.text[c.cursor]) {
		c.cursor++
	}
	c.goalCol = -1
}

func (c *composer) DeleteWordBack() {
	end := c.cursor
	c.WordLeft()
	if c.cursor == end {
		return
	}
	c.text = append(c.text[:c.cursor], c.text[end:]...)
	c.goalCol = -1
}

func (c *composer) DeleteToLineStart() {
	end := c.cursor
	c.LineStart()
	if c.cursor == end {
		return
	}
	c.text = append(c.text[:c.cursor], c.text[end:]...)
	c.goalCol = -1
}

func (c *composer) DeleteToLineEnd() {
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
func (c *composer) Update(msg tea.KeyPressMsg, widgetWidth int) bool {
	if msg.Text != "" {
		c.Insert(msg.Text)
		return true
	}

	switch {
	case matchesKey(msg, 'j', tea.ModCtrl), matchesEnterKey(msg, tea.ModCtrl):
		c.Insert("\n")
		return true
	case matchesEnterKey(msg, 0):
		// Plain Enter submits; modified Enter chords remain available to the
		// controller for bind dispatch.
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
	case matchesKey(msg, tea.KeyBackspace, 0), matchesKey(msg, 'h', tea.ModCtrl):
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
		c.moveVertical(-maxComposerBodyRows, widgetWidth)
		return true
	case matchesKey(msg, tea.KeyPgDown, 0):
		c.moveVertical(maxComposerBodyRows, widgetWidth)
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

func (c *composer) moveVertical(delta, widgetWidth int) {
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
