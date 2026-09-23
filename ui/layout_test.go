package ui

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func testBar(name string) LayoutNode {
	return LayoutNode{Type: LayoutTypeBar, Name: name}
}

func TestNormalizeLayoutUsesParentAxisAndCopiesDeclaration(t *testing.T) {
	for _, axis := range []string{LayoutTypeRow, LayoutTypeColumn} {
		minimum, maximum, title := 0, 20, "Chat"
		tree := LayoutTree{Root: LayoutNode{Type: axis, Children: []LayoutNode{
			{Type: LayoutTypeInput},
			{Type: LayoutTypeBar, Name: "status"},
			{Type: LayoutTypeSeparator},
			{Type: LayoutTypePane, Name: "chat", MinSize: &minimum, MaxSize: &maximum, Title: &title},
			{Type: LayoutTypeColumn, ID: "empty", Children: []LayoutNode{}},
			{Type: LayoutTypeRow, ID: "single", Children: []LayoutNode{{Type: LayoutTypePane, Name: "map"}}},
		}}}
		normalized, err := NormalizeLayoutTree(tree)
		if err != nil {
			t.Fatal(err)
		}
		for i, child := range normalized.Root.Children {
			want := Fraction(1)
			if axis == LayoutTypeColumn && i < 3 {
				want = AutoSize()
			}
			if child.Size != want {
				t.Errorf("%s child %d size = %v, want %v", axis, i, child.Size, want)
			}
		}
		if tree.Root.Children[0].Size.Kind != LayoutSizeDefault {
			t.Fatal("normalization changed the declaration")
		}
		minimum, maximum, title = 10, 30, "Changed"
		tree.Root.Children[5].Children[0].Name = "changed"
		pane := normalized.Root.Children[3]
		if *pane.MinSize != 0 || *pane.MaxSize != 20 || *pane.Title != "Chat" ||
			normalized.Root.Children[5].Children[0].Name != "map" {
			t.Fatal("normalized tree aliases mutable declaration fields")
		}
		again, err := NormalizeLayoutTree(normalized)
		if err != nil || !reflect.DeepEqual(again, normalized) {
			t.Fatal("normalization is not idempotent")
		}
	}
}

func TestParseLayoutSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw  string
		want LayoutSize
	}{
		{raw: "auto", want: AutoSize()},
		{raw: "1fr", want: Fraction(1)},
		{raw: "12fr", want: Fraction(12)},
		{raw: "1%", want: Percent(1)},
		{raw: "30%", want: Percent(30)},
		{raw: "100%", want: Percent(100)},
	}
	for _, test := range tests {
		t.Run(test.raw, func(t *testing.T) {
			got, err := ParseLayoutSize(test.raw)
			if err != nil {
				t.Fatalf("ParseLayoutSize(%q): %v", test.raw, err)
			}
			if got != test.want {
				t.Fatalf("ParseLayoutSize(%q) = %+v, want %+v", test.raw, got, test.want)
			}
		})
	}
}

func TestParseLayoutSizeRejectsNonCanonicalForms(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"", "40", " auto", "auto ", "Auto", "fr", "0fr", "1.5fr",
		"%", "0%", "101%", "30.5%", "-1fr", "+1fr", "1FR",
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := ParseLayoutSize(raw); err == nil {
				t.Fatalf("ParseLayoutSize(%q) unexpectedly succeeded", raw)
			}
		})
	}
}

func TestDefaultLayoutTreeIsCanonical(t *testing.T) {
	t.Parallel()

	tree := DefaultLayoutTree()
	if err := ValidateLayoutTree(tree); err != nil {
		t.Fatalf("ValidateLayoutTree(DefaultLayoutTree()): %v", err)
	}
	want := LayoutTree{Root: LayoutNode{
		Type: LayoutTypeColumn,
		Children: []LayoutNode{
			{Type: LayoutTypePane, Name: OutputPaneName, Border: PaneBorderNone, Size: Fraction(1)},
			{Type: LayoutTypeInput, Size: AutoSize()},
		},
	}}
	if !reflect.DeepEqual(tree, want) {
		t.Fatalf("DefaultLayoutTree() = %#v, want %#v", tree, want)
	}
}

func TestValidateLayoutTreeAcceptsTypedResourcesAndOutputRegion(t *testing.T) {
	t.Parallel()

	tree := LayoutTree{Root: LayoutNode{
		Type: LayoutTypeColumn,
		Children: []LayoutNode{
			{
				Type: LayoutTypeRow, ID: "workspace", Hidden: true,
				Children: []LayoutNode{
					{Type: LayoutTypePane, Name: OutputPaneName, Size: Fraction(3)},
					{Type: LayoutTypeBar, Name: "vitals", Size: Fraction(1)},
				},
			},
			{Type: LayoutTypeInput},
		},
	}}
	if err := ValidateLayoutTree(tree); err != nil {
		t.Fatalf("ValidateLayoutTree: %v", err)
	}
}

