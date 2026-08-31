package ui

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"
)

// Layout limits keep malformed or generated configuration from creating
// unbounded trees or arithmetic inputs.
const (
	MaxLayoutCells = 1 << 14
	MaxLayoutDepth = 64
	MaxLayoutNodes = 4096
)

// Node type names in the canonical layout tree. Pane and bar leaves reference
// resources by name. LegacyReference is emitted only by the v1 dock adapter to
// preserve its bar-first, pane-fallback name resolution.
const (
	LayoutTypeRow             = "row"
	LayoutTypeColumn          = "column"
	LayoutTypeInput           = "input"
	LayoutTypeSeparator       = "separator"
	LayoutTypePane            = "pane"
	LayoutTypeBar             = "bar"
	LayoutTypeLegacyReference = "_legacy_reference"

	// OutputPaneName is the pre-created system pane that receives the MUD
	// transcript, local echo, and prompts.
	OutputPaneName = "output"
)

// PaneBorder is the closed set of pane-frame modes. The zero value has
// PaneBorderFull semantics, matching an omitted border field.
type PaneBorder string

const (
	PaneBorderFull       PaneBorder = "full"
	PaneBorderHorizontal PaneBorder = "horizontal"
	PaneBorderNone       PaneBorder = "none"
)

// LayoutSizeKind identifies how a node is sized along its parent's axis.
type LayoutSizeKind uint8

const (
	// LayoutSizeDefault is the zero value and means Fraction(1).
	LayoutSizeDefault LayoutSizeKind = iota
	LayoutSizeCells
	LayoutSizeFraction
	LayoutSizePercent
	LayoutSizeAuto
)

// LayoutSize is one main-axis sizing rule. Value is cells, fraction weight,
// or an integer percentage according to Kind; it is zero for Default and Auto.
type LayoutSize struct {
	Kind  LayoutSizeKind
	Value int
}

// Cells returns a fixed-cell sizing rule.
func Cells(cells int) LayoutSize { return LayoutSize{Kind: LayoutSizeCells, Value: cells} }

// Fraction returns a weighted share of the space left by non-fractional tracks.
func Fraction(weight int) LayoutSize {
	return LayoutSize{Kind: LayoutSizeFraction, Value: weight}
}

// Percent returns an integer percentage of the parent's extent after gaps.
func Percent(percent int) LayoutSize {
	return LayoutSize{Kind: LayoutSizePercent, Value: percent}
}

// AutoSize returns a rule that uses the widget's measured preferred size.
func AutoSize() LayoutSize { return LayoutSize{Kind: LayoutSizeAuto} }

// ParseLayoutSize parses root-tree size strings: "auto", positive fractional
// units such as "2fr", and integer percentages such as "30%". Fixed cell sizes
// are represented as numbers by the caller and constructed with Cells; numeric
// strings are not accepted.
func ParseLayoutSize(raw string) (LayoutSize, error) {
	if raw == "auto" {
		return AutoSize(), nil
	}
	if raw == "" || strings.TrimSpace(raw) != raw {
		return LayoutSize{}, fmt.Errorf("invalid layout size %q", raw)
	}

	kind := LayoutSizeDefault
	digits := ""
	limit := MaxLayoutCells
	switch {
	case strings.HasSuffix(raw, "fr"):
		kind, digits = LayoutSizeFraction, strings.TrimSuffix(raw, "fr")
	case strings.HasSuffix(raw, "%"):
		kind, digits, limit = LayoutSizePercent, strings.TrimSuffix(raw, "%"), 100
	default:
		return LayoutSize{}, fmt.Errorf("invalid layout size %q: expected auto, Nfr, or P%%", raw)
	}
	if digits == "" {
		return LayoutSize{}, fmt.Errorf("invalid layout size %q", raw)
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return LayoutSize{}, fmt.Errorf("invalid layout size %q: value must be a positive integer", raw)
		}
	}
	value, err := strconv.Atoi(digits)
	if err != nil || value < 1 || value > limit {
		return LayoutSize{}, fmt.Errorf("invalid layout size %q: value must be between 1 and %d", raw, limit)
	}
	return LayoutSize{Kind: kind, Value: value}, nil
}

