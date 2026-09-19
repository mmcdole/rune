package tui

import (
	"image"

	"github.com/mmcdole/rune/ui"
)

func countEdges(edges, mask borderEdges) int {
	n := 0
	for bits := edges & mask; bits != 0; bits &= bits - 1 {
		n++
	}
	return n
}

func (n *resolvedNode) hasBorder(edge borderEdges) bool {
	return n != nil && n.edges&edge != 0
}

func seamBorders(children []*resolvedNode, index int, axis splitAxis) (before, after bool) {
	if axis == axisHorizontal {
		return children[index].hasBorder(borderRight), children[index+1].hasBorder(borderLeft)
	}
	return children[index].hasBorder(borderBottom), children[index+1].hasBorder(borderTop)
}

// boundaryGaps returns the space between each pair of active children. With
// no declared gap, a divider reuses an existing framed seam; otherwise it
// reserves one cell for a rule.
func boundaryGaps(node ui.LayoutNode, children []*resolvedNode, axis splitAxis) []int {
	gaps := make([]int, max(0, len(children)-1))
	for i := range gaps {
		gaps[i] = node.Gap
		before, after := seamBorders(children, i, axis)
		if node.Dividers && gaps[i] == 0 && !before && !after {
			gaps[i] = 1
		}
	}
	return gaps
}

func gapCells(gaps []int) int {
	total := 0
	for _, gap := range gaps {
		total += gap
	}
	return total
}

// A container requests the union of its descendants' cross-axis edges. When
// that edge is shared, descendants without their own border reserve the same
// boundary cell rather than rendering content into it.
func containerBorders(node ui.LayoutNode, children []*resolvedNode) borderEdges {
	if len(children) == 0 {
		return 0
	}
	any := func(edge borderEdges) bool {
		for _, child := range children {
			if child.hasBorder(edge) {
				return true
			}
		}
		return false
	}
	var edges borderEdges
	if node.Type == ui.LayoutTypeRow {
		if children[0].hasBorder(borderLeft) {
			edges |= borderLeft
		}
		if children[len(children)-1].hasBorder(borderRight) {
			edges |= borderRight
		}
		if any(borderTop) {
			edges |= borderTop
		}
		if any(borderBottom) {
			edges |= borderBottom
		}
		return edges
	}
	if children[0].hasBorder(borderTop) {
		edges |= borderTop
	}
	if children[len(children)-1].hasBorder(borderBottom) {
		edges |= borderBottom
	}
	if any(borderLeft) {
		edges |= borderLeft
	}
	if any(borderRight) {
		edges |= borderRight
	}
	return edges
}

func nodeAxis(node ui.LayoutNode) splitAxis {
	if node.Type == ui.LayoutTypeRow {
		return axisHorizontal
	}
	return axisVertical
}

func axisExtent(rect image.Rectangle, axis splitAxis) int {
	if axis == axisHorizontal {
		return rect.Dx()
	}
	return rect.Dy()
}

func crossExtent(rect image.Rectangle, axis splitAxis) int {
	if axis == axisHorizontal {
		return rect.Dy()
	}
	return rect.Dx()
}

func childRect(parent image.Rectangle, axis splitAxis, position, size int) image.Rectangle {
	if axis == axisHorizontal {
		return image.Rect(position, parent.Min.Y, position+size, parent.Max.Y)
	}
	return image.Rect(parent.Min.X, position, parent.Max.X, position+size)
}

// Panes have external chrome. Composite widgets already include their own
// rules, and only need insets for boundaries supplied by the surrounding tree.
func contentInsets(node *resolvedNode) borderEdges {
	if node.node.Type == ui.LayoutTypePane {
		return node.edges | node.shared
	}
	return node.shared &^ node.edges
}

func childSharedEdges(node *resolvedNode, inherited borderEdges, gaps []int, seams bool) []borderEdges {
	axis := nodeAxis(node.node)
	start, end, cross := borderLeft, borderRight, borderTop|borderBottom
	if axis == axisVertical {
		start, end, cross = borderTop, borderBottom, borderLeft|borderRight
	}
	shared := make([]borderEdges, len(node.children))
	for i := range shared {
		shared[i] = inherited & cross
		if i == 0 {
			shared[i] |= inherited & start
		}
		if i == len(shared)-1 {
			shared[i] |= inherited & end
		}
	}
	if seams {
		for i, gap := range gaps {
			before, after := seamBorders(node.children, i, axis)
			if gap == 0 && (node.node.Dividers || before && after) {
				if before {
					shared[i] |= end
				}
				if after {
					shared[i+1] |= start
				}
			}
		}
	}
	return shared
}

func assignSharedEdges(node *resolvedNode, inherited borderEdges) {
	node.shared = inherited
	if node.widget != nil {
		return
	}
	shared := childSharedEdges(node, inherited, boundaryGaps(node.node, node.children, nodeAxis(node.node)), true)
	for i, child := range node.children {
		assignSharedEdges(child, shared[i])
	}
}

