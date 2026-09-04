package lua

import (
	"fmt"
	"math"

	"github.com/mmcdole/rune/script"
	"github.com/mmcdole/rune/ui"
)

// registerLayoutFuncs owns layout installation and structural-region state.
// Root-node trees are parsed and validated here, and every downstream consumer
// receives the same canonical LayoutTree.
func (e *Engine) registerLayoutFuncs() {
	e.vm.RegisterModule("rune._ui", map[string]script.GoFunc{
		"layout": func(c *script.Call) error {
			candidate, err := parseLayout(c.Table(1))
			if err != nil {
				return c.Errorf("rune.ui.layout: %v", err)
			}

			// Commit only after parsing and whole-tree validation succeed. A
			// failed replacement leaves both the active tree and the UI alone.
			e.layout = candidate
			e.host.OnPresentationChange()
			return nil
		},
		"region_show": func(c *script.Call) error {
			return e.setRegionVisible(c, "show", true)
		},
		"region_hide": func(c *script.Call) error {
			return e.setRegionVisible(c, "hide", false)
		},
		"region_toggle": func(c *script.Call) error {
			id, err := layoutRegionID(c, "toggle")
			if err != nil {
				return err
			}
			hidden, found := e.layout.RegionHidden(id)
			if !found {
				c.Return(false)
				return nil
			}
			if !hidden && e.refuseHidingInput(c, "toggle", id) {
				return nil
			}
			updated, _, _ := e.layout.WithRegionVisibility(id, hidden)
			e.layout = updated
			c.Return(true)
			e.host.OnPresentationChange()
			return nil
		},
		"region_is_hidden": func(c *script.Call) error {
			id, err := layoutRegionID(c, "is_hidden")
			if err != nil {
				return err
			}
			hidden, found := e.layout.RegionHidden(id)
			if !found {
				c.Return(nil)
				return nil
			}
			c.Return(hidden)
			return nil
		},
	}, nil)
}

func layoutRegionID(c *script.Call, operation string) (string, error) {
	if c.Arg(1).Kind() != script.KindString || c.Arg(1).Str() == "" {
		return "", c.Errorf("rune.ui.regions.%s: id must be a non-empty string", operation)
	}
	return c.Arg(1).Str(), nil
}

func (e *Engine) setPaneVisible(c *script.Call, operation string, visible bool) error {
	name, err := paneName(c, operation)
	if err != nil {
		return err
	}
	updated, found, changed := e.layout.WithPaneVisibility(name, visible)
	c.Return(found)
	if !changed {
		return nil
	}
	e.layout = updated
	e.host.OnPresentationChange()
	return nil
}

func (e *Engine) setRegionVisible(c *script.Call, operation string, visible bool) error {
	id, err := layoutRegionID(c, operation)
	if err != nil {
		return err
	}
	if !visible && e.refuseHidingInput(c, operation, id) {
		return nil
	}
	updated, found, changed := e.layout.WithRegionVisibility(id, visible)
	c.Return(found)
	if !changed {
		return nil
	}
	e.layout = updated
	e.host.OnPresentationChange()
	return nil
}

// GetLayout returns the current canonical layout tree.
func (e *Engine) GetLayout() ui.LayoutTree {
	return e.layout
}

// parseLayout reads a root-node table and validates the complete tree. Any
// failure leaves the caller's active layout untouched.
func parseLayout(tbl script.TableView) (ui.LayoutTree, error) {
	state := layoutNodeParser{active: make(map[uintptr]bool)}
	root, err := state.parseTable(tbl, "root", 0, true)
	if err != nil {
		return ui.LayoutTree{}, err
	}
	tree, err := ui.NormalizeLayoutTree(ui.LayoutTree{Root: root})
	if err != nil {
		return ui.LayoutTree{}, err
	}

	inputCount := countInputNodes(root)
	if inputCount != 1 {
		return ui.LayoutTree{}, fmt.Errorf("layout must contain exactly one input node; got %d", inputCount)
	}
	return tree, nil
}

type layoutNodeParser struct {
	active map[uintptr]bool
	nodes  int
}

func (p *layoutNodeParser) parse(
	value script.Value,
	path string,
	depth int,
	root bool,
) (ui.LayoutNode, error) {
	if value.Kind() != script.KindTable {
		return ui.LayoutNode{}, fmt.Errorf("%s must be a table, got %s", path, value.Kind())
	}
	return p.parseTable(value.Table(), path, depth, root)
}

