package tui

import (
	"image"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	runetext "github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui"
)

// defaultRenderInterval bounds screen rendering to about 60 a second.
const defaultRenderInterval = 16 * time.Millisecond

// renderTick ends a throttle window. The first change after idle renders
// immediately; changes inside the window are drawn together at its end.
// At most one tick is outstanding, including retries of rejected scroll
// reports. Bubble Tea owns the separate terminal flush clock.
type renderTick struct{}

// renderIfDue draws pending changes and reports the resulting scroll state.
// Reporting stays behind the throttle so it cannot run ahead of the screen.
// A rejected report keeps the timer alive without requiring another redraw.
// With a zero interval, a failed report waits for the next Update instead.
func (m *Model) renderIfDue() tea.Cmd {
	if m.throttled {
		return nil
	}
	rendered := m.dirty
	if rendered {
		m.render()
		m.dirty = false
	}
	reported := m.reportScrollState()
	if m.renderInterval <= 0 || (!rendered && reported) {
		return nil
	}
	m.throttled = true
	return tea.Tick(m.renderInterval, func(time.Time) tea.Msg { return renderTick{} })
}

// View implements tea.Model, returning the prepared screen and terminal options.
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
// renderIfDue decides when and owns the dirty flag.
func (m *Model) render() {
	m.renders++
	if m.width <= 0 || m.height <= 0 {
		m.screen = ""
		return
	}
	if m.canvas.RenderBuffer == nil || m.canvas.Width() != m.width || m.canvas.Height() != m.height {
		m.canvas = newCanvas(m.width, m.height)
	} else if m.needsClear {
		// A new layout may leave gaps where widgets used to be. Clear once
		// when rendering it, even if several layout updates preceded this render.
		m.canvas.Clear()
	}
	m.needsClear = false
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

// drawLabels fetches current pane titles and input labels after borders have
// been restored. Label-only changes do not need a new layout plan.
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
	for _, leaf := range plan.leaves {
		if leaf.widget != m.input || leaf.content.Empty() {
			continue
		}
		for _, label := range m.input.Labels() {
			position := label.Position.Add(leaf.content.Min)
			drawLabel(canvas, plan.borders, label.Text, position.X, leaf.outer.Max.X, position.Y)
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