func (m *Model) leafPreferred(leaf *resolvedNode, axis splitAxis, cross int) int {
	if axis != axisVertical {
		return 1
	}
	chrome := insetBorders(image.Rect(0, 0, max(0, cross), ui.MaxLayoutCells), contentInsets(leaf))
	frameHeight := ui.MaxLayoutCells - chrome.Dy()
	limit := ui.MaxLayoutCells
	if m.height > 0 {
		limit = min(limit, m.height)
	}
	if maximum := nodeMaximum(leaf.node); maximum > 0 {
		limit = min(limit, maximum)
	}
	return frameHeight + leaf.widget.MeasureHeight(max(1, chrome.Dx()), max(0, limit-frameHeight))
}

func (m *Model) preferred(node *resolvedNode, axis splitAxis, cross int) int {
	if node.widget != nil {
		return m.leafPreferred(node, axis, cross)
	}

	direction := nodeAxis(node.node)
	if direction == axis {
		gaps := boundaryGaps(node.node, node.children, direction)
		total := gapCells(gaps) - countTrue(seamOverlaps(node.children, gaps, direction))
		for _, child := range node.children {
			desired := 0
			switch child.node.Size.Kind {
			case ui.LayoutSizeCells:
				desired = child.node.Size.Value
			default:
				// An auto container asks every non-fixed child for its
				// intrinsic preference. Fraction and percent rules take
				// effect later if the container is assigned a different size.
				desired = m.preferred(child, axis, cross)
			}
			desired = max(desired, m.minimum(child, axis))
			if maximum := nodeMaximum(child.node); maximum > 0 {
				desired = min(desired, maximum)
			}
			total += desired
		}
		return total
	}

	// A row's preferred height depends on the widths assigned to its children;
	// wrapped input, search, and multiline widgets are then measurable side by
	// side.
	if axis == axisVertical && direction == axisHorizontal {
		widths := m.allocateChildren(node, max(0, cross), axisHorizontal, 1).sizes
		preferred := 0
		for i, child := range node.children {
			preferred = max(preferred, m.preferred(child, axisVertical, widths[i]))
		}
		return preferred
	}

	// Widgets do not expose a preferred-width contract. Layout validation rejects
	// auto-sized children of rows, so recursive cross-axis measurement uses only
	// intrinsic minima.
	preferred := 0
	for _, child := range node.children {
		preferred = max(preferred, m.intrinsicMinimum(child, axis))
	}
	return preferred
}

func (m *Model) minimum(node *resolvedNode, axis splitAxis) int {
	minimum := 0
	if node.node.MinSize != nil {
		minimum = *node.node.MinSize
	}
	minimum = max(minimum, m.intrinsicMinimum(node, axis))
	if node.widget != nil && node.node.Type != ui.LayoutTypePane && node.node.Size.Kind == ui.LayoutSizeCells {
		minimum = min(minimum, node.node.Size.Value)
	}
	// max_size is a hard user constraint. If it is smaller than intrinsic
	// chrome, degrade that chrome inside the capped rectangle instead of making
	// the parent allocation inconsistent and triggering a global fallback.
	if maximum := nodeMaximum(node.node); maximum > 0 {
		minimum = min(minimum, maximum)
	}
	return minimum
}

// intrinsicMinimum is independent of the node's own track constraint. A
// node's min_size is expressed along its parent's axis and therefore must not
// leak into cross-axis measurement.
func (m *Model) intrinsicMinimum(node *resolvedNode, axis splitAxis) int {
	if node.widget != nil {
		minimum := node.widget.MinimumSize()
		if axis == axisVertical {
			return minimum.Y + countEdges(contentInsets(node), borderTop|borderBottom)
		}
		return minimum.X + countEdges(contentInsets(node), borderLeft|borderRight)
	}

	direction := nodeAxis(node.node)
	if direction == axis {
		gaps := boundaryGaps(node.node, node.children, direction)
		total := gapCells(gaps) - countTrue(seamOverlaps(node.children, gaps, direction))
		for _, child := range node.children {
			total += m.minimum(child, axis)
		}
		return max(0, total)
	}
	widest := 0
	for _, child := range node.children {
		widest = max(widest, m.intrinsicMinimum(child, axis))
	}
	return widest
}

func nodeMaximum(node ui.LayoutNode) int {
	if node.MaxSize == nil {
		return 0
	}
	return *node.MaxSize
}

// seamOverlaps reports where resolved geometry lets adjacent framed panes share
// one boundary cell.
func seamOverlaps(children []*resolvedNode, gaps []int, axis splitAxis) []bool {
	overlaps := make([]bool, max(0, len(children)-1))
	for i := range overlaps {
		if gaps[i] == 0 {
			before, after := seamBorders(children, i, axis)
			overlaps[i] = before && after
		}
	}
	return overlaps
}

