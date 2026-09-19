package tui

import (
	"image"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	runetext "github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui"
)

// View implements tea.Model.
func (m *Model) View() tea.View {
	view := tea.View{AltScreen: true}
	if m.mouseEnabled {
		view.MouseMode = tea.MouseModeCellMotion
	}
	if m.numpadMode {
		// The default kitty disambiguation flag reports NumLock-on keypad
		// digits as plain text, indistinguishable from the number row.
		view.KeyboardEnhancements.ReportAllKeysAsEscapeCodes = true
		view.KeyboardEnhancements.ReportAssociatedText = true
	}
	if !m.initialized {
		view.Content = "Loading..."
		return view
	}
	if m.width <= 0 || m.height <= 0 {
		return view
	}

	if m.frameInterval <= 0 {
		m.compose()
	}
	view.Content = m.renderedContent
	return view
}

// newCanvas returns a blank cell grid. A ScreenBuffer renders every cell, so
// the composed block is exactly its size with no trimming to undo.
func newCanvas(width, height int) uv.ScreenBuffer {
	canvas := uv.NewScreenBuffer(width, height)
	canvas.Method = ansi.GraphemeWidth
	return canvas
}

// frameCanvas returns a blank canvas of the terminal size, reusing the last
// frame's cells: allocating them anew dominated composition.
func (m *Model) frameCanvas() uv.ScreenBuffer {
	if m.canvas.RenderBuffer == nil || m.canvas.Width() != m.width || m.canvas.Height() != m.height {
		m.canvas = newCanvas(m.width, m.height)
	} else {
		m.canvas.Clear()
	}
	return m.canvas
}

// compose paints the current layout plan into renderedContent: leaf content,
// then the frame grid's junction glyphs, then the labels that sit on it. The
// frame clock decides when.
func (m *Model) compose() {
	m.stale = false
	m.compositions++
	if m.width <= 0 || m.height <= 0 {
		m.renderedContent = ""
		return
	}
	plan := m.layoutPlan
	canvas := m.frameCanvas()
	for _, leaf := range plan.leaves {
		if !leaf.content.Empty() {
			drawStyled(canvas, leaf.surface.View(), leaf.content)
		}
	}

	// Only marked cells can hold a glyph; most of the grid is content.
	frame := plan.frame
	for i := range frame.horizontal {
		if !frame.horizontal[i] && !frame.vertical[i] {
			continue
		}
		x, y := i%frame.width, i/frame.width
		glyph := frame.glyph(x, y)
		cell := m.frameCells[glyph]
		if cell == nil {
			cell = styledCell(m.styles.PaneBorder.Render(glyph))
			m.frameCells[glyph] = cell
		}
		canvas.SetCell(x, y, cell)
	}
	m.drawFrameLabels(canvas, plan)
	m.renderedContent = canvas.Render()
}

// drawFrameLabels paints pane titles and rule labels over the frame grid.
func (m *Model) drawFrameLabels(canvas uv.ScreenBuffer, plan layoutPlan) {
	for _, leaf := range plan.leaves {
		if leaf.node.Type != ui.LayoutTypePane || leaf.frames&frameTop == 0 {
			continue
		}
		title := leaf.surface.(paneResource).Title()
		if leaf.node.Title != nil {
			title = *leaf.node.Title
		}
		if title == "" {
			continue
		}
		left, right := leaf.outer.Min.X, leaf.outer.Max.X
		if leaf.frames&frameLeft != 0 {
			left++
		}
		if leaf.frames&frameRight != 0 {
			right--
		}
		title = " " + runetext.VisualizeTerminalControls(title, false) + " "
		drawFrameLabel(canvas, plan.frame, m.styles.PaneHeader.Render(title), left, right, leaf.outer.Min.Y)
	}
	for _, rule := range plan.rules {
		for _, label := range rule.Labels {
			drawFrameLabel(canvas, plan.frame, label.Style.Render(label.Text), label.At, rule.To, rule.At)
		}
	}
}

func drawStyled(canvas uv.ScreenBuffer, content string, rect image.Rectangle) {
	rect = rect.Intersect(canvas.Bounds())
	if rect.Empty() {
		return
	}
	uv.NewStyledString(content).Draw(canvas, rect)
}

func styledCell(rendered string) *uv.Cell {
	canvas := newCanvas(1, 1)
	drawStyled(canvas, rendered, canvas.Bounds())
	return canvas.CellAt(0, 0)
}

// Labels cover only their text cells, never the remaining rule or a junction.
func drawFrameLabel(canvas uv.ScreenBuffer, frame frameGrid, label string, left, right, y int) {
	for x := left; x < right; x++ {
		if frame.at(frame.vertical, x, y) || frame.at(frame.vertical, x, y-1) || frame.at(frame.vertical, x, y+1) {
			right = x
			break
		}
	}
	if right <= left {
		return
	}
	label = ansi.Truncate(label, right-left, "")
	drawStyled(canvas, label, image.Rect(left, y, left+ansi.StringWidth(label), y+1))
}
