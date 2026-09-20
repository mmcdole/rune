package tui

import (
	"image"

	uv "github.com/charmbracelet/ultraviolet"

	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/widget"
)

type borderEdges uint8

const (
	borderLeft borderEdges = 1 << iota
	borderRight
	borderTop
	borderBottom
	borderAll = borderLeft | borderRight | borderTop | borderBottom
)

// borderGrid records line connectivity independently from content rendering.
// Titles are rendered after the grid, so a shared lower pane header naturally
// owns a horizontal pane boundary.
type borderGrid struct {
	width, height int
	horizontal    []bool
	vertical      []bool
	cells         []borderCell
}

type borderCell struct {
	x, y int
	cell *uv.Cell
}

func newBorderGrid(width, height int) borderGrid {
	return borderGrid{
		width: width, height: height,
		horizontal: make([]bool, max(0, width*height)),
		vertical:   make([]bool, max(0, width*height)),
	}
}

func (f borderGrid) inside(x, y int) bool {
	return x >= 0 && y >= 0 && x < f.width && y < f.height
}

func (f borderGrid) markRule(rule widget.Rule) {
	if rule.Vertical {
		f.markVertical(rule.At, rule.From, rule.To)
	} else {
		f.markHorizontal(rule.At, rule.From, rule.To)
	}
}

func (f borderGrid) at(cells []bool, x, y int) bool {
	return f.inside(x, y) && cells[y*f.width+x]
}

func (f borderGrid) markHorizontal(y, left, right int) {
	if y < 0 || y >= f.height {
		return
	}
	left, right = max(0, left), min(f.width, right)
	for x := left; x < right; x++ {
		f.horizontal[y*f.width+x] = true
	}
}

func (f borderGrid) markVertical(x, top, bottom int) {
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
func (f borderGrid) glyph(x, y int) string {
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
func (f borderGrid) connects(x, y int, along, across []bool, ownAlong bool) bool {
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

// paneBorders maps the canonical pane border mode to rendered edges.
func paneBorders(border ui.PaneBorder) borderEdges {
	switch border {
	case ui.PaneBorderNone:
		return 0
	case ui.PaneBorderHorizontal:
		return borderTop | borderBottom
	}
	return borderAll
}

func (m *Model) inputBorders(width, height int) borderEdges {
	if width <= 0 || height <= 0 {
		return 0
	}
	var edges borderEdges
	for _, rule := range m.input.Rules(width, height) {
		if rule.Vertical && rule.From == 0 && rule.To == height {
			if rule.At == 0 {
				edges |= borderLeft
			}
			if rule.At == width-1 {
				edges |= borderRight
			}
		} else if !rule.Vertical && rule.From == 0 && rule.To == width {
			if rule.At == 0 {
				edges |= borderTop
			}
			if rule.At == height-1 {
				edges |= borderBottom
			}
		}
	}
	return edges
}

func borderedPane(leaf *resolvedNode) bool {
	return leaf.node.Type == ui.LayoutTypePane && leaf.edges != 0
}

// joinableSeparator reports a default-character separator placed by a column. It
// draws through the border grid so it joins dividers and pane borders. A custom
// character, or a separator placed by a row, keeps the widget rendering.
func joinableSeparator(leaf *resolvedNode) bool {
	return leaf.node.Type == ui.LayoutTypeSeparator &&
		leaf.node.SeparatorChar == "" && leaf.parentAxis == axisVertical
}

// Panes have external borders. Composite widgets already include their own
// rules, and only need insets for boundaries supplied by the surrounding tree.
// Measurement supplies anticipated edges; placement supplies the final ones.
func contentInsets(nodeType string, edges, shared borderEdges) borderEdges {
	if nodeType == ui.LayoutTypePane {
		return edges | shared
	}
	return shared &^ edges
}

func insetBorders(rect image.Rectangle, edges borderEdges) image.Rectangle {
	if edges&borderLeft != 0 && rect.Min.X < rect.Max.X {
		rect.Min.X++
	}
	if edges&borderRight != 0 && rect.Min.X < rect.Max.X {
		rect.Max.X--
	}
	if edges&borderTop != 0 && rect.Min.Y < rect.Max.Y {
		rect.Min.Y++
	}
	if edges&borderBottom != 0 && rect.Min.Y < rect.Max.Y {
		rect.Max.Y--
	}
	return rect
}

// planBorders adds leaf borders to the dividers marked during placement.
// Shared coordinates merge in borderGrid, including T and cross junctions.
// Default separators give up their content rectangle to the joined border.
func (m *Model) planBorders(plan *layoutPlan) {
	for i := range plan.leaves {
		leaf := plan.leaves[i]
		if leaf.widget == m.input {
			for _, rule := range m.input.Rules(leaf.content.Dx(), leaf.content.Dy()) {
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
				plan.borders.markRule(rule)
			}
		}
		if joinableSeparator(leaf) {
			plan.borders.markHorizontal(leaf.outer.Min.Y, leaf.outer.Min.X, leaf.outer.Max.X)
			leaf.content = image.Rectangle{}
			continue
		}
		if !borderedPane(leaf) {
			continue
		}
		outer := leaf.outer
		if leaf.edges&borderTop != 0 {
			plan.borders.markHorizontal(outer.Min.Y, outer.Min.X, outer.Max.X)
		}
		if leaf.edges&borderBottom != 0 {
			plan.borders.markHorizontal(outer.Max.Y-1, outer.Min.X, outer.Max.X)
		}
		if leaf.edges&borderLeft != 0 {
			plan.borders.markVertical(outer.Min.X, outer.Min.Y, outer.Max.Y)
		}
		if leaf.edges&borderRight != 0 {
			plan.borders.markVertical(outer.Max.X-1, outer.Min.Y, outer.Max.Y)
		}
	}
}

// resolveBorderCells prepares a layout's borders on its first render. Geometry
// updates before that render can replace the layout without preparing unused cells.
func (m *Model) resolveBorderCells(grid *borderGrid) {
	grid.cells = []borderCell{} // non-nil also records an empty, prepared grid
	for index := range grid.horizontal {
		if !grid.horizontal[index] && !grid.vertical[index] {
			continue
		}
		x, y := index%grid.width, index/grid.width
		glyph := grid.glyph(x, y)
		cell := m.borderCells[glyph]
		if cell == nil {
			cell = styledCell(m.styles.PaneBorder.Render(glyph))
			m.borderCells[glyph] = cell
		}
		grid.cells = append(grid.cells, borderCell{x: x, y: y, cell: cell})
	}
}
