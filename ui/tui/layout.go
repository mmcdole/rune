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

const defaultPaneContentHeight = 10

type splitAxis uint8

const (
	axisHorizontal splitAxis = iota
	axisVertical
)

// layoutPlan is the complete geometry for one frame. The same plan sizes the
// output viewport for interaction and places every leaf for rendering.
type layoutPlan struct {
	leaves   []placedLeaf
	dividers []dividerRule
	frame    frameGrid
	output   image.Rectangle
}

// dividerRule is one container-owned rule drawn between two adjacent active
// children. A row draws vertical rules; a column draws horizontal rules. The
// resolver prunes inactive children first, so a rule never borders a hidden or
// empty sibling.
type dividerRule struct {
	vertical bool
	at       int
	from, to int
}

type leafKind uint8

const (
	leafWidget leafKind = iota
	leafPane
)

// placedLeaf snapshots identity and geometry, not rendering. parentAxis is
// the axis of the container that placed the leaf; the root leaf reports
// vertical.
type placedLeaf struct {
	node       ui.LayoutNode
	kind       leafKind
	widget     widget.Widget
	pane       paneResource
	outer      image.Rectangle
	content    image.Rectangle
	frames     frameEdges
	parentAxis splitAxis
}

type resolvedNode struct {
	node      ui.LayoutNode
	leaf      *placedLeaf
	children  []*resolvedNode
	frames    frameEdges
	hasOutput bool
	hasInput  bool
}

type frameEdges uint8

const (
	frameLeft frameEdges = 1 << iota
	frameRight
	frameTop
	frameBottom
	frameAll = frameLeft | frameRight | frameTop | frameBottom
)

func (n *resolvedNode) framesEdge(edge frameEdges) bool {
	return n != nil && n.frames&edge != 0
}

func seamFrames(children []*resolvedNode, index int, axis splitAxis) (before, after bool) {
	if axis == axisHorizontal {
		return children[index].framesEdge(frameRight), children[index+1].framesEdge(frameLeft)
	}
	return children[index].framesEdge(frameBottom), children[index+1].framesEdge(frameTop)
}