func (p *layoutNodeParser) parseTable(
	tbl script.TableView,
	path string,
	depth int,
	root bool,
) (ui.LayoutNode, error) {
	if depth > ui.MaxLayoutDepth {
		return ui.LayoutNode{}, fmt.Errorf("%s exceeds the maximum layout depth of %d", path, ui.MaxLayoutDepth)
	}
	p.nodes++
	if p.nodes > ui.MaxLayoutNodes {
		return ui.LayoutNode{}, fmt.Errorf("layout exceeds the maximum of %d nodes", ui.MaxLayoutNodes)
	}

	if p.active[tbl.Id()] {
		return ui.LayoutNode{}, fmt.Errorf("%s contains a cycle", path)
	}
	p.active[tbl.Id()] = true
	defer delete(p.active, tbl.Id())

	typeValue := tbl.Field("type")
	if typeValue.Kind() != script.KindString || typeValue.Str() == "" {
		return ui.LayoutNode{}, fmt.Errorf("%s.type must be a non-empty string, got %s", path, typeValue.Kind())
	}
	node := ui.LayoutNode{Type: typeValue.Str()}
	allowed, ok := layoutFields(node.Type)
	if !ok {
		return ui.LayoutNode{}, fmt.Errorf("%s: unknown type %q", path, node.Type)
	}
	if err := validateLayoutFields(tbl, path, allowed); err != nil {
		return ui.LayoutNode{}, err
	}
	if name := tbl.Field("name"); name.Kind() != script.KindNil {
		if name.Kind() != script.KindString || name.Str() == "" {
			return ui.LayoutNode{}, fmt.Errorf("%s.name must be a non-empty string, got %s", path, name.Kind())
		}
		node.Name = name.Str()
	}

	if id := tbl.Field("id"); id.Kind() != script.KindNil {
		if id.Kind() != script.KindString || id.Str() == "" {
			return ui.LayoutNode{}, fmt.Errorf("%s.id must be a non-empty string, got %s", path, id.Kind())
		}
		node.ID = id.Str()
	}

	if root {
		for _, field := range []string{"id", "size", "min_size", "max_size", "hidden"} {
			if tbl.Field(field).Kind() != script.KindNil {
				return ui.LayoutNode{}, fmt.Errorf("%s.%s is not allowed on the root node", path, field)
			}
		}
	} else {
		var err error
		if node.Size, err = parseNodeSize(tbl.Field("size"), path+".size"); err != nil {
			return ui.LayoutNode{}, err
		}
		if node.MinSize, err = parseOptionalCells(tbl.Field("min_size"), path+".min_size", 0); err != nil {
			return ui.LayoutNode{}, err
		}
		if node.MaxSize, err = parseOptionalCells(tbl.Field("max_size"), path+".max_size", 1); err != nil {
			return ui.LayoutNode{}, err
		}
		if hidden := tbl.Field("hidden"); hidden.Kind() != script.KindNil {
			// A pane placement is addressed by name; a region needs an id so it
			// can be shown again.
			if node.Type != ui.LayoutTypePane && node.ID == "" {
				return ui.LayoutNode{}, fmt.Errorf("%s.hidden requires an id so the region can be shown again", path)
			}
			if hidden.Kind() != script.KindBool {
				return ui.LayoutNode{}, fmt.Errorf("%s.hidden must be a boolean, got %s", path, hidden.Kind())
			}
			node.Hidden = hidden.Bool()
		}
	}

	if node.IsContainer() {
		gap, err := parseOptionalInteger(tbl.Field("gap"), path+".gap", 0, ui.MaxLayoutCells)
		if err != nil {
			return ui.LayoutNode{}, err
		}
		node.Gap = gap
		if dividers := tbl.Field("dividers"); dividers.Kind() != script.KindNil {
			if dividers.Kind() != script.KindBool {
				return ui.LayoutNode{}, fmt.Errorf("%s.dividers must be a boolean, got %s", path, dividers.Kind())
			}
			node.Dividers = dividers.Bool()
		}

		children := tbl.Field("children")
		if children.Kind() != script.KindTable {
			return ui.LayoutNode{}, fmt.Errorf("%s.children must be an array, got %s", path, children.Kind())
		}
		if err := validateLayoutArray(children.Table(), path+".children"); err != nil {
			return ui.LayoutNode{}, err
		}
		node.Children = make([]ui.LayoutNode, 0, children.Table().Len())
		for i := 1; i <= children.Table().Len(); i++ {
			child, err := p.parse(
				children.Table().Index(i),
				fmt.Sprintf("%s.children[%d]", path, i),
				depth+1,
				false,
			)
			if err != nil {
				return ui.LayoutNode{}, err
			}
			node.Children = append(node.Children, child)
		}
		return node, nil
	}

	if err := parseLeafFields(tbl, &node, path); err != nil {
		return ui.LayoutNode{}, err
	}
	return node, nil
}

func layoutFields(typeName string) (map[string]bool, bool) {
	fields := map[string]bool{
		"type": true, "size": true, "min_size": true, "max_size": true,
	}
	switch typeName {
	case ui.LayoutTypeRow, ui.LayoutTypeColumn:
		fields["id"] = true
		fields["children"] = true
		fields["gap"] = true
		fields["dividers"] = true
		fields["hidden"] = true
	case ui.LayoutTypePane:
		fields["name"] = true
		fields["title"] = true
		fields["border"] = true
		fields["hidden"] = true
	case ui.LayoutTypeBar:
		fields["name"] = true
	case ui.LayoutTypeSeparator:
		fields["char"] = true
	case ui.LayoutTypeInput:
	default:
		return nil, false
	}
	return fields, true
}

