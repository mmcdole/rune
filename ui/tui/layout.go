package tui

import (
	"image"

	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/widget"
)

type splitAxis uint8

const (
	axisHorizontal splitAxis = iota
	axisVertical
)

// layoutPlan is the complete geometry for one frame. The same plan sizes the
// output window for interaction and places every leaf for rendering.
type layoutPlan struct {
	leaves  []*resolvedNode
	borders borderGrid
	output  image.Rectangle
	// Text changes only affect geometry for these panes or their auto ancestors.
	autoPanes map[string]bool
}

// resolvedNode is an active tree node. Leaves carry a widget and its geometry;
// containers carry children. parentAxis determines separator orientation.
type resolvedNode struct {
	node       ui.LayoutNode
	widget     widget.Widget
	children   []*resolvedNode
	outer      image.Rectangle
	content    image.Rectangle
	edges      borderEdges // requested boundaries, used for measurement and seam allocation
	shared     borderEdges // anticipated shared cells, used only for measurement
	parentAxis splitAxis
	hasInput   bool
}

func (m *Model) resolveLayout() layoutPlan {
	plan := layoutPlan{}
	if m.width <= 0 || m.height <= 0 {
		return plan
	}
	root := m.resolveNode(m.layout.Root, m.width, axisVertical)
	if root == nil {
		return plan
	}
	assignSharedEdges(root, 0)
	plan.borders = newBorderGrid(m.width, m.height)
	m.placeNode(root, image.Rect(0, 0, m.width, m.height), axisVertical, 0, &plan)
	plan.collectAutoPanes(root, false)
	m.planBorders(&plan)
	for _, leaf := range plan.leaves {
		if leaf.widget == m.output {
			plan.output = leaf.content
			break
		}
	}
	return plan
}

func (p *layoutPlan) collectAutoPanes(node *resolvedNode, auto bool) {
	auto = auto || node.node.Size.Kind == ui.LayoutSizeAuto
	if auto && node.node.Type == ui.LayoutTypePane {
		if p.autoPanes == nil {
			p.autoPanes = make(map[string]bool)
		}
		p.autoPanes[node.node.Name] = true
	}
	for _, child := range node.children {
		p.collectAutoPanes(child, auto)
	}
}

// applyLayout resolves current state and applies all leaf rectangles once at
// the end of Update, before geometry-dependent navigation and rendering.
func (m *Model) applyLayout() {
	if !m.initialized {
		return
	}
	plan := m.resolveLayout()
	if width, height := plan.output.Dx(), plan.output.Dy(); width > 0 && height > 0 {
		m.output.SetSize(width, height)
	} else {
		m.output.SetFallbackSize(m.width, m.height)
	}
	for _, leaf := range plan.leaves {
		if !leaf.content.Empty() && leaf.widget != m.output {
			leaf.widget.SetSize(leaf.content.Dx(), leaf.content.Dy())
		}
	}
	m.layoutPlan = plan
	m.needsClear = true
}

