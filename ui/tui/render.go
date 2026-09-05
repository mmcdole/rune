package tui

import (
	"image"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	runetext "github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/widget"
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

	view.Content = m.renderPlan(m.layoutPlan)
	return view
}

func (m *Model) renderPlan(plan layoutPlan) string {
	canvas := lipgloss.NewCanvas(m.width, m.height)
	for i := range plan.leaves {
		leaf := plan.leaves[i]
		if leaf.content.Empty() {
			continue
		}
		drawStyled(canvas, leaf.surface.View(), leaf.content)
	}

	frameCells := make(map[string]*uv.Cell)
	for y := 0; y < plan.frame.height; y++ {
		for x := 0; x < plan.frame.width; x++ {
			glyph := plan.frame.glyph(x, y)
			if glyph == "" {
				continue
			}
			cell := frameCells[glyph]
			if cell == nil {
				cell = styledCell(m.styles.PaneBorder.Render(glyph))
				frameCells[glyph] = cell
			}
			canvas.SetCell(x, y, cell)
		}
	}
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
	return exactCanvasRender(canvas, m.width, m.height)
}

// frameGrid records line connectivity independently from content rendering.
// Titles are painted after the grid, so a shared lower pane header naturally
// owns a horizontal pane boundary.
type frameGrid struct {
	width, height int
	horizontal    []bool
	vertical      []bool
}

func newFrameGrid(width, height int) frameGrid {
	return frameGrid{
		width: width, height: height,
		horizontal: make([]bool, max(0, width*height)),
		vertical:   make([]bool, max(0, width*height)),
	}
}

func (f frameGrid) inside(x, y int) bool {
	return x >= 0 && y >= 0 && x < f.width && y < f.height
}

func (f frameGrid) markRule(rule widget.Rule) {
	if rule.Vertical {
		f.markVertical(rule.At, rule.From, rule.To)
	} else {
		f.markHorizontal(rule.At, rule.From, rule.To)
	}
}

func (f frameGrid) at(cells []bool, x, y int) bool {
	return f.inside(x, y) && cells[y*f.width+x]
}

func (f frameGrid) markHorizontal(y, left, right int) {
	if y < 0 || y >= f.height {
		return
	}
	left, right = max(0, left), min(f.width, right)
	for x := left; x < right; x++ {
		f.horizontal[y*f.width+x] = true
	}
}

func (f frameGrid) markVertical(x, top, bottom int) {
	if x < 0 || x >= f.width {
		return
	}
	top, bottom = max(0, top), min(f.height, bottom)
	for y := top; y < bottom; y++ {
		f.vertical[y*f.width+x] = true
	}
}

// glyph selects the box-drawing character for one cell from the lines that
// meet there. Every marked cell looks at all four neighbors, so a rule that
// ends against a border produces a tee on the border side as well as its own.
func (f frameGrid) glyph(x, y int) string {
	ownVertical := f.at(f.vertical, x, y)
	ownHorizontal := f.at(f.horizontal, x, y)
	if !ownVertical && !ownHorizontal {
		return ""
	}
	glyph := junctionGlyph(
		f.connects(x, y-1, f.vertical, f.horizontal, ownVertical),
		f.connects(x, y+1, f.vertical, f.horizontal, ownVertical),
		f.connects(x-1, y, f.horizontal, f.vertical, ownHorizontal),
		f.connects(x+1, y, f.horizontal, f.vertical, ownHorizontal),
	)
	if glyph != "" {
		return glyph
	}
	if ownVertical {
		return "│"
	}
	return "─"
}

// connects reports whether the neighbor at (x, y) continues a line along the
// given axis into the current cell. A neighbor that also carries the other
// axis, such as a pane corner beside the end of a separator, only connects
// when the current cell runs along that axis itself; otherwise the rule stops
// short instead of sprouting a tee into the corner.
func (f frameGrid) connects(x, y int, along, across []bool, ownAlong bool) bool {
	return f.at(along, x, y) && (ownAlong || !f.at(across, x, y))
}

func junctionGlyph(up, down, left, right bool) string {
	switch {
	case up && down && left && right:
		return "┼"
	case up && down && left:
		return "┤"
	case up && down && right:
		return "├"
	case up && left && right:
		return "┴"
	case down && left && right:
		return "┬"
	case up && down:
		return "│"
	case left && right:
		return "─"
	case down && right:
		return "┌"
	case down && left:
		return "┐"
	case up && right:
		return "└"
	case up && left:
		return "┘"
	case up || down:
		return "│"
	case left || right:
		return "─"
	}
	return ""
}

func framedPane(leaf *layoutNode) bool {
	return leaf.node.Type == ui.LayoutTypePane && leaf.frames != 0
}

