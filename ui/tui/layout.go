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
	rules   []widget.Rule
	borders borderGrid
	output  image.Rectangle
	// Only auto-sized panes (or their auto-sized ancestors) depend on text.
	// A single conservative flag avoids a per-widget invalidation graph.
	contentSized bool
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

type borderEdges uint8

const (
	borderLeft borderEdges = 1 << iota
	borderRight
	borderTop
	borderBottom
	borderAll = borderLeft | borderRight | borderTop | borderBottom
)

func (m *Model) resolveLayout() layoutPlan {
	plan := layoutPlan{}
	if m.width <= 0 || m.height <= 0 {
		return plan
	}
	root, ok := m.resolveNode(m.layout.Root, m.width, axisVertical)
	if !ok {
		return plan
	}
	assignSharedEdges(root, 0)
	m.placeNode(root, image.Rect(0, 0, m.width, m.height), axisVertical, 0, &plan)
	plan.contentSized = contentSized(root, false)
	plan.borders = newBorderGrid(m.width, m.height)
	m.planBorders(&plan)
	for _, leaf := range plan.leaves {
		if leaf.widget == m.output {
			plan.output = leaf.content
			break
		}
	}
	return plan
}

func contentSized(node *resolvedNode, auto bool) bool {
	auto = auto || node.node.Size.Kind == ui.LayoutSizeAuto
	if node.node.Type == ui.LayoutTypePane {
		return auto
	}
	for _, child := range node.children {
		if contentSized(child, auto) {
			return true
		}
	}
	return false
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
}

// paneBorders maps the canonical pane border mode to rendered edges.
func paneBorders(border ui.PaneBorder) borderEdges {
	if border == "" {
		return borderAll
	}
	switch border {
	case ui.PaneBorderNone:
		return 0
	case ui.PaneBorderHorizontal:
		return borderTop | borderBottom
	case ui.PaneBorderFull:
		return borderAll
	}
	return borderAll
}

func (m *Model) resolveWidget(node ui.LayoutNode, w widget.Widget, availableWidth int) (*resolvedNode, bool) {
	// Empty bars collapse even when assigned a fixed track. Measuring a widget
	// must not change the geometry used by input handling.
	height := w.MeasureHeight(max(1, availableWidth), ui.MaxLayoutCells)
	if height <= 0 {
		return nil, false
	}
	return &resolvedNode{
		node: node, widget: w, hasInput: w == m.input,
		edges: widgetBorders(w, availableWidth, height),
	}, true
}

func widgetBorders(w widget.Widget, width, height int) borderEdges {
	decorated, ok := w.(interface{ Rules(int, int) []widget.Rule })
	if !ok || width <= 0 || height <= 0 {
		return 0
	}
	var edges borderEdges
	for _, rule := range decorated.Rules(width, height) {
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

func (m *Model) resolveBar(node ui.LayoutNode, name string, availableWidth int) (*resolvedNode, bool) {
	bar, ok := m.bars[name]
	if !ok {
		return nil, false
	}
	return m.resolveWidget(node, bar, availableWidth)
}

// resolvePane selects a named buffer, creating it on first placement so a
// declared pane renders as an empty titled box instead of silently vanishing.
func (m *Model) resolvePane(node ui.LayoutNode, name string) (*resolvedNode, bool) {
	pane := m.pane(name)
	return &resolvedNode{node: node, widget: pane, edges: paneBorders(node.Border)}, true
}

// resolveNode prunes hidden placements and leaves that cannot currently
// render. Pane and bar leaves each select their own namespace.
func (m *Model) resolveNode(node ui.LayoutNode, availableWidth int, parentAxis splitAxis) (*resolvedNode, bool) {
	if node.Hidden {
		return nil, false
	}
	if parentAxis == axisHorizontal {
		if node.Size.Kind == ui.LayoutSizeCells {
			availableWidth = min(availableWidth, node.Size.Value)
		}
		if maximum := nodeMaximum(node); maximum > 0 {
			availableWidth = min(availableWidth, maximum)
		}
	}
	if node.IsContainer() {
		children := make([]*resolvedNode, 0, len(node.Children))
		for _, child := range node.Children {
			if resolved, ok := m.resolveNode(child, availableWidth, nodeAxis(node)); ok {
				children = append(children, resolved)
			}
		}
		if len(children) == 0 {
			return nil, false
		}
		resolved := &resolvedNode{node: node, children: children}
		for _, child := range children {
			resolved.hasInput = resolved.hasInput || child.hasInput
		}
		resolved.edges = containerBorders(node, children)
		// A hard cap can force descendants to drop chrome. Do not promise that
		// capped container's boundary to a neighbor before the fallback runs.
		if maximum := nodeMaximum(node); resolved.hasInput && maximum > 0 && maximum < m.intrinsicMinimum(resolved, parentAxis) {
			resolved.edges = 0
		}
		return resolved, true
	}

	switch node.Type {
	case ui.LayoutTypeInput:
		resolved, ok := m.resolveWidget(node, m.input, availableWidth)
		if ok && parentAxis == axisVertical {
			height := m.input.MeasureHeight(availableWidth, ui.MaxLayoutCells)
			if node.Size.Kind == ui.LayoutSizeCells {
				height = node.Size.Value
			}
			if maximum := nodeMaximum(node); maximum > 0 {
				height = min(height, maximum)
			}
			resolved.edges = widgetBorders(m.input, availableWidth, height)
		}
		return resolved, ok
	case ui.LayoutTypeSeparator:
		separator := widget.NewSeparator()
		separator.SetChar(node.SeparatorChar)
		return m.resolveWidget(node, separator, availableWidth)
	case ui.LayoutTypePane:
		return m.resolvePane(node, node.Name)
	case ui.LayoutTypeBar:
		return m.resolveBar(node, node.Name, availableWidth)
	default:
		return nil, false
	}
}

func (m *Model) placeNode(node *resolvedNode, rect image.Rectangle, parentAxis splitAxis, shared borderEdges, plan *layoutPlan) {
	if rect.Empty() {
		return
	}
	if node.widget != nil {
		leaf := node
		insets := leaf.edges | shared
		if node.node.Type != ui.LayoutTypePane {
			// Composite widgets include their own rules. Only boundaries
			// supplied by neighbors consume additional content cells.
			insets = shared &^ widgetBorders(leaf.widget, rect.Dx(), rect.Dy())
		}
		leaf.outer, leaf.content = rect, insetBorders(rect, insets)
		leaf.parentAxis = parentAxis
		plan.leaves = append(plan.leaves, leaf)
		return
	}

	axis := nodeAxis(node.node)
	allocation := m.allocateChildren(node, axisExtent(rect, axis), axis, crossExtent(rect, axis))
	childShared := childSharedEdges(node, shared, allocation.gaps, !allocation.constrained)

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
			gap := allocation.gaps[i]
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
				rule := widget.Rule{Vertical: axis == axisHorizontal,
					At: at}
				if rule.Vertical {
					rule.From, rule.To = rect.Min.Y, rect.Max.Y
				} else {
					rule.From, rule.To = rect.Min.X, rect.Max.X
				}
				plan.rules = append(plan.rules, rule)
			}
			position += gap
			if allocation.overlaps[i] {
				position--
			}
		}
	}
}