// LayoutTree is the representation shared across the layout pipeline. Raw Lua
// table shape and version fields are normalized before construction; v1 trees
// may retain private legacy resource references.
type LayoutTree struct {
	Root LayoutNode
}

// LayoutNode is either a row/column container or a leaf. Pane, bar, and private
// legacy-reference leaves use Name to select a resource. ID identifies a hideable
// structural region and is valid only on non-root containers. Size, MinSize,
// and MaxSize apply along the parent's axis. The root has no parent and cannot
// carry those constraints or visibility state. Hidden is the placement's
// visibility gate: valid on identified regions and pane placements (name is
// the runtime handle for the latter). On a legacy reference it gates only the
// pane fallback, never a registered bar. Title and Border are pane-only;
// SeparatorChar is separator-only. A non-nil empty Title deliberately
// suppresses title text, while nil requests the pane resource's generated
// title.
type LayoutNode struct {
	Type          string
	Name          string
	ID            string
	Children      []LayoutNode
	Size          LayoutSize
	MinSize       *int
	MaxSize       *int
	Gap           int
	Hidden        bool
	Title         *string
	Border        PaneBorder
	SeparatorChar string
}

// IsContainer reports whether n divides its rectangle among children.
func (n LayoutNode) IsContainer() bool {
	return n.Type == LayoutTypeRow || n.Type == LayoutTypeColumn
}

// DefaultLayoutTree returns a fresh canonical default layout.
func DefaultLayoutTree() LayoutTree {
	return LayoutTree{Root: LayoutNode{
		Type: LayoutTypeColumn,
		Children: []LayoutNode{
			{Type: LayoutTypePane, Name: OutputPaneName, Border: PaneBorderNone},
			{Type: LayoutTypeInput, Size: AutoSize()},
			{Type: LayoutTypeBar, Name: "status", Size: AutoSize()},
		},
	}}
}

// Every hideable placement carries the same gate: the node's Hidden bit,
// pruned by the resolver, reset by layout installation. Regions and panes
// differ only in how a placement is addressed, so both APIs are matchers over
// one pair of tree walks.

// gateMatch selects the placements one visibility operation addresses.
type gateMatch func(LayoutNode) bool

func regionMatch(id string) gateMatch {
	return func(node LayoutNode) bool { return node.ID == id }
}

func paneMatch(name string) gateMatch {
	return func(node LayoutNode) bool { return paneLeafMatch(node, name) }
}

// gateVisible reports the matched placements' own gate, without ancestor
// gates or the activity of resources below them. The gate counts as visible
// when any matched placement is visible.
func (t LayoutTree) gateVisible(match gateMatch) (visible, found bool) {
	var find func(LayoutNode)
	find = func(node LayoutNode) {
		if match(node) {
			found = true
			visible = visible || !node.Hidden
			return
		}
		for _, child := range node.Children {
			find(child)
		}
	}
	find(t.Root)
	return visible, found
}

// withGateVisibility returns a tree with every matched placement's gate
// updated. Nodes along matching paths are copied so an already-published
// LayoutTree remains an immutable snapshot. Found distinguishes an unknown
// handle; Changed lets callers avoid publishing an idempotent update.
func (t LayoutTree) withGateVisibility(match gateMatch, visible bool) (updated LayoutTree, found, changed bool) {
	var update func(LayoutNode) (LayoutNode, bool, bool)
	update = func(node LayoutNode) (LayoutNode, bool, bool) {
		if match(node) {
			hidden := !visible
			if node.Hidden == hidden {
				return node, true, false
			}
			node.Hidden = hidden
			return node, true, true
		}
		found, changed := false, false
		var copied []LayoutNode
		for i, child := range node.Children {
			updatedChild, childFound, childChanged := update(child)
			found = found || childFound
			if childChanged {
				if copied == nil {
					copied = append([]LayoutNode(nil), node.Children...)
				}
				copied[i] = updatedChild
				changed = true
			}
		}
		if copied != nil {
			node.Children = copied
		}
		return node, found, changed
	}

	root, found, changed := update(t.Root)
	if changed {
		t.Root = root
	}
	return t, found, changed
}

