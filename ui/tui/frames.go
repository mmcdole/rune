package tui

import (
	"image"

	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/widget"
)

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
