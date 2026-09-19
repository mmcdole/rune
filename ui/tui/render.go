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

	if m.renderInterval <= 0 {
		m.render()
	}
	view.Content = m.screen
	return view
}

// newCanvas returns a blank cell grid. A ScreenBuffer renders every cell, so
// the rendered block is exactly its size with no trimming to undo.
func newCanvas(width, height int) uv.ScreenBuffer {
	canvas := uv.NewScreenBuffer(width, height)
	canvas.Method = ansi.GraphemeWidth
	return canvas
}

// render draws leaf content, resolved borders, and their labels.
// renderThrottled decides when.
func (m *Model) render() {
	m.dirty = false
	m.renders++
	if m.width <= 0 || m.height <= 0 {
		m.screen = ""
		return
	}
	if m.canvas.RenderBuffer == nil || m.canvas.Width() != m.width || m.canvas.Height() != m.height {
		m.canvas = newCanvas(m.width, m.height)
	} else if m.layoutPlan.borders.cells == nil {
		// A new layout may leave gaps where widgets used to be. Clear once
		// when rendering it, even if several layout updates preceded this render.
		m.canvas.Clear()
	}
	if m.layoutPlan.borders.cells == nil {
		m.resolveBorderCells(&m.layoutPlan.borders)
	}
	plan := m.layoutPlan
	canvas := m.canvas
	for _, leaf := range plan.leaves {
		if !leaf.content.Empty() {
			drawStyled(canvas, leaf.widget.View(), leaf.content)
		}
	}

	for _, border := range plan.borders.cells {
		canvas.SetCell(border.x, border.y, border.cell)
	}
	m.drawLabels(canvas, plan)
	m.screen = canvas.Render()
}

// drawLabels renders pane titles and rule labels over the borders.
func (m *Model) drawLabels(canvas uv.ScreenBuffer, plan layoutPlan) {
	for _, leaf := range plan.leaves {
		if leaf.node.Type != ui.LayoutTypePane || leaf.edges&borderTop == 0 {
			continue
		}
		title := leaf.widget.(pane).Title()
		if leaf.node.Title != nil {
			title = *leaf.node.Title
		}
		if title == "" {
			continue
		}
		left, right := leaf.outer.Min.X, leaf.outer.Max.X
		if leaf.edges&borderLeft != 0 {
			left++
		}
		if leaf.edges&borderRight != 0 {
			right--
		}
		title = " " + runetext.VisualizeTerminalControls(title, false) + " "
		drawLabel(canvas, plan.borders, m.styles.PaneHeader.Render(title), left, right, leaf.outer.Min.Y)
	}
	for _, rule := range plan.rules {
		for _, label := range rule.Labels {
			drawLabel(canvas, plan.borders, label.Style.Render(label.Text), label.At, rule.To, rule.At)
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
func drawLabel(canvas uv.ScreenBuffer, borders borderGrid, label string, left, right, y int) {
	for x := left; x < right; x++ {
		if borders.at(borders.vertical, x, y) || borders.at(borders.vertical, x, y-1) || borders.at(borders.vertical, x, y+1) {
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