// RegionVisible reports the placement gate for a structural region.
func (t LayoutTree) RegionVisible(id string) (visible, found bool) {
	if id == "" {
		return false, false
	}
	return t.gateVisible(regionMatch(id))
}

// WithRegionVisibility returns a tree with one region gate updated.
func (t LayoutTree) WithRegionVisibility(id string, visible bool) (updated LayoutTree, found, changed bool) {
	if id == "" {
		return t, false, false
	}
	return t.withGateVisibility(regionMatch(id), visible)
}

func nodeHideable(node LayoutNode) bool {
	return (node.IsContainer() && node.ID != "") ||
		node.Type == LayoutTypePane || node.Type == LayoutTypeLegacyReference
}

// paneLeafMatch reports whether node is a placement of the named pane. Legacy
// references share the pane gate: their bar-first resolution is unaffected,
// but the pane fallback honors it.
func paneLeafMatch(node LayoutNode, name string) bool {
	return (node.Type == LayoutTypePane || node.Type == LayoutTypeLegacyReference) &&
		node.Name == name
}

// PaneVisible reports the placement gate for a named pane. V1 legacy trees may
// place one name more than once; the pane counts as visible when any placement
// is.
func (t LayoutTree) PaneVisible(name string) (visible, found bool) {
	if name == "" {
		return false, false
	}
	return t.gateVisible(paneMatch(name))
}

// WithPaneVisibility returns a tree with every placement of the named pane
// updated, so multi-placement legacy trees stay in sync.
func (t LayoutTree) WithPaneVisibility(name string, visible bool) (updated LayoutTree, found, changed bool) {
	if name == "" {
		return t, false, false
	}
	return t.withGateVisibility(paneMatch(name), visible)
}

// AxisTrack is the measured, main-axis input to AllocateAxis. Min is zero
// when omitted, Max is zero when unbounded, and Auto is the measured preferred
// size used only by LayoutSizeAuto.
type AxisTrack struct {
	Size LayoutSize
	Min  int
	Max  int
	Auto int
}

// ErrLayoutTooSmall reports that an axis cannot honor its tracks' minima.
var ErrLayoutTooSmall = errors.New("layout extent is smaller than its minimum sizes")

// ValidateLayoutTree validates structural shape and constraints. Widget
// registration and application-level requirements, such as the mandatory input
// composer, belong to the loader.
func ValidateLayoutTree(tree LayoutTree) error {
	state := layoutValidation{
		ids:       make(map[string]string),
		resources: make(map[layoutResourceKey]string),
	}
	if err := state.node(tree.Root, "root", 0, true, ""); err != nil {
		return err
	}
	_, err := validateRegionContents(tree.Root, "root")
	return err
}

type layoutValidation struct {
	ids       map[string]string
	resources map[layoutResourceKey]string
	nodes     int
}

type layoutResourceKey struct {
	typeName string
	name     string
}

