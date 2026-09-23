package ui

import (
	"fmt"
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
// resources by name.
const (
	LayoutTypeRow       = "row"
	LayoutTypeColumn    = "column"
	LayoutTypeInput     = "input"
	LayoutTypeSeparator = "separator"
	LayoutTypePane      = "pane"
	LayoutTypeBar       = "bar"

	// OutputPaneName is the pre-created system pane that receives the MUD
	// output, local echo, and prompts.
	OutputPaneName = "output"
)

// PaneBorder is the closed set of pane border modes. The zero value has
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
	// LayoutSizeDefault marks an omitted declaration size. Normalization chooses
	// the axis-aware default; the allocator treats it as Fraction(1).
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
// table shape is normalized before construction.
type LayoutTree struct {
	Root LayoutNode
}

// NormalizeLayoutTree validates and copies a declaration, resolving omitted
// sizes along each parent's axis. Lua and Go callers use the same defaults.
func NormalizeLayoutTree(tree LayoutTree) (LayoutTree, error) {
	if err := ValidateLayoutTree(tree); err != nil {
		return LayoutTree{}, err
	}
	var normalize func(LayoutNode, string) LayoutNode
	normalize = func(node LayoutNode, parent string) LayoutNode {
		if parent != "" && node.Size.Kind == LayoutSizeDefault {
			node.Size = Fraction(1)
			if parent == LayoutTypeColumn {
				switch node.Type {
				case LayoutTypeInput, LayoutTypeBar, LayoutTypeSeparator:
					node.Size = AutoSize()
				}
			}
		}
		if node.MinSize != nil {
			value := *node.MinSize
			node.MinSize = &value
		}
		if node.MaxSize != nil {
			value := *node.MaxSize
			node.MaxSize = &value
		}
		if node.Title != nil {
			value := *node.Title
			node.Title = &value
		}
		if node.Children != nil {
			children := make([]LayoutNode, len(node.Children))
			for i, child := range node.Children {
				children[i] = normalize(child, node.Type)
			}
			node.Children = children
		}
		return node
	}
	return LayoutTree{Root: normalize(tree.Root, "")}, nil
}

// LayoutNode is either a row/column container or a leaf. Pane and bar leaves
// use Name to select a resource. ID identifies a hideable structural region
// and is valid only on non-root containers. Size, MinSize, and MaxSize apply
// along the parent's axis. The root has no parent and cannot carry those
// constraints or visibility state. Gap and Dividers are container-only: Gap
// reserves cells between active children, and Dividers draws a rule between
// them when their borders do not already provide one. Hidden is the
// local hidden state: valid on identified regions and pane
// placements (name is the runtime handle for the latter). Title and Border
// are pane-only; SeparatorChar is separator-only. A non-nil empty Title
// deliberately suppresses title text, while nil requests the pane resource's
// generated title.
type LayoutNode struct {
	Type          string
	Name          string
	ID            string
	Children      []LayoutNode
	Size          LayoutSize
	MinSize       *int
	MaxSize       *int
	Gap           int
	Dividers      bool
	Hidden        bool
	Title         *string
	Border        PaneBorder
	SeparatorChar string
}

// IsContainer reports whether n divides its rectangle among children.
func (n LayoutNode) IsContainer() bool {
	return n.Type == LayoutTypeRow || n.Type == LayoutTypeColumn
}

// DefaultLayoutTree is the recovery layout, usable before Lua core is loaded.
// Core scripts install the normal arrangement, including the status bar.
func DefaultLayoutTree() LayoutTree {
	return LayoutTree{Root: LayoutNode{
		Type: LayoutTypeColumn,
		Children: []LayoutNode{
			{Type: LayoutTypePane, Name: OutputPaneName, Border: PaneBorderNone, Size: Fraction(1)},
			{Type: LayoutTypeInput, Size: AutoSize()},
		},
	}}
}

func regionMatch(id string) func(LayoutNode) bool {
	return func(node LayoutNode) bool { return node.ID == id }
}

func paneMatch(name string) func(LayoutNode) bool {
	return func(node LayoutNode) bool { return node.Type == LayoutTypePane && node.Name == name }
}

// find returns the uniquely identified placement without folding ancestor state.
func (t LayoutTree) find(match func(LayoutNode) bool) (LayoutNode, bool) {
	var walk func(LayoutNode) (LayoutNode, bool)
	walk = func(node LayoutNode) (LayoutNode, bool) {
		if match(node) {
			return node, true
		}
		for _, child := range node.Children {
			if found, ok := walk(child); ok {
				return found, true
			}
		}
		return LayoutNode{}, false
	}
	return walk(t.Root)
}

func (t LayoutTree) hidden(match func(LayoutNode) bool) (hidden, found bool) {
	node, found := t.find(match)
	return node.Hidden, found
}

// withVisibility copies only the path to the matched placement. Published
// trees remain immutable; unchanged or unknown placements need no new snapshot.
func (t LayoutTree) withVisibility(match func(LayoutNode) bool, visible bool) (updated LayoutTree, found, changed bool) {
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
		for i, child := range node.Children {
			updated, found, changed := update(child)
			if !found {
				continue
			}
			if changed {
				node.Children = append([]LayoutNode(nil), node.Children...)
				node.Children[i] = updated
			}
			return node, true, changed
		}
		return node, false, false
	}

	root, found, changed := update(t.Root)
	if changed {
		t.Root = root
	}
	return t, found, changed
}