// boundaryGaps returns the space between each pair of active children. With
// no declared gap, a divider reuses an existing framed seam; otherwise it
// reserves one cell for a rule.
func boundaryGaps(node ui.LayoutNode, children []*resolvedNode, axis splitAxis) []int {
	gaps := make([]int, max(0, len(children)-1))
	for i := range gaps {
		gaps[i] = node.Gap
		before, after := seamFrames(children, i, axis)
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

func containerFrameEdges(node ui.LayoutNode, children []*resolvedNode) frameEdges {
	if len(children) == 0 {
		return 0
	}
	gaps := boundaryGaps(node, children, nodeAxis(node))
	all := func(edge frameEdges) bool {
		for _, gap := range gaps {
			// A one-cell divider connects the children's cross-axis frames.
			// Wider gaps and undrawn gaps break the composite perimeter.
			if gap > 0 && (!node.Dividers || gap > 1) {
				return false
			}
		}
		for _, child := range children {
			if !child.framesEdge(edge) {
				return false
			}
		}
		return true
	}
	fills := false
	for _, child := range children {
		kind := child.node.Size.Kind
		if (kind == ui.LayoutSizeDefault || kind == ui.LayoutSizeFraction) &&
			child.node.MaxSize == nil {
			fills = true
			break
		}
	}

	var edges frameEdges
	if node.Type == ui.LayoutTypeRow {
		if children[0].framesEdge(frameLeft) {
			edges |= frameLeft
		}
		if fills && children[len(children)-1].framesEdge(frameRight) {
			edges |= frameRight
		}
		if fills && all(frameTop) {
			edges |= frameTop
		}
		if fills && all(frameBottom) {
			edges |= frameBottom
		}
		return edges
	}
	if children[0].framesEdge(frameTop) {
		edges |= frameTop
	}
	if fills && children[len(children)-1].framesEdge(frameBottom) {
		edges |= frameBottom
	}
	if fills && all(frameLeft) {
		edges |= frameLeft
	}
	if fills && all(frameRight) {
		edges |= frameRight
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

func (m *Model) resolveWidget(node ui.LayoutNode, w widget.Widget, availableWidth int) (*resolvedNode, bool) {
	// Empty bars collapse even when assigned a fixed track. Width-sensitive
	// widgets receive their current allowance before visibility is measured.
	w.SetSize(max(1, availableWidth), 0)
	if w.PreferredHeight() <= 0 {
		return nil, false
	}
	leaf := &placedLeaf{node: node, kind: leafWidget, widget: w}
	return &resolvedNode{
		node: node, leaf: leaf,
		hasInput: w == m.input,
	}, true
}

func (m *Model) resolveBar(node ui.LayoutNode, name string, availableWidth int) (*resolvedNode, bool) {
	bar, ok := m.bars[name]
	if !ok {
		return nil, false
	}
	return m.resolveWidget(node, bar, availableWidth)
}

// resolvePane places a named pane buffer, creating it on first placement so a
// declared pane renders as an empty titled box instead of silently vanishing.
func (m *Model) resolvePane(node ui.LayoutNode, name string) (*resolvedNode, bool) {
	pane := m.panes.Create(name)
	leaf := &placedLeaf{
		node: node, kind: leafPane, pane: pane,
		frames: paneFrames(node.Border),
	}
	return &resolvedNode{
		node: node, leaf: leaf, frames: leaf.frames,
		hasOutput: pane == m.output,
	}, true
}

// resolveNode prunes hidden placements and leaves that cannot currently
// render. Pane and bar leaves each select their own resource namespace.
func (m *Model) resolveNode(node ui.LayoutNode, availableWidth int) (*resolvedNode, bool) {
	if node.Hidden {
		return nil, false
	}
	if node.IsContainer() {
		children := make([]*resolvedNode, 0, len(node.Children))
		for _, child := range node.Children {
			if resolved, ok := m.resolveNode(child, availableWidth); ok {
				children = append(children, resolved)
			}
		}
		if len(children) == 0 {
			return nil, false
		}
		resolved := &resolvedNode{node: node, children: children}
		for _, child := range children {
			resolved.hasOutput = resolved.hasOutput || child.hasOutput
			resolved.hasInput = resolved.hasInput || child.hasInput
		}
		resolved.frames = containerFrameEdges(node, children)
		return resolved, true
	}

	switch node.Type {
	case ui.LayoutTypeInput:
		return m.resolveWidget(node, m.input, availableWidth)
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

func (m *Model) leafPreferred(leaf *placedLeaf, axis splitAxis, cross int) int {
	switch leaf.kind {
	case leafPane:
		if axis == axisVertical {
			height := defaultPaneContentHeight
			if leaf.frames&frameTop != 0 {
				height++
			}
			if leaf.frames&frameBottom != 0 {
				height++
			}
			return height
		}
		// Validation rejects intrinsic widths, so this is reached only by
		// direct Go callers; keep the measurement deterministic.
		return 1
	case leafWidget:
		if axis == axisVertical {
			leaf.widget.SetSize(max(1, cross), 0)
			return leaf.widget.PreferredHeight()
		}
		// Non-pane widgets expose intrinsic height but no intrinsic width, so
		// horizontal measurement uses a stable one-cell minimum.
		return 1
	}
	return 1
}

func (m *Model) preferred(node *resolvedNode, axis splitAxis, cross int) int {
	if node.leaf != nil {
		return m.leafPreferred(node.leaf, axis, cross)
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
	if node.leaf != nil {
		if axis == axisVertical && (node.hasOutput || node.hasInput) {
			return 1
		}
		if node.leaf.kind == leafPane {
			minimum := 0
			if axis == axisVertical {
				if node.leaf.frames&frameTop != 0 {
					minimum++
				}
				if node.leaf.frames&frameBottom != 0 {
					minimum++
				}
			} else {
				if node.leaf.frames&frameLeft != 0 {
					minimum++
				}
				if node.leaf.frames&frameRight != 0 {
					minimum++
				}
			}
			return minimum
		}
		return 0
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
			before, after := seamFrames(children, i, axis)
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
	sizes    []int
	gaps     []int
	overlaps []bool
}

func fallbackAllocation(
	children []*resolvedNode,
	extent int,
	axis splitAxis,
	tracks []ui.AxisTrack,
) childAllocation {
	count := len(children)
	result := childAllocation{
		sizes:    make([]int, count),
		gaps:     make([]int, max(0, count-1)),
		overlaps: make([]bool, max(0, count-1)),
	}
	extent = max(0, extent)
	if extent == 0 || count == 0 {
		return result
	}

	// Retry without gaps or ordinary minima, preserving the original sizing
	// rules and hard maxima. On a vertical split, reserve the scarce rows for
	// input first and output second so the user retains a recovery surface.
	relaxed := append([]ui.AxisTrack(nil), tracks...)
	for i := range relaxed {
		relaxed[i].Min = 0
	}
	remainingProtected := extent
	if axis == axisVertical {
		for _, protected := range []func(*resolvedNode) bool{
			func(child *resolvedNode) bool { return child.hasInput },
			func(child *resolvedNode) bool { return child.hasOutput },
		} {
			for i, child := range children {
				if remainingProtected == 0 {
					break
				}
				if protected(child) &&
					(relaxed[i].Max == 0 || relaxed[i].Min < relaxed[i].Max) {
					relaxed[i].Min++
					remainingProtected--
				}
			}
		}
	}

	if sizes, err := ui.AllocateAxis(extent, 0, relaxed); err == nil {
		result.sizes = sizes
		return result
	}

	// Validated trees cannot reach this branch; it keeps direct, invalid Go
	// callers bounded and preserves any protected rows without risking a loop.
	remaining := extent
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
	}
	if used > extent {
		return fallbackAllocation(node.children, extent, axis, tracks)
	}
	return childAllocation{sizes: sizes, gaps: gaps, overlaps: overlaps}
}

func (m *Model) placeNode(node *resolvedNode, rect image.Rectangle, parentAxis splitAxis, plan *layoutPlan) {
	if rect.Empty() {
		return
	}
	if node.leaf != nil {
		leaf := *node.leaf
		leaf.outer, leaf.content = rect, rect
		leaf.parentAxis = parentAxis
		plan.leaves = append(plan.leaves, leaf)
		return
	}

	axis := nodeAxis(node.node)
	allocation := m.allocateChildren(node, axisExtent(rect, axis), axis, crossExtent(rect, axis))

	position := rect.Min.X
	if axis == axisVertical {
		position = rect.Min.Y
	}
	for i, child := range node.children {
		size := allocation.sizes[i]
		childArea := childRect(rect, axis, position, size).Intersect(rect)
		m.placeNode(child, childArea, axis, plan)
		position += size
		if i < len(node.children)-1 {
			gap := allocation.gaps[i]
			// The tiny-terminal fallback drops gaps, and with them the cell a
			// divider draws in, so dividers degrade away with their gap.
			if node.node.Dividers && gap >= 1 {
				rule := dividerRule{vertical: axis == axisHorizontal,
					at: position + (gap-1)/2}
				if rule.vertical {
					rule.from, rule.to = rect.Min.Y, rect.Max.Y
				} else {
					rule.from, rule.to = rect.Min.X, rect.Max.X
				}
				plan.dividers = append(plan.dividers, rule)
			}
			position += gap
			if allocation.overlaps[i] {
				position--
			}
		}
	}
}

func (m *Model) resolveLayout() layoutPlan {
	plan := layoutPlan{}
	if m.width <= 0 || m.height <= 0 {
		return plan
	}
	root, ok := m.resolveNode(m.layout.Root, m.width)
	if !ok {
		return plan
	}
	m.placeNode(root, image.Rect(0, 0, m.width, m.height), axisVertical, &plan)
	needsFrame := len(plan.dividers) > 0
	for i := range plan.leaves {
		if needsFrame || paneLeaf(&plan.leaves[i]) || separatorLeaf(&plan.leaves[i]) {
			needsFrame = true
			break
		}
	}
	if needsFrame {
		plan.frame = newFrameGrid(m.width, m.height)
		m.planFrames(&plan)
	}
	for _, leaf := range plan.leaves {
		if leaf.kind == leafPane && leaf.pane == m.output {
			plan.output = leaf.content
			break
		}
	}
	return plan
}

func (m *Model) invalidateLayout() {
	m.layoutPlanValid = false
}

func (m *Model) ensureLayout() layoutPlan {
	if !m.layoutPlanValid {
		m.layoutPlan = m.resolveLayout()
		m.layoutPlanValid = true
	}
	return m.layoutPlan
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

// finalizeLayoutPlan applies output geometry before returning the plan. A
// height change can clamp a scrolled viewport back to live mode; rebuilding in
// that case refreshes generated pane-title state captured by planFrames.
func (m *Model) finalizeLayoutPlan() (layoutPlan, bool) {
	plan := m.ensureLayout()
	changed := m.applyOutputGeometry(plan)
	if changed {
		m.invalidateLayout()
		plan = m.ensureLayout()
		m.applyOutputGeometry(plan)
	}
	return plan, changed
}

// syncViewportSize makes geometry current outside View. Search preview,
// scrolling, and append-time wrapping therefore observe the same output rect as
// the next rendered frame.
func (m *Model) syncViewportSize() bool {
	if !m.initialized {
		return false
	}
	_, changed := m.finalizeLayoutPlan()
	return changed
}

// frameGrid records line connectivity independently from content rendering.
// Titles are painted after the grid, so a shared lower pane header naturally
// owns a horizontal pane boundary.
type frameGrid struct {
	width, height int
	horizontal    []bool
	vertical      []bool
	titles        []frameTitle
}

type frameTitle struct {
	point image.Point
	width int
	text  string
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

func paneLeaf(leaf *placedLeaf) bool {
	return leaf.kind == leafPane && leaf.frames != 0
}

// separatorLeaf reports a default-character separator placed by a column. It
// draws through the frame grid so it joins dividers and pane borders. A custom
// character, or a separator placed by a row, keeps the widget rendering.
func separatorLeaf(leaf *placedLeaf) bool {
	return leaf.kind == leafWidget && leaf.node.Type == ui.LayoutTypeSeparator &&
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
	for _, rule := range plan.dividers {
		if rule.vertical {
			plan.frame.markVertical(rule.at, rule.from, rule.to)
		} else {
			plan.frame.markHorizontal(rule.at, rule.from, rule.to)
		}
	}
	for i := range plan.leaves {
		leaf := &plan.leaves[i]
		if separatorLeaf(leaf) {
			plan.frame.markHorizontal(leaf.outer.Min.Y, leaf.outer.Min.X, leaf.outer.Max.X)
			leaf.content = image.Rectangle{}
			continue
		}
		if !paneLeaf(leaf) {
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
		leaf.content = insetFrame(outer, leaf.frames)

		if leaf.frames&frameTop != 0 {
			title := leaf.pane.Title()
			if leaf.node.Title != nil {
				title = *leaf.node.Title
			}
			if title == "" {
				continue
			}
			title = " " + runetext.VisualizeTerminalControls(title, false) + " "
			left := outer.Min.X
			width := outer.Dx()
			if leaf.frames&frameLeft != 0 {
				left++
				width--
			}
			if leaf.frames&frameRight != 0 {
				width--
			}
			width = max(0, width)
			title = ansi.Truncate(title, width, "")
			plan.frame.titles = append(plan.frame.titles, frameTitle{
				point: image.Pt(left, outer.Min.Y), width: lipgloss.Width(title), text: title,
			})
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

func (m *Model) renderPlan(plan layoutPlan) string {
	canvas := lipgloss.NewCanvas(m.width, m.height)
	for i := range plan.leaves {
		leaf := &plan.leaves[i]
		if leaf.content.Empty() {
			continue
		}
		var view string
		switch leaf.kind {
		case leafWidget:
			leaf.widget.SetSize(leaf.content.Dx(), leaf.content.Dy())
			view = leaf.widget.View()
		case leafPane:
			view = leaf.pane.View(leaf.content.Dx(), leaf.content.Dy())
		}
		drawStyled(canvas, view, leaf.content)
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
	for _, title := range plan.frame.titles {
		if title.width <= 0 {
			continue
		}
		rect := image.Rect(title.point.X, title.point.Y, title.point.X+title.width, title.point.Y+1)
		drawStyled(canvas, m.styles.PaneHeader.Render(title.text), rect)
	}
	return exactCanvasRender(canvas, m.width, m.height)
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

	plan, _ := m.finalizeLayoutPlan()
	view.Content = m.renderPlan(plan)
	return view
}