func (v *layoutValidation) node(node LayoutNode, path string, depth int, root bool, parentType string) error {
	v.nodes++
	if v.nodes > MaxLayoutNodes {
		return fmt.Errorf("%s: layout exceeds the limit of %d nodes", path, MaxLayoutNodes)
	}
	if depth > MaxLayoutDepth {
		return fmt.Errorf("%s: layout exceeds the maximum depth of %d", path, MaxLayoutDepth)
	}
	if node.Type == "" {
		return fmt.Errorf("%s: type must be a non-empty string", path)
	}
	switch node.Type {
	case LayoutTypeRow, LayoutTypeColumn, LayoutTypeInput,
		LayoutTypeSeparator, LayoutTypePane, LayoutTypeBar, LayoutTypeLegacyReference:
	default:
		return fmt.Errorf("%s: unknown type %q", path, node.Type)
	}
	switch node.Type {
	case LayoutTypePane, LayoutTypeBar, LayoutTypeLegacyReference:
		if node.Name == "" {
			return fmt.Errorf("%s: %s requires a non-empty name", path, node.Type)
		}
		if node.Type == LayoutTypeLegacyReference &&
			(node.Name == LayoutTypeInput || node.Name == LayoutTypeSeparator) {
			return fmt.Errorf("%s: legacy reference name %q is reserved; use type = %q", path, node.Name, node.Name)
		}
		if node.Type != LayoutTypeLegacyReference {
			key := layoutResourceKey{typeName: node.Type, name: node.Name}
			if previous, exists := v.resources[key]; exists {
				return fmt.Errorf("%s: duplicate %s name %q (already used at %s)", path, node.Type, node.Name, previous)
			}
			v.resources[key] = path
		}
	default:
		if node.Name != "" {
			return fmt.Errorf("%s: name is only valid on pane, bar, and legacy-reference leaves", path)
		}
	}
	if node.ID != "" {
		if root {
			return fmt.Errorf("%s: the root cannot have an id", path)
		}
		if !node.IsContainer() {
			return fmt.Errorf("%s: id is only valid on row and column regions", path)
		}
		if previous, exists := v.ids[node.ID]; exists {
			return fmt.Errorf("%s: duplicate id %q (already used at %s)", path, node.ID, previous)
		}
		v.ids[node.ID] = path
	}

	if root {
		if node.Size != (LayoutSize{}) || node.MinSize != nil || node.MaxSize != nil {
			return fmt.Errorf("%s: the root cannot have size, min_size, or max_size", path)
		}
		if node.Hidden {
			return fmt.Errorf("%s: the root cannot be hidden", path)
		}
	} else if err := validateNodeSize(node, path); err != nil {
		return err
	}
	if node.Hidden && !nodeHideable(node) {
		return fmt.Errorf("%s: hidden is only valid on a pane or an identified row or column region", path)
	}
	if !root && node.Size.Kind == LayoutSizeAuto && parentType == LayoutTypeRow {
		return fmt.Errorf("%s.size: intrinsic width is not supported; use cells, %%, or fr", path)
	}

	if node.IsContainer() {
		if len(node.Children) < 2 {
			return fmt.Errorf("%s: %s needs at least two children", path, node.Type)
		}
		if node.Gap < 0 || node.Gap > MaxLayoutCells {
			return fmt.Errorf("%s: gap must be between 0 and %d cells", path, MaxLayoutCells)
		}
		if node.Title != nil || node.Border != "" || node.SeparatorChar != "" {
			return fmt.Errorf("%s: title, border, and separator char are only valid on leaves", path)
		}
		for i, child := range node.Children {
			childPath := fmt.Sprintf("%s.children[%d]", path, i+1)
			if err := v.node(child, childPath, depth+1, false, node.Type); err != nil {
				return err
			}
		}
		return nil
	}

	if len(node.Children) != 0 {
		return fmt.Errorf("%s: leaf %q cannot have children", path, node.Type)
	}
	if node.Gap != 0 {
		return fmt.Errorf("%s: gap is only valid on row and column containers", path)
	}
	return validateLeafFields(node, path)
}