func TestValidateLayoutTreeAcceptsMeasurableAutoContainer(t *testing.T) {
	t.Parallel()

	tree := LayoutTree{Root: LayoutNode{
		Type: LayoutTypeColumn,
		Children: []LayoutNode{
			{
				Type: LayoutTypeRow,
				Size: AutoSize(),
				Children: []LayoutNode{
					{Type: LayoutTypeBar, Name: "vitals"}, {Type: LayoutTypePane, Name: "map"},
				},
			},
			{Type: LayoutTypeInput},
		},
	}}
	if err := ValidateLayoutTree(tree); err != nil {
		t.Fatalf("ValidateLayoutTree: %v", err)
	}
}

func TestValidateLayoutTreeNamespacesPaneAndBarNamesSeparately(t *testing.T) {
	t.Parallel()

	tree := LayoutTree{Root: LayoutNode{
		Type: LayoutTypeRow,
		Children: []LayoutNode{
			{Type: LayoutTypePane, Name: "shared"},
			{Type: LayoutTypeBar, Name: "shared"},
		},
	}}
	if err := ValidateLayoutTree(tree); err != nil {
		t.Fatalf("ValidateLayoutTree: %v", err)
	}
}

func TestValidateLayoutTreeRejectsInvalidStructure(t *testing.T) {
	t.Parallel()

	zero, one, two := 0, 1, 2
	validChildren := []LayoutNode{testBar("a"), testBar("b")}
	tests := []struct {
		name string
		tree LayoutTree
		want string
	}{
		{
			name: "missing type",
			tree: LayoutTree{Root: LayoutNode{}},
			want: "root: type must be a non-empty string",
		},
		{
			name: "unknown type",
			tree: LayoutTree{Root: LayoutNode{Type: "vitals"}},
			want: `root: unknown type "vitals"`,
		},
		{
			name: "pane requires name",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypePane}},
			want: "pane requires a non-empty name",
		},
		{
			name: "bar requires name",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypeBar}},
			want: "bar requires a non-empty name",
		},
		{
			name: "name belongs only to resources",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypeInput, Name: "other"}},
			want: "name is only valid on pane and bar leaves",
		},
		{
			name: "dividers on a leaf",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypeRow, Children: []LayoutNode{
				{Type: LayoutTypeBar, Name: "vitals", Dividers: true},
				testBar("b"),
			}}},
			want: "dividers are only valid on row and column containers",
		},
		{
			name: "root id",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypeRow, ID: "screen", Children: validChildren}},
			want: "root cannot have an id",
		},
		{
			name: "root size",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypePane, Name: OutputPaneName, Size: Fraction(1)}},
			want: "root cannot have size",
		},
		{
			name: "root minimum",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypePane, Name: OutputPaneName, MinSize: &zero}},
			want: "root cannot have size",
		},
		{
			name: "hidden root",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypeRow, Hidden: true, Children: validChildren}},
			want: "root cannot be hidden",
		},
		{
			name: "leaf cannot have children",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypePane, Name: "chat", Children: validChildren}},
			want: `leaf "pane" cannot have children`,
		},
		{
			name: "leaf cannot have gap",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypePane, Name: "chat", Gap: 1}},
			want: "gap is only valid on row and column",
		},
		{
			name: "container cannot have presentation fields",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypeRow, Children: validChildren, Border: PaneBorderNone}},
			want: "title, border, and separator char are only valid on leaves",
		},
		{
			name: "pane border is a closed enum",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypePane, Name: "chat", Border: PaneBorder("rounded")}},
			want: "border must be",
		},
		{
			name: "pane rejects separator field",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypePane, Name: "chat", SeparatorChar: "="}},
			want: "char is only valid on separator",
		},
		{
			name: "separator character occupies one cell",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypeSeparator, SeparatorChar: "wide"}},
			want: "exactly one terminal cell",
		},
		{
			name: "input has no presentation fields",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypeInput, Border: PaneBorderNone}},
			want: "does not accept presentation fields",
		},
		{
			name: "auto width is unsupported",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypeRow, Children: []LayoutNode{
				{Type: LayoutTypePane, Name: OutputPaneName, Size: AutoSize()}, {Type: LayoutTypeInput},
			}}},
			want: "intrinsic width is not supported",
		},
		{
			name: "id on leaf",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypeRow, Children: []LayoutNode{
				{Type: LayoutTypePane, Name: "chat", ID: "leaf"}, testBar("status"),
			}}},
			want: "id is only valid on row and column regions",
		},
		{
			name: "duplicate ids",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypeColumn, Children: []LayoutNode{
				{Type: LayoutTypeRow, ID: "shared", Children: []LayoutNode{testBar("a"), testBar("b")}},
				{Type: LayoutTypeRow, ID: "shared", Children: []LayoutNode{testBar("c"), testBar("d")}},
			}}},
			want: `root.children[2]: duplicate id "shared" (already used at root.children[1])`,
		},
		{
			name: "hidden anonymous region",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypeColumn, Children: []LayoutNode{
				{Type: LayoutTypeRow, Hidden: true, Children: validChildren}, {Type: LayoutTypeInput},
			}}},
			want: "hidden is only valid on a pane or an identified row or column region",
		},
		{
			name: "hidden bar leaf",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypeRow, Children: []LayoutNode{
				{Type: LayoutTypePane, Name: OutputPaneName}, {Type: LayoutTypeInput},
				{Type: LayoutTypeBar, Name: "status", Hidden: true},
			}}},
			want: "hidden is only valid on a pane or an identified row or column region",
		},
		{
			name: "hidden region contains input",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypeColumn, Children: []LayoutNode{
				{Type: LayoutTypeRow, ID: "input_region", Hidden: true, Children: []LayoutNode{
					{Type: LayoutTypePane, Name: OutputPaneName}, {Type: LayoutTypeInput},
				}},
				testBar("status"),
			}}},
			want: `region "input_region" contains input and cannot be hidden`,
		},
		{
			name: "hidden ancestor region contains nested input",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypeColumn, Children: []LayoutNode{
				{Type: LayoutTypeRow, ID: "workspace", Hidden: true, Children: []LayoutNode{
					{Type: LayoutTypePane, Name: "chat"},
					{Type: LayoutTypeColumn, ID: "io", Children: []LayoutNode{
						{Type: LayoutTypePane, Name: OutputPaneName}, {Type: LayoutTypeInput},
					}},
				}},
				testBar("status"),
			}}},
			want: `region "workspace" contains input and cannot be hidden`,
		},
		{
			name: "duplicate pane names",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypeRow, Children: []LayoutNode{
				{Type: LayoutTypePane, Name: "chat"}, {Type: LayoutTypePane, Name: "chat"},
			}}},
			want: `duplicate pane name "chat"`,
		},
		{
			name: "duplicate bar names",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypeRow, Children: []LayoutNode{
				{Type: LayoutTypeBar, Name: "status"}, {Type: LayoutTypeBar, Name: "status"},
			}}},
			want: `duplicate bar name "status"`,
		},
		{
			name: "min exceeds max",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypeRow, Children: []LayoutNode{
				{Type: LayoutTypeBar, Name: "a", MinSize: &two, MaxSize: &one}, testBar("b"),
			}}},
			want: "min_size must not exceed max_size",
		},
		{
			name: "fixed below minimum",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypeRow, Children: []LayoutNode{
				{Type: LayoutTypeBar, Name: "a", Size: Cells(1), MinSize: &two}, testBar("b"),
			}}},
			want: "fixed size 1 is below min_size 2",
		},
		{
			name: "fixed above maximum",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypeRow, Children: []LayoutNode{
				{Type: LayoutTypeBar, Name: "a", Size: Cells(2), MaxSize: &one}, testBar("b"),
			}}},
			want: "fixed size 2 exceeds max_size 1",
		},
		{
			name: "unknown size kind",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypeRow, Children: []LayoutNode{
				{Type: LayoutTypeBar, Name: "a", Size: LayoutSize{Kind: 99}}, testBar("b"),
			}}},
			want: "unknown layout size kind 99",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateLayoutTree(test.tree)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateLayoutTree() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestRegionVisibilityIsCopyOnWrite(t *testing.T) {
	t.Parallel()

	original := LayoutTree{Root: LayoutNode{
		Type: LayoutTypeColumn,
		Children: []LayoutNode{
			{
				Type: LayoutTypeRow, ID: "workspace",
				Children: []LayoutNode{
					{
						Type: LayoutTypeColumn, ID: "sidebar", Hidden: true,
						Children: []LayoutNode{
							{Type: LayoutTypePane, Name: OutputPaneName},
							{Type: LayoutTypeBar, Name: "status"},
						},
					},
					{Type: LayoutTypePane, Name: "chat"},
				},
			},
			{Type: LayoutTypeInput},
		},
	}}
	if err := ValidateLayoutTree(original); err != nil {
		t.Fatalf("ValidateLayoutTree: %v", err)
	}
	if hidden, found := original.RegionHidden("sidebar"); !found || !hidden {
		t.Fatalf("initial RegionHidden(sidebar) = %v, %v; want true, true", hidden, found)
	}

	updated, found, changed := original.WithRegionVisibility("sidebar", true)
	if !found || !changed {
		t.Fatalf("WithRegionVisibility(sidebar, true) found=%v changed=%v", found, changed)
	}
	if !original.Root.Children[0].Children[0].Hidden {
		t.Fatal("copy-on-write update mutated the previously published tree")
	}
	if updated.Root.Children[0].Children[0].Hidden {
		t.Fatal("updated tree retained the hidden gate")
	}

	idempotent, found, changed := updated.WithRegionVisibility("sidebar", true)
	if !found || changed || !reflect.DeepEqual(idempotent, updated) {
		t.Fatalf("idempotent update found=%v changed=%v tree changed=%v", found, changed, !reflect.DeepEqual(idempotent, updated))
	}
	missing, found, changed := updated.WithRegionVisibility("missing", false)
	if found || changed || !reflect.DeepEqual(missing, updated) {
		t.Fatalf("unknown update found=%v changed=%v tree changed=%v", found, changed, !reflect.DeepEqual(missing, updated))
	}
	if _, found := updated.RegionHidden(""); found {
		t.Fatal("empty region id unexpectedly resolved")
	}
}