// RegionHidden reports the local hidden state of a structural region.
func (t LayoutTree) RegionHidden(id string) (hidden, found bool) {
	if id == "" {
		return false, false
	}
	return t.hidden(regionMatch(id))
}

// WithRegionVisibility returns a tree with one region’s hidden state updated.
func (t LayoutTree) WithRegionVisibility(id string, visible bool) (updated LayoutTree, found, changed bool) {
	if id == "" {
		return t, false, false
	}
	return t.withVisibility(regionMatch(id), visible)
}

func nodeHideable(node LayoutNode) bool {
	return (node.IsContainer() && node.ID != "") || node.Type == LayoutTypePane
}

// PaneHidden reports the local hidden state of a named pane.
func (t LayoutTree) PaneHidden(name string) (hidden, found bool) {
	if name == "" {
		return false, false
	}
	return t.hidden(paneMatch(name))
}

// WithPaneVisibility returns a tree with the named pane's hidden state
// updated.
func (t LayoutTree) WithPaneVisibility(name string, visible bool) (updated LayoutTree, found, changed bool) {
	if name == "" {
		return t, false, false
	}
	return t.withVisibility(paneMatch(name), visible)
}

// ValidateLayoutTree validates structural shape and constraints. Widget
// registration and application-level requirements, such as the mandatory input
// input widget, belong to the loader.
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
		LayoutTypeSeparator, LayoutTypePane, LayoutTypeBar:
	default:
		return fmt.Errorf("%s: unknown type %q", path, node.Type)
	}
	switch node.Type {
	case LayoutTypePane, LayoutTypeBar:
		if node.Name == "" {
			return fmt.Errorf("%s: %s requires a non-empty name", path, node.Type)
		}
		key := layoutResourceKey{typeName: node.Type, name: node.Name}
		if previous, exists := v.resources[key]; exists {
			return fmt.Errorf("%s: duplicate %s name %q (already used at %s)", path, node.Type, node.Name, previous)
		}
		v.resources[key] = path
	default:
		if node.Name != "" {
			return fmt.Errorf("%s: name is only valid on pane and bar leaves", path)
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
	if node.Dividers {
		return fmt.Errorf("%s: dividers are only valid on row and column containers", path)
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
	default:
		return fmt.Errorf("%s: unknown leaf type %q", path, node.Type)
	}
	return nil
}

// validateRegionContents keeps the input widget reachable. A region may
// carry an id while it contains input, so ids stay usable as plain handles,
// but it cannot be declared hidden while it does. The region API refuses to
// hide such a region at runtime for the same reason.
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
	if node.ID != "" && node.Hidden && containsInput {
		return false, fmt.Errorf("%s: region %q contains input and cannot be hidden", path, node.ID)
	}
	return containsInput, nil
}

// RegionContainsInput reports whether the identified region holds the input
// input widget at any depth. Such a region exists and may be shown or queried,
// but hiding it would make the input widget unreachable.
func (t LayoutTree) RegionContainsInput(id string) (containsInput, found bool) {
	if id == "" {
		return false, false
	}
	node, found := t.find(regionMatch(id))
	return found && subtreeContainsInput(node), found
}

func subtreeContainsInput(node LayoutNode) bool {
	if node.Type == LayoutTypeInput {
		return true
	}
	for _, child := range node.Children {
		if subtreeContainsInput(child) {
			return true
		}
	}
	return false
}

func validateNodeSize(node LayoutNode, path string) error {
	if err := ValidateLayoutSize(node.Size); err != nil {
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

// ValidateLayoutSize validates a single layout sizing rule.
func ValidateLayoutSize(size LayoutSize) error {
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