func validateLeafFields(node LayoutNode, path string) error {
	validBorder := func(border PaneBorder) bool {
		return border == "" || border == PaneBorderFull ||
			border == PaneBorderHorizontal || border == PaneBorderNone
	}

	switch node.Type {
	case LayoutTypePane:
		if !validBorder(node.Border) {
			return fmt.Errorf("%s.border must be %q, %q, or %q", path, PaneBorderFull, PaneBorderHorizontal, PaneBorderNone)
		}
		if node.SeparatorChar != "" {
			return fmt.Errorf("%s: char is only valid on separator leaves", path)
		}
	case LayoutTypeSeparator:
		if node.Title != nil || node.Border != "" {
			return fmt.Errorf("%s: title and border are only valid on pane leaves", path)
		}
		if node.SeparatorChar != "" && runewidth.StringWidth(node.SeparatorChar) != 1 {
			return fmt.Errorf("%s.char must occupy exactly one terminal cell", path)
		}
	case LayoutTypeInput, LayoutTypeBar:
		if node.Title != nil || node.Border != "" || node.SeparatorChar != "" {
			return fmt.Errorf("%s: %s does not accept presentation fields", path, node.Type)
		}
	case LayoutTypeLegacyReference:
		if node.Title != nil || node.SeparatorChar != "" {
			return fmt.Errorf("%s: legacy references do not accept title or separator fields", path)
		}
		if !validBorder(node.Border) {
			return fmt.Errorf("%s.border must be %q, %q, or %q", path, PaneBorderFull, PaneBorderHorizontal, PaneBorderNone)
		}
	default:
		return fmt.Errorf("%s: unknown leaf type %q", path, node.Type)
	}
	return nil
}

// validateRegionContents keeps the input composer reachable. Pane resources,
// including output, may be hidden directly or as part of a region.
func validateRegionContents(node LayoutNode, path string) (containsInput bool, err error) {
	containsInput = node.Type == LayoutTypeInput
	for i, child := range node.Children {
		childPath := fmt.Sprintf("%s.children[%d]", path, i+1)
		childContainsInput, err := validateRegionContents(child, childPath)
		if err != nil {
			return false, err
		}
		containsInput = containsInput || childContainsInput
	}
	if node.ID != "" && containsInput {
		return false, fmt.Errorf("%s: region %q cannot contain input", path, node.ID)
	}
	return containsInput, nil
}

func validateNodeSize(node LayoutNode, path string) error {
	if err := validateLayoutSize(node.Size); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if node.MinSize != nil && (*node.MinSize < 0 || *node.MinSize > MaxLayoutCells) {
		return fmt.Errorf("%s: min_size must be between 0 and %d cells", path, MaxLayoutCells)
	}
	if node.MaxSize != nil && (*node.MaxSize < 1 || *node.MaxSize > MaxLayoutCells) {
		return fmt.Errorf("%s: max_size must be between 1 and %d cells", path, MaxLayoutCells)
	}
	if node.MinSize != nil && node.MaxSize != nil && *node.MinSize > *node.MaxSize {
		return fmt.Errorf("%s: min_size must not exceed max_size", path)
	}
	if node.Size.Kind == LayoutSizeCells {
		if node.MinSize != nil && node.Size.Value < *node.MinSize {
			return fmt.Errorf("%s: fixed size %d is below min_size %d", path, node.Size.Value, *node.MinSize)
		}
		if node.MaxSize != nil && node.Size.Value > *node.MaxSize {
			return fmt.Errorf("%s: fixed size %d exceeds max_size %d", path, node.Size.Value, *node.MaxSize)
		}
	}
	return nil
}

func validateLayoutSize(size LayoutSize) error {
	switch size.Kind {
	case LayoutSizeDefault, LayoutSizeAuto:
		if size.Value != 0 {
			return fmt.Errorf("layout size kind %d does not accept a value", size.Kind)
		}
	case LayoutSizeCells, LayoutSizeFraction:
		if size.Value < 1 || size.Value > MaxLayoutCells {
			return fmt.Errorf("layout size must be between 1 and %d", MaxLayoutCells)
		}
	case LayoutSizePercent:
		if size.Value < 1 || size.Value > 100 {
			return fmt.Errorf("layout percentage must be between 1 and 100")
		}
	default:
		return fmt.Errorf("unknown layout size kind %d", size.Kind)
	}
	return nil
}

