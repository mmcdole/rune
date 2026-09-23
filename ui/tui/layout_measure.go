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

// boundary records the gap and shared-cell overlap between two children.
type boundary struct {
	gap           int
	overlap       bool
	before, after borderEdges // shared edges supplied by each neighbor
}

// With no declared gap, a divider needs a cell only when neither child
// supplies a border at the boundary.
func childBoundaries(node ui.LayoutNode, children []*resolvedNode, axis splitAxis) []boundary {
	boundaries := make([]boundary, max(0, len(children)-1))
	start, end := borderLeft, borderRight
	if axis == axisVertical {
		start, end = borderTop, borderBottom
	}
	for i := range boundaries {
		before, after := seamBorders(children, i, axis)
		gap := node.Gap
		if node.Dividers && gap == 0 && !before && !after {
			gap = 1
		}
		b := boundary{gap: gap, overlap: gap == 0 && before && after}
		if b.overlap || (gap == 0 && node.Dividers) {
			if before {
				b.before = end
			}
			if after {
				b.after = start
			}
		}
		boundaries[i] = b
	}
	return boundaries
}

func boundarySpace(boundaries []boundary) int {
	total := 0
	for _, boundary := range boundaries {
		total += boundary.gap
		if boundary.overlap {
			total--
		}
	}
	return total
}

// A container requests the union of its descendants' cross-axis edges. When
// that edge is shared, descendants without their own border reserve the same
// boundary cell rather than rendering content into it. Along the split axis,
// only the first and last children can supply the outer edges.
func containerBorders(node ui.LayoutNode, children []*resolvedNode) borderEdges {
	start, end, cross := borderTop, borderBottom, borderLeft|borderRight
	if node.Type == ui.LayoutTypeRow {
		start, end, cross = borderLeft, borderRight, borderTop|borderBottom
	}
	var edges borderEdges
	for i, child := range children {
		if child == nil {
			continue
		}
		edges |= child.edges & cross
		if i == 0 {
			edges |= child.edges & start
		}
		if i == len(children)-1 {
			edges |= child.edges & end
		}
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

// The allocation carries the seam decisions. Constrained allocations have
// empty boundaries, so they inherit outer edges without sharing sibling cells.
func childSharedEdges(node *resolvedNode, index int, inherited borderEdges, boundaries []boundary) borderEdges {
	start, end, cross := borderLeft, borderRight, borderTop|borderBottom
	if nodeAxis(node.node) == axisVertical {
		start, end, cross = borderTop, borderBottom, borderLeft|borderRight
	}
	shared := inherited & cross
	if index == 0 {
		shared |= inherited & start
	} else {
		shared |= boundaries[index-1].after
	}
	if index == len(node.children)-1 {
		shared |= inherited & end
	} else {
		shared |= boundaries[index].before
	}
	return shared
}

func assignSharedEdges(node *resolvedNode, inherited borderEdges) {
	node.shared = inherited
	if node.widget != nil {
		return
	}
	for i, child := range node.children {
		assignSharedEdges(child, childSharedEdges(node, i, inherited, node.boundaries))
	}
}

func (m *Model) leafPreferred(leaf *resolvedNode, axis splitAxis, cross int) int {
	if axis != axisVertical {
		return 1
	}
	insets := leaf.edges | leaf.shared
	contentWidth := max(1, max(0, cross)-countEdges(insets, borderLeft|borderRight))
	borderRows := countEdges(insets, borderTop|borderBottom)
	limit := ui.MaxLayoutCells
	if m.height > 0 {
		limit = min(limit, m.height)
	}
	if maximum := nodeMaximum(leaf.node); maximum > 0 {
		limit = min(limit, maximum)
	}
	return borderRows + leaf.widget.MeasureHeight(contentWidth, max(0, limit-borderRows))
}

func (m *Model) preferred(node *resolvedNode, axis splitAxis, cross int) int {
	if node.widget != nil {
		return m.leafPreferred(node, axis, cross)
	}

	direction := nodeAxis(node.node)
	if direction == axis {
		total := boundarySpace(node.boundaries)
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
	// borders, degrade those borders inside the capped rectangle instead of making
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
		insets := node.edges | node.shared
		if axis == axisVertical {
			return minimum.Y + countEdges(insets, borderTop|borderBottom)
		}
		return minimum.X + countEdges(insets, borderLeft|borderRight)
	}

	direction := nodeAxis(node.node)
	if direction == axis {
		total := boundarySpace(node.boundaries)
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

type childAllocation struct {
	sizes       []int
	boundaries  []boundary
	constrained bool
}

func fallbackAllocation(
	children []*resolvedNode,
	extent int,
	axis splitAxis,
	tracks []axisTrack,
) childAllocation {
	count := len(children)
	result := childAllocation{
		constrained: true,
		sizes:       make([]int, count),
		boundaries:  make([]boundary, max(0, count-1)),
	}
	extent = max(0, extent)
	if extent == 0 || count == 0 {
		return result
	}

	// Retry without gaps or ordinary minima, preserving the original sizing
	// rules and hard maxima. Keep the input reachable on either axis.
	relaxed := append([]axisTrack(nil), tracks...)
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

	if sizes, err := allocateAxis(extent, relaxed); err == nil {
		result.sizes = sizes
		return result
	}

	// A valid tree can still derive an auto preference above MaxLayoutCells.
	// Preserve its reserved minima and fill remaining space when the relaxed
	// allocator rejects those derived constraints. Direct invalid callers also
	// stay bounded by the available extent and hard maxima.
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

// Under pressure, preserve content rather than the borders included in normal
// minima. In particular the input can fall back to one editable row.
func interactionMinimum(node *resolvedNode, axis splitAxis) int {
	if node.widget != nil {
		minimum := node.widget.MinimumSize()
		if axis == axisHorizontal {
			return minimum.X
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
	boundaries := node.boundaries
	space := boundarySpace(boundaries)
	effectiveExtent := max(0, extent) - space
	tracks := make([]axisTrack, len(node.children))
	for i, child := range node.children {
		track := axisTrack{
			Size: child.node.Size,
			Min:  m.minimum(child, axis),
			Max:  nodeMaximum(child.node),
		}
		if track.Size.Kind == ui.LayoutSizeAuto {
			track.Auto = m.preferred(child, axis, cross)
		}
		tracks[i] = track
	}
	sizes, err := allocateAxis(effectiveExtent, tracks)
	if err != nil {
		return fallbackAllocation(node.children, extent, axis, tracks)
	}

	used := space
	for i, size := range sizes {
		if size < 0 || (i < len(boundaries) && boundaries[i].overlap && (size == 0 || sizes[i+1] == 0)) {
			return fallbackAllocation(node.children, extent, axis, tracks)
		}
		used += size
		child := node.children[i]
		if child.widget == m.input {
			width, height := cross, size
			start, end := borderTop, borderBottom
			if axis == axisHorizontal {
				width, height, start, end = size, cross, borderLeft, borderRight
			}
			var required borderEdges
			if i > 0 && boundaries[i-1].overlap {
				required |= start
			}
			if i < len(boundaries) && boundaries[i].overlap {
				required |= end
			}
			if required&^m.inputBorders(width, height) != 0 {
				return fallbackAllocation(node.children, extent, axis, tracks)
			}
		}
	}
	if used > extent {
		return fallbackAllocation(node.children, extent, axis, tracks)
	}
	return childAllocation{sizes: sizes, boundaries: boundaries}
}