// parseLeafFields reads leaf presentation fields. It checks Lua types
// only; closed-set values and cell widths are validated once by
// ui.ValidateLayoutTree.
func parseLeafFields(tbl script.TableView, node *ui.LayoutNode, path string) error {
	switch node.Type {
	case ui.LayoutTypePane:
		if title := tbl.Field("title"); title.Kind() != script.KindNil {
			if title.Kind() != script.KindString {
				return fmt.Errorf("%s.title must be a string, got %s", path, title.Kind())
			}
			value := title.Str()
			node.Title = &value
		}
		if border := tbl.Field("border"); border.Kind() != script.KindNil {
			if border.Kind() != script.KindString {
				return fmt.Errorf("%s.border must be a string, got %s", path, border.Kind())
			}
			node.Border = ui.PaneBorder(border.Str())
		}
	case ui.LayoutTypeSeparator:
		if char := tbl.Field("char"); char.Kind() != script.KindNil {
			if char.Kind() != script.KindString {
				return fmt.Errorf("%s.char must be a string, got %s", path, char.Kind())
			}
			node.SeparatorChar = char.Str()
		}
	}
	return nil
}

func parseNodeSize(value script.Value, path string) (ui.LayoutSize, error) {
	switch value.Kind() {
	case script.KindNil:
		return ui.LayoutSize{}, nil
	case script.KindString:
		size, err := ui.ParseLayoutSize(value.Str())
		if err != nil {
			return ui.LayoutSize{}, fmt.Errorf("%s: %w", path, err)
		}
		return size, nil
	case script.KindNumber:
		cells, err := parseIntegerValue(value, path, 1, ui.MaxLayoutCells)
		if err != nil {
			return ui.LayoutSize{}, err
		}
		return ui.Cells(cells), nil
	default:
		return ui.LayoutSize{}, fmt.Errorf("%s must be a cell count or size string, got %s", path, value.Kind())
	}
}

func parseOptionalCells(value script.Value, path string, min int) (*int, error) {
	if value.Kind() == script.KindNil {
		return nil, nil
	}
	cells, err := parseIntegerValue(value, path, min, ui.MaxLayoutCells)
	if err != nil {
		return nil, err
	}
	return &cells, nil
}

func parseOptionalInteger(value script.Value, path string, min, max int) (int, error) {
	if value.Kind() == script.KindNil {
		return 0, nil
	}
	return parseIntegerValue(value, path, min, max)
}

func parseIntegerValue(value script.Value, path string, min, max int) (int, error) {
	if value.Kind() != script.KindNumber {
		return 0, fmt.Errorf("%s must be an integer, got %s", path, value.Kind())
	}
	n := value.Num()
	if math.IsNaN(n) || math.IsInf(n, 0) || math.Trunc(n) != n {
		return 0, fmt.Errorf("%s must be a finite integer", path)
	}
	if n < float64(min) || n > float64(max) {
		return 0, fmt.Errorf("%s must be between %d and %d", path, min, max)
	}
	return int(n), nil
}

func validateLayoutFields(tbl script.TableView, path string, allowed map[string]bool) error {
	var validationErr error
	tbl.Each(func(key, _ script.Value) bool {
		if key.Kind() != script.KindString {
			validationErr = fmt.Errorf("%s field names must be strings, got %s", path, key.Kind())
			return false
		}
		if !allowed[key.Str()] {
			validationErr = fmt.Errorf("%s has unknown field %q", path, key.Str())
			return false
		}
		return true
	})
	return validationErr
}

func validateLayoutArray(tbl script.TableView, path string) error {
	length := tbl.Len()
	count := 0
	var validationErr error
	tbl.Each(func(key, _ script.Value) bool {
		count++
		if key.Kind() != script.KindNumber || math.Trunc(key.Num()) != key.Num() ||
			key.Num() < 1 || key.Num() > float64(length) {
			validationErr = fmt.Errorf("%s must be a dense array", path)
			return false
		}
		return true
	})
	if validationErr != nil {
		return validationErr
	}
	if count != length {
		return fmt.Errorf("%s must be a dense array", path)
	}
	return nil
}

func countInputNodes(root ui.LayoutNode) (input int) {
	var walk func(ui.LayoutNode)
	walk = func(node ui.LayoutNode) {
		if node.Type == ui.LayoutTypeInput {
			input++
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(root)
	return input
}

// refuseHidingInput returns nil, err to the caller when hiding the region
// would remove the input composer from the layout. Unknown regions are left
// to the caller's own not-found handling.
func (e *Engine) refuseHidingInput(c *script.Call, operation, id string) bool {
	containsInput, found := e.layout.RegionContainsInput(id)
	if !found || !containsInput {
		return false
	}
	c.Return(nil, fmt.Sprintf("rune.ui.regions.%s: region %q contains input and cannot be hidden", operation, id))
	return true
}