// AllocateAxis resolves measured tracks into child sizes for one container
// axis. Extent includes the gaps between children; the returned sizes do not.
// Uncapped fractional tracks consume all remaining cells. If there are no
// such tracks, unused cells are intentionally left at the end of the axis.
//
// Fixed, percentage, and auto sizes are preferred sizes. Fractional tracks
// divide the remainder. If preferred sizes overcommit the axis, tracks shrink
// toward Min in this order: fractional, percentage, then fixed and auto.
// Max caps every track. An impossible minimum returns ErrLayoutTooSmall.
func AllocateAxis(extent, gap int, tracks []AxisTrack) ([]int, error) {
	if extent < 0 {
		return nil, fmt.Errorf("layout extent must not be negative")
	}
	if gap < 0 || gap > MaxLayoutCells {
		return nil, fmt.Errorf("layout gap must be between 0 and %d", MaxLayoutCells)
	}
	if len(tracks) > MaxLayoutNodes {
		return nil, fmt.Errorf("layout axis exceeds the limit of %d tracks", MaxLayoutNodes)
	}
	if len(tracks) == 0 {
		return []int{}, nil
	}

	gapCount := len(tracks) - 1
	if gapCount > 0 && gap > extent/gapCount {
		return nil, fmt.Errorf("%w: gaps need %d cells, have %d", ErrLayoutTooSmall, gap*gapCount, extent)
	}
	available := extent - gap*gapCount

	normalized := make([]AxisTrack, len(tracks))
	minimum := 0
	for i, track := range tracks {
		if track.Size.Kind == LayoutSizeDefault {
			track.Size = Fraction(1)
		}
		if err := validateLayoutSize(track.Size); err != nil {
			return nil, fmt.Errorf("track %d: %w", i+1, err)
		}
		if track.Min < 0 || track.Min > MaxLayoutCells {
			return nil, fmt.Errorf("track %d: min must be between 0 and %d", i+1, MaxLayoutCells)
		}
		if track.Max < 0 || track.Max > MaxLayoutCells {
			return nil, fmt.Errorf("track %d: max must be between 0 and %d", i+1, MaxLayoutCells)
		}
		if track.Max > 0 && track.Min > track.Max {
			return nil, fmt.Errorf("track %d: min must not exceed max", i+1)
		}
		if track.Size.Kind == LayoutSizeAuto && (track.Auto < 0 || track.Auto > MaxLayoutCells) {
			return nil, fmt.Errorf("track %d: auto size must be between 0 and %d", i+1, MaxLayoutCells)
		}
		normalized[i] = track
		minimum += track.Min
	}
	if minimum > available {
		return nil, fmt.Errorf("%w: tracks need at least %d cells plus %d gaps, have %d",
			ErrLayoutTooSmall, minimum, gap*gapCount, extent)
	}

	sizes := make([]int, len(normalized))
	flexIndices := make([]int, 0, len(normalized))
	flexWeights := make([]int, 0, len(normalized))
	nonFlexTotal := 0
	for i, track := range normalized {
		var target int
		switch track.Size.Kind {
		case LayoutSizeCells:
			target = track.Size.Value
		case LayoutSizePercent:
			target = roundedPercent(available, track.Size.Value)
		case LayoutSizeAuto:
			target = track.Auto
		case LayoutSizeFraction:
			flexIndices = append(flexIndices, i)
			flexWeights = append(flexWeights, track.Size.Value)
			continue
		}
		sizes[i] = clampTrack(target, track)
		nonFlexTotal += sizes[i]
	}

	if remainder := available - nonFlexTotal; remainder > 0 && len(flexIndices) > 0 {
		shares := proportional(remainder, flexWeights)
		for j, index := range flexIndices {
			sizes[index] = clampTrack(shares[j], normalized[index])
		}
	} else {
		for _, index := range flexIndices {
			sizes[index] = normalized[index].Min
		}
	}

	total := sumInts(sizes)
	if total > available {
		over := total - available
		over = shrinkTracks(sizes, normalized, over, LayoutSizeFraction)
		over = shrinkTracks(sizes, normalized, over, LayoutSizePercent)
		over = shrinkTracks(sizes, normalized, over, LayoutSizeCells, LayoutSizeAuto)
		if over > 0 {
			return nil, fmt.Errorf("%w: tracks need %d cells plus %d gaps, have %d",
				ErrLayoutTooSmall, available+over, gap*gapCount, extent)
		}
	}

	if remaining := available - sumInts(sizes); remaining > 0 {
		growFractions(sizes, normalized, remaining)
	}
	return sizes, nil
}