// resolveNode prunes hidden placements and leaves that cannot currently
// render, returning nil for an inactive subtree. Leaf selection and initial
// border measurement live here; placement applies the final dimensions later.
func (m *Model) resolveNode(node ui.LayoutNode, availableWidth int, parentAxis splitAxis) *resolvedNode {
	if node.Hidden {
		return nil
	}
	if parentAxis == axisHorizontal {
		if node.Size.Kind == ui.LayoutSizeCells {
			availableWidth = min(availableWidth, node.Size.Value)
		}
		if maximum := nodeMaximum(node); maximum > 0 {
			availableWidth = min(availableWidth, maximum)
		}
	}
	resolved := &resolvedNode{node: node}
	if node.IsContainer() {
		resolved.children = make([]*resolvedNode, 0, len(node.Children))
		for _, childNode := range node.Children {
			if child := m.resolveNode(childNode, availableWidth, nodeAxis(node)); child != nil {
				resolved.children = append(resolved.children, child)
				resolved.hasInput = resolved.hasInput || child.hasInput
			}
		}
		if len(resolved.children) == 0 {
			return nil
		}
		resolved.edges = containerBorders(node, resolved.children)
		// A hard cap can force descendants to drop borders. Do not promise that
		// capped container's boundary to a neighbor before the fallback runs.
		if maximum := nodeMaximum(node); resolved.hasInput && maximum > 0 && maximum < m.intrinsicMinimum(resolved, parentAxis) {
			resolved.edges = 0
		}
		return resolved
	}

	switch node.Type {
	case ui.LayoutTypeInput:
		var height int
		if parentAxis == axisVertical && node.Size.Kind == ui.LayoutSizeCells {
			height = node.Size.Value
		} else {
			height = m.input.MeasureHeight(max(1, availableWidth), ui.MaxLayoutCells)
		}
		if parentAxis == axisVertical {
			if maximum := nodeMaximum(node); maximum > 0 {
				height = min(height, maximum)
			}
		}
		resolved.widget, resolved.hasInput = m.input, true
		resolved.edges = m.inputBorders(availableWidth, height)
	case ui.LayoutTypeSeparator:
		resolved.widget = widget.NewSeparator(node.SeparatorChar, m.styles.PaneBorder)
	case ui.LayoutTypePane:
		// First placement creates an empty named buffer, so it remains a
		// visible pane even before any text is written to it.
		resolved.widget = m.pane(node.Name)
		resolved.edges = paneBorders(node.Border)
	case ui.LayoutTypeBar:
		bar := m.bars[node.Name]
		// Missing and empty bars collapse even when assigned a fixed track.
		if bar == nil || bar.MeasureHeight(availableWidth, 1) == 0 {
			return nil
		}
		resolved.widget = bar
	default:
		return nil
	}
	return resolved
}

func (m *Model) placeNode(node *resolvedNode, rect image.Rectangle, parentAxis splitAxis, shared borderEdges, plan *layoutPlan) {
	if rect.Empty() {
		return
	}
	if node.widget != nil {
		edges := node.edges
		if node.widget == m.input {
			// Constrained input may drop borders at the allocated size.
			edges = m.inputBorders(rect.Dx(), rect.Dy())
		}
		node.outer, node.content = rect, insetBorders(rect, contentInsets(node.node.Type, edges, shared))
		node.parentAxis = parentAxis
		plan.leaves = append(plan.leaves, node)
		return
	}

	axis := nodeAxis(node.node)
	allocation := m.allocateChildren(node, axisExtent(rect, axis), axis, crossExtent(rect, axis))
	childShared := childSharedEdges(node, shared, allocation.boundaries, !allocation.constrained)

	position := rect.Min.X
	if axis == axisVertical {
		position = rect.Min.Y
	}
	for i, child := range node.children {
		size := allocation.sizes[i]
		childArea := childRect(rect, axis, position, size).Intersect(rect)
		inherited := childShared[i]
		if childArea.Min.X != rect.Min.X {
			inherited &^= shared & borderLeft
		}
		if childArea.Max.X != rect.Max.X {
			inherited &^= shared & borderRight
		}
		if childArea.Min.Y != rect.Min.Y {
			inherited &^= shared & borderTop
		}
		if childArea.Max.Y != rect.Max.Y {
			inherited &^= shared & borderBottom
		}
		m.placeNode(child, childArea, axis, inherited, plan)
		position += size
		if i < len(node.children)-1 {
			gap := allocation.boundaries[i].gap
			// The tiny-terminal fallback drops gaps, and with them the cell a
			// divider draws in, so dividers degrade away with their gap.
			if node.node.Dividers && !allocation.constrained {
				at := position + (gap-1)/2
				if gap == 0 {
					before, _ := seamBorders(node.children, i, axis)
					at = position
					if before {
						at--
					}
				}
				if axis == axisHorizontal {
					plan.borders.markVertical(at, rect.Min.Y, rect.Max.Y)
				} else {
					plan.borders.markHorizontal(at, rect.Min.X, rect.Max.X)
				}
			}
			position += gap
			if allocation.boundaries[i].overlap {
				position--
			}
		}
	}
}
