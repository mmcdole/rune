package lua

import (
	"fmt"
	"math"

	"github.com/mattn/go-runewidth"
	"github.com/mmcdole/rune/script"
	"github.com/mmcdole/rune/ui"
)

// parseLegacyLayout converts accepted v1 dock fields to a canonical LayoutTree.
// It preserves permissive entry parsing and marks late-bound resource references
// explicitly.
func parseLegacyLayout(tbl script.TableView) (ui.LayoutTree, error) {
	top, topHasParsedEntry := parseLegacyDock(tbl.Field("top"))
	bottom, bottomHasParsedEntry := parseLegacyDock(tbl.Field("bottom"))
	// A v1 table whose docks contain no parseable entries selects the default
	// layout.
	if !topHasParsedEntry && !bottomHasParsedEntry {
		return ui.DefaultLayoutTree(), nil
	}
	// V1 names resolve bars before panes. Because the canonical tree reserves
	// "output" for the transcript pane, the first legacy "output" reference
	// becomes an explicit bar and later occurrences are omitted to keep resource
	// placement unique.
	outputBarSeen := false
	top = lowerLegacyOutputReferences(top, &outputBarSeen)
	bottom = lowerLegacyOutputReferences(bottom, &outputBarSeen)

	children := make([]ui.LayoutNode, 0, len(top)+1+len(bottom))
	children = append(children, top...)
	children = append(children, ui.LayoutNode{
		Type: ui.LayoutTypePane, Name: ui.OutputPaneName,
		Border: ui.PaneBorderNone,
	})
	children = append(children, bottom...)

	root := children[0]
	if len(children) > 1 {
		root = ui.LayoutNode{Type: ui.LayoutTypeColumn, Children: children}
	}
	tree := ui.LayoutTree{Root: root}
	if err := ui.ValidateLayoutTree(tree); err != nil {
		return ui.LayoutTree{}, fmt.Errorf("legacy layout: %w", err)
	}
	return tree, nil
}

func parseLegacyDock(value script.Value) (nodes []ui.LayoutNode, hasParsedEntry bool) {
	if value.Kind() != script.KindTable {
		return nil, false
	}
	tbl := value.Table()
	nodes = make([]ui.LayoutNode, 0, tbl.Len())
	for i := 1; i <= tbl.Len(); i++ {
		if node, ok := parseLegacyEntry(tbl.Index(i)); ok {
			hasParsedEntry = true
			nodes = append(nodes, node)
		}
	}
	return nodes, hasParsedEntry
}

func lowerLegacyOutputReferences(nodes []ui.LayoutNode, seen *bool) []ui.LayoutNode {
	normalized := nodes[:0]
	for _, node := range nodes {
		if node.Type == ui.LayoutTypeLegacyReference && node.Name == ui.OutputPaneName {
			if *seen {
				continue
			}
			*seen = true
			node.Type = ui.LayoutTypeBar
			node.Border = ""
			node.Hidden = false
		}
		normalized = append(normalized, node)
	}
	return normalized
}

func parseLegacyEntry(value script.Value) (ui.LayoutNode, bool) {
	switch value.Kind() {
	case script.KindString:
		if value.Str() == "" {
			return ui.LayoutNode{}, false
		}
		return newLegacyLayoutNode(value.Str()), true
	case script.KindTable:
	default:
		return ui.LayoutNode{}, false
	}

	tbl := value.Table()
	name := tbl.Field("name").Str()
	if name == "" {
		return ui.LayoutNode{}, false
	}
	node := newLegacyLayoutNode(name)

	// V1 finite numeric heights are truncated toward zero; canonical validation
	// then enforces the layout cell bounds.
	if height := tbl.Field("height"); height.Kind() == script.KindNumber {
		n := height.Num()
		if !math.IsNaN(n) && !math.IsInf(n, 0) {
			if cells := int(n); cells != 0 {
				node.Size = ui.Cells(cells)
			}
		}
	}

	// V1 preserves a valid one-cell separator char and ignores other options.
	tbl.Each(func(key, option script.Value) bool {
		if key.Kind() != script.KindString || option.Kind() != script.KindString {
			return true
		}
		if node.Type == ui.LayoutTypeSeparator && key.Str() == "char" &&
			runewidth.StringWidth(option.Str()) == 1 {
			node.SeparatorChar = option.Str()
		}
		return true
	})
	return node, true
}

func newLegacyLayoutNode(name string) ui.LayoutNode {
	// V1 pane references use horizontal rules without side walls. A user-supplied
	// v1 border option has no semantics and is ignored.
	node := ui.LayoutNode{Size: ui.AutoSize()}
	// V1 resolves input and separator structurally; every other name remains a
	// late-bound resource reference. V1 pane placements start hidden until a
	// script shows them; the gate never affects a registered bar.
	switch name {
	case ui.LayoutTypeInput, ui.LayoutTypeSeparator:
		node.Type = name
	default:
		node.Type = ui.LayoutTypeLegacyReference
		node.Name = name
		node.Border = ui.PaneBorderHorizontal
		node.Hidden = true
	}
	return node
}