func roundedPercent(total, percent int) int {
	whole := (total / 100) * percent
	remainder := (total % 100) * percent
	return whole + (remainder+50)/100
}

func clampTrack(size int, track AxisTrack) int {
	size = max(size, track.Min)
	if track.Max > 0 {
		size = min(size, track.Max)
	}
	return size
}

// proportional divides total by positive weights using largest-remainder
// rounding. Equal remainders favor the earlier track.
func proportional(total int, weights []int) []int {
	shares := make([]int, len(weights))
	if total <= 0 || len(weights) == 0 {
		return shares
	}
	weightTotal := sumInts(weights)
	type residue struct {
		index int
		value int64
	}
	residues := make([]residue, len(weights))
	allocated := 0
	whole := total / weightTotal
	remainder := total % weightTotal
	for i, weight := range weights {
		product := int64(remainder) * int64(weight)
		shares[i] = whole*weight + int(product/int64(weightTotal))
		allocated += shares[i]
		residues[i] = residue{index: i, value: product % int64(weightTotal)}
	}
	sort.SliceStable(residues, func(i, j int) bool {
		return residues[i].value > residues[j].value
	})
	for i := 0; i < total-allocated; i++ {
		shares[residues[i].index]++
	}
	return shares
}

func shrinkTracks(sizes []int, tracks []AxisTrack, amount int, kinds ...LayoutSizeKind) int {
	if amount <= 0 {
		return 0
	}
	wanted := make(map[LayoutSizeKind]bool, len(kinds))
	for _, kind := range kinds {
		wanted[kind] = true
	}
	indices := make([]int, 0, len(tracks))
	capacities := make([]int, 0, len(tracks))
	totalCapacity := 0
	for i, track := range tracks {
		if !wanted[track.Size.Kind] || sizes[i] <= track.Min {
			continue
		}
		capacity := sizes[i] - track.Min
		indices = append(indices, i)
		capacities = append(capacities, capacity)
		totalCapacity += capacity
	}
	if totalCapacity == 0 {
		return amount
	}
	take := min(amount, totalCapacity)
	reductions := proportional(take, capacities)
	for i, index := range indices {
		sizes[index] -= reductions[i]
	}
	return amount - take
}

func growFractions(sizes []int, tracks []AxisTrack, amount int) int {
	for amount > 0 {
		indices := make([]int, 0, len(tracks))
		weights := make([]int, 0, len(tracks))
		for i, track := range tracks {
			if track.Size.Kind != LayoutSizeFraction || (track.Max > 0 && sizes[i] >= track.Max) {
				continue
			}
			indices = append(indices, i)
			weights = append(weights, track.Size.Value)
		}
		if len(indices) == 0 {
			return amount
		}

		grants := proportional(amount, weights)
		consumed := 0
		for i, index := range indices {
			grant := grants[i]
			if maxSize := tracks[index].Max; maxSize > 0 {
				grant = min(grant, maxSize-sizes[index])
			}
			sizes[index] += grant
			consumed += grant
		}
		if consumed == 0 {
			return amount
		}
		amount -= consumed
	}
	return 0
}

func sumInts(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}