// joinableSeparator reports a default-character separator placed by a column. It
// draws through the frame grid so it joins dividers and pane borders. A custom
// character, or a separator placed by a row, keeps the widget rendering.
func joinableSeparator(leaf *layoutNode) bool {
	return leaf.node.Type == ui.LayoutTypeSeparator &&
		leaf.node.SeparatorChar == "" && leaf.parentAxis == axisVertical
}

func insetFrame(rect image.Rectangle, frames frameEdges) image.Rectangle {
	if frames&frameLeft != 0 && rect.Min.X < rect.Max.X {
		rect.Min.X++
	}
	if frames&frameRight != 0 && rect.Min.X < rect.Max.X {
		rect.Max.X--
	}
	if frames&frameTop != 0 && rect.Min.Y < rect.Max.Y {
		rect.Min.Y++
	}
	if frames&frameBottom != 0 && rect.Min.Y < rect.Max.Y {
		rect.Max.Y--
	}
	return rect
}

// planFrames gives every piece of chrome one owner. Each framed pane marks
// its configured edges and insets its content rectangle; each container with
// dividers marks the rules between its active children; each default
// separator marks its row and gives up its content rectangle. Shared
// coordinates merge naturally in frameGrid, including T and cross junctions.
func (m *Model) planFrames(plan *layoutPlan) {
	for _, rule := range plan.rules {
		plan.frame.markRule(rule)
	}
	for i := range plan.leaves {
		leaf := plan.leaves[i]
		if decorated, ok := leaf.surface.(interface{ Rules(int, int) []widget.Rule }); ok {
			for _, rule := range decorated.Rules(leaf.content.Dx(), leaf.content.Dy()) {
				rule = rule.Translate(leaf.content.Min)
				// Extend edge-aligned rules to the boundaries reserved by the
				// surrounding layout so separators meet neighboring dividers.
				if rule.Vertical {
					if rule.At == leaf.content.Min.X {
						rule.At = leaf.outer.Min.X
					} else if rule.At == leaf.content.Max.X-1 {
						rule.At = leaf.outer.Max.X - 1
					}
				} else {
					if rule.From == leaf.content.Min.X {
						rule.From = leaf.outer.Min.X
					}
					if rule.To == leaf.content.Max.X {
						rule.To = leaf.outer.Max.X
					}
				}
				plan.frame.markRule(rule)
				plan.rules = append(plan.rules, rule)
			}
		}
		if joinableSeparator(leaf) {
			plan.frame.markHorizontal(leaf.outer.Min.Y, leaf.outer.Min.X, leaf.outer.Max.X)
			leaf.content = image.Rectangle{}
			continue
		}
		if !framedPane(leaf) {
			continue
		}
		outer := leaf.outer
		if leaf.frames&frameTop != 0 {
			plan.frame.markHorizontal(outer.Min.Y, outer.Min.X, outer.Max.X)
		}
		if leaf.frames&frameBottom != 0 {
			plan.frame.markHorizontal(outer.Max.Y-1, outer.Min.X, outer.Max.X)
		}
		if leaf.frames&frameLeft != 0 {
			plan.frame.markVertical(outer.Min.X, outer.Min.Y, outer.Max.Y)
		}
		if leaf.frames&frameRight != 0 {
			plan.frame.markVertical(outer.Max.X-1, outer.Min.Y, outer.Max.Y)
		}
	}
}

func drawStyled(canvas *lipgloss.Canvas, content string, rect image.Rectangle) {
	rect = rect.Intersect(canvas.Bounds())
	if rect.Empty() {
		return
	}
	uv.NewStyledString(content).Draw(canvas, rect)
}

func styledCell(rendered string) *uv.Cell {
	canvas := lipgloss.NewCanvas(1, 1)
	drawStyled(canvas, rendered, canvas.Bounds())
	return canvas.CellAt(0, 0)
}

// Labels cover only their text cells, never the remaining rule or a junction.
func drawFrameLabel(canvas *lipgloss.Canvas, frame frameGrid, label string, left, right, y int) {
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

// exactCanvasRender retains the Canvas cell clipping/compositing semantics
// while making the returned block exactly width by height. Ultraviolet
// deliberately trims trailing blanks from each rendered line.
func exactCanvasRender(canvas *lipgloss.Canvas, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(canvas.Render(), "\n")
	if len(lines) < height {
		lines = append(lines, make([]string, height-len(lines))...)
	} else if len(lines) > height {
		lines = lines[:height]
	}
	for i, line := range lines {
		visible := ansi.StringWidth(line)
		if visible > width {
			line = ansi.Truncate(line, width, "")
			visible = ansi.StringWidth(line)
		}
		if visible < width {
			line += strings.Repeat(" ", width-visible)
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}
