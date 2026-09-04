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
// output viewport for interaction and places every leaf for rendering.
type layoutPlan struct {
	leaves []*layoutNode
	rules  []widget.Rule
	frame  frameGrid
	output image.Rectangle
}

// layoutNode is an active tree node. Leaves carry a surface and its geometry;
// containers carry children. parentAxis determines separator orientation.
type layoutNode struct {
	node       ui.LayoutNode
	surface    widget.Widget
	children   []*layoutNode
	outer      image.Rectangle
	content    image.Rectangle
	frames     frameEdges // requested boundaries, used for measurement and seam allocation
	shared     frameEdges // anticipated shared cells, used only for measurement
	parentAxis splitAxis
	hasInput   bool
}

type frameEdges uint8

const (
	frameLeft frameEdges = 1 << iota
	frameRight
	frameTop
	frameBottom
	frameAll = frameLeft | frameRight | frameTop | frameBottom
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
	plan.frame = newFrameGrid(m.width, m.height)
	m.planFrames(&plan)
	for _, leaf := range plan.leaves {
		if leaf.surface == m.output {
			plan.output = leaf.content
			break
		}
	}
	return plan
}

func (m *Model) applyOutputGeometry(plan layoutPlan) (scrollStateChanged bool) {
	beforeMode := m.output.viewport.Mode()
	beforeLines := m.output.viewport.NewLineCount()
	width, height := plan.output.Dx(), plan.output.Dy()
	if width > 0 && height > 0 {
		m.output.setGeometry(width, height)
	} else {
		m.output.setFallbackGeometry(m.width, m.height)
	}
	return beforeMode != m.output.viewport.Mode() ||
		beforeLines != m.output.viewport.NewLineCount()
}

// applyLayout resolves current state and applies all leaf rectangles once at
// the end of Update, before geometry-dependent navigation and painting.
func (m *Model) applyLayout() bool {
	if !m.initialized {
		return false
	}
	plan := m.resolveLayout()
	changed := m.applyOutputGeometry(plan)
	for _, leaf := range plan.leaves {
		if !leaf.content.Empty() && leaf.surface != m.output {
			leaf.surface.SetSize(leaf.content.Dx(), leaf.content.Dy())
		}
	}
	m.layoutPlan = plan
	return changed
}

// paneFrames maps the canonical pane border mode to rendered edges.
func paneFrames(border ui.PaneBorder) frameEdges {
	if border == "" {
		return frameAll
	}
	switch border {
	case ui.PaneBorderNone:
		return 0
	case ui.PaneBorderHorizontal:
		return frameTop | frameBottom
	case ui.PaneBorderFull:
		return frameAll
	}
	return frameAll
}

func (m *Model) resolveWidget(node ui.LayoutNode, w widget.Widget, availableWidth int) (*layoutNode, bool) {
	// Empty bars collapse even when assigned a fixed track. Measuring a surface
	// must not change the geometry used by input handling.
	height := w.MeasureHeight(max(1, availableWidth), ui.MaxLayoutCells)
	if height <= 0 {
		return nil, false
	}
	return &layoutNode{
		node: node, surface: w, hasInput: w == m.input,
		frames: surfaceFrames(w, availableWidth, height),
	}, true
}

func surfaceFrames(surface widget.Widget, width, height int) frameEdges {
	decorated, ok := surface.(interface{ Rules(int, int) []widget.Rule })
	if !ok || width <= 0 || height <= 0 {
		return 0
	}
	var edges frameEdges
	for _, rule := range decorated.Rules(width, height) {
		if rule.Vertical && rule.From == 0 && rule.To == height {
			if rule.At == 0 {
				edges |= frameLeft
			}
			if rule.At == width-1 {
				edges |= frameRight
			}
		} else if !rule.Vertical && rule.From == 0 && rule.To == width {
			if rule.At == 0 {
				edges |= frameTop
			}
			if rule.At == height-1 {
				edges |= frameBottom
			}
		}
	}
	return edges
}

func (m *Model) resolveBar(node ui.LayoutNode, name string, availableWidth int) (*layoutNode, bool) {
	bar, ok := m.bars[name]
	if !ok {
		return nil, false
	}
	return m.resolveWidget(node, bar, availableWidth)
}

// resolvePane selects a named buffer, creating it on first placement so a
// declared pane renders as an empty titled box instead of silently vanishing.
func (m *Model) resolvePane(node ui.LayoutNode, name string) (*layoutNode, bool) {
	pane := m.panes.Create(name)
	return &layoutNode{node: node, surface: pane, frames: paneFrames(node.Border)}, true
}

// resolveNode prunes hidden placements and leaves that cannot currently
// render. Pane and bar leaves each select their own resource namespace.
func (m *Model) resolveNode(node ui.LayoutNode, availableWidth int, parentAxis splitAxis) (*layoutNode, bool) {
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
		children := make([]*layoutNode, 0, len(node.Children))
		for _, child := range node.Children {
			if resolved, ok := m.resolveNode(child, availableWidth, nodeAxis(node)); ok {
				children = append(children, resolved)
			}
		}
		if len(children) == 0 {
			return nil, false
		}
		resolved := &layoutNode{node: node, children: children}
		for _, child := range children {
			resolved.hasInput = resolved.hasInput || child.hasInput
		}
		resolved.frames = containerFrameEdges(node, children)
		// A hard cap can force descendants to drop chrome. Do not promise that
		// capped container's boundary to a neighbor before the fallback runs.
		if maximum := nodeMaximum(node); resolved.hasInput && maximum > 0 && maximum < m.intrinsicMinimum(resolved, parentAxis) {
			resolved.frames = 0
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
			resolved.frames = surfaceFrames(m.input, availableWidth, height)
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

func (m *Model) placeNode(node *layoutNode, rect image.Rectangle, parentAxis splitAxis, shared frameEdges, plan *layoutPlan) {
	if rect.Empty() {
		return
	}
	if node.surface != nil {
		leaf := node
		insets := leaf.frames | shared
		if node.node.Type != ui.LayoutTypePane {
			// Composite widgets include their own rules. Only boundaries
			// supplied by neighbors consume additional content cells.
			insets = shared &^ surfaceFrames(leaf.surface, rect.Dx(), rect.Dy())
		}
		leaf.outer, leaf.content = rect, insetFrame(rect, insets)
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
			inherited &^= shared & frameLeft
		}
		if childArea.Max.X != rect.Max.X {
			inherited &^= shared & frameRight
		}
		if childArea.Min.Y != rect.Min.Y {
			inherited &^= shared & frameTop
		}
		if childArea.Max.Y != rect.Max.Y {
			inherited &^= shared & frameBottom
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
					before, _ := seamFrames(node.children, i, axis)
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