func TestPaneVisibilityIsCopyOnWrite(t *testing.T) {
	t.Parallel()

	original := LayoutTree{Root: LayoutNode{
		Type: LayoutTypeColumn,
		Children: []LayoutNode{
			{Type: LayoutTypePane, Name: OutputPaneName},
			{Type: LayoutTypePane, Name: "chat", Hidden: true},
			{Type: LayoutTypeInput},
		},
	}}
	if err := ValidateLayoutTree(original); err != nil {
		t.Fatalf("ValidateLayoutTree: %v", err)
	}
	if hidden, found := original.PaneHidden("chat"); !found || !hidden {
		t.Fatalf("initial PaneHidden(chat) = %v, %v; want true, true", hidden, found)
	}
	if hidden, found := original.PaneHidden(OutputPaneName); !found || hidden {
		t.Fatalf("initial PaneHidden(output) = %v, %v; want false, true", hidden, found)
	}

	updated, found, changed := original.WithPaneVisibility("chat", true)
	if !found || !changed {
		t.Fatalf("WithPaneVisibility(chat, true) found=%v changed=%v", found, changed)
	}
	if !original.Root.Children[1].Hidden {
		t.Fatal("copy-on-write update mutated the previously published tree")
	}
	if updated.Root.Children[1].Hidden {
		t.Fatal("updated tree retained the hidden gate")
	}

	idempotent, found, changed := updated.WithPaneVisibility("chat", true)
	if !found || changed || !reflect.DeepEqual(idempotent, updated) {
		t.Fatalf("idempotent update found=%v changed=%v tree changed=%v", found, changed, !reflect.DeepEqual(idempotent, updated))
	}
	missing, found, changed := updated.WithPaneVisibility("missing", false)
	if found || changed || !reflect.DeepEqual(missing, updated) {
		t.Fatalf("unknown update found=%v changed=%v tree changed=%v", found, changed, !reflect.DeepEqual(missing, updated))
	}
	if _, found := updated.PaneHidden(""); found {
		t.Fatal("empty pane name unexpectedly resolved")
	}
}

func TestValidateLayoutTreeBoundsDepth(t *testing.T) {
	t.Parallel()

	node := testBar("leaf")
	for i := range MaxLayoutDepth + 1 {
		node = LayoutNode{
			Type:     LayoutTypeRow,
			Children: []LayoutNode{testBar("sibling-" + strconv.Itoa(i)), node},
		}
	}
	err := ValidateLayoutTree(LayoutTree{Root: node})
	if err == nil || !strings.Contains(err.Error(), "maximum depth") {
		t.Fatalf("ValidateLayoutTree() error = %v, want depth error", err)
	}
}

func TestDividersValidateOnContainers(t *testing.T) {
	t.Parallel()

	tree := LayoutTree{Root: LayoutNode{
		Type:     LayoutTypeRow,
		Dividers: true,
		Children: []LayoutNode{testBar("a"), testBar("b")},
	}}
	if err := ValidateLayoutTree(tree); err != nil {
		t.Fatalf("dividers on a container should validate: %v", err)
	}
}