func countTrue(values []bool) int {
	total := 0
	for _, value := range values {
		if value {
			total++
		}
	}
	return total
}

type childAllocation struct {
	sizes       []int
	gaps        []int
	overlaps    []bool
	constrained bool
}

func fallbackAllocation(
	children []*resolvedNode,
	extent int,
	axis splitAxis,
	tracks []ui.AxisTrack,
) childAllocation {
	count := len(children)
	result := childAllocation{
		constrained: true,
		sizes:       make([]int, count),
		gaps:        make([]int, max(0, count-1)),
		overlaps:    make([]bool, max(0, count-1)),
	}
	extent = max(0, extent)
	if extent == 0 || count == 0 {
		return result
	}

	// Retry without gaps or ordinary minima, preserving the original sizing
	// rules and hard maxima. Keep the editor reachable on either axis.
	relaxed := append([]ui.AxisTrack(nil), tracks...)
	for i := range relaxed {
		relaxed[i].Min = 0
	}
	remaining := extent
	// Reserve input's subtree minimum first, then other declared/measured minima
	// in layout order. No pane names or scrollback-specific priority here.
	for _, protectInput := range []bool{true, false} {
		for i, child := range children {
			if child.hasInput != protectInput {
				continue
			}
			minimum := tracks[i].Min
			if child.hasInput {
				minimum = max(1, interactionMinimum(child, axis))
			}
			grant := min(minimum, remaining)
			if relaxed[i].Max > 0 {
				grant = min(grant, relaxed[i].Max)
			}
			relaxed[i].Min = grant
			remaining -= grant
		}
	}

	if sizes, err := ui.AllocateAxis(extent, 0, relaxed); err == nil {
		result.sizes = sizes
		return result
	}

	// Validated trees cannot reach this branch; it keeps direct, invalid Go
	// callers bounded and preserves any protected rows without risking a loop.
	remaining = extent
	for i, track := range relaxed {
		grant := min(track.Min, remaining)
		result.sizes[i] = grant
		remaining -= grant
	}
	for remaining > 0 {
		grew := false
		for i, track := range relaxed {
			if remaining == 0 {
				break
			}
			if track.Max > 0 && result.sizes[i] >= track.Max {
				continue
			}
			result.sizes[i]++
			remaining--
			grew = true
		}
		if !grew {
			break
		}
	}
	return result
}

// Under pressure, preserve content rather than the chrome included in normal
// minima. In particular the editor can fall back to one editable row.
func interactionMinimum(node *resolvedNode, axis splitAxis) int {
	if node.widget != nil {
		minimum := node.widget.MinimumSize()
		if axis == axisHorizontal {
			return minimum.X
		}
		if node.hasInput {
			return 1
		}
		return minimum.Y
	}
	minimum := 0
	for _, child := range node.children {
		value := interactionMinimum(child, axis)
		if nodeAxis(node.node) == axis {
			minimum += value
		} else {
			minimum = max(minimum, value)
		}
	}
	return minimum
}

func (m *Model) allocateChildren(node *resolvedNode, extent int, axis splitAxis, cross int) childAllocation {
	gaps := boundaryGaps(node.node, node.children, axis)
	overlaps := seamOverlaps(node.children, gaps, axis)
	effectiveExtent := max(0, extent) - gapCells(gaps) + countTrue(overlaps)
	tracks := make([]ui.AxisTrack, len(node.children))
	for i, child := range node.children {
		track := ui.AxisTrack{
			Size: child.node.Size,
			Min:  m.minimum(child, axis),
			Max:  nodeMaximum(child.node),
		}
		if track.Size.Kind == ui.LayoutSizeAuto {
			track.Auto = m.preferred(child, axis, cross)
		}
		tracks[i] = track
	}
	sizes, err := ui.AllocateAxis(effectiveExtent, 0, tracks)
	if err != nil {
		return fallbackAllocation(node.children, extent, axis, tracks)
	}

	used := gapCells(gaps) - countTrue(overlaps)
	for i, size := range sizes {
		if size < 0 || (i < len(overlaps) && overlaps[i] && (size == 0 || sizes[i+1] == 0)) {
			return fallbackAllocation(node.children, extent, axis, tracks)
		}
		used += size
		child := node.children[i]
		if child.widget != nil && child.node.Type != ui.LayoutTypePane {
			width, height := cross, size
			start, end := borderTop, borderBottom
			if axis == axisHorizontal {
				width, height, start, end = size, cross, borderLeft, borderRight
			}
			var required borderEdges
			if i > 0 && overlaps[i-1] {
				required |= start
			}
			if i < len(overlaps) && overlaps[i] {
				required |= end
			}
			if required&^widgetBorders(child.widget, width, height) != 0 {
				return fallbackAllocation(node.children, extent, axis, tracks)
			}
		}
	}
	if used > extent {
		return fallbackAllocation(node.children, extent, axis, tracks)
	}
	return childAllocation{sizes: sizes, gaps: gaps, overlaps: overlaps}
}
