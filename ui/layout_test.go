package ui

import (
	"errors"
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func testBar(name string) LayoutNode {
	return LayoutNode{Type: LayoutTypeBar, Name: name}
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
			{Type: LayoutTypePane, Name: OutputPaneName, Border: PaneBorderNone},
			{Type: LayoutTypeInput, Size: AutoSize()},
			{Type: LayoutTypeBar, Name: "status", Size: AutoSize()},
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
			name: "container needs two children",
			tree: LayoutTree{Root: LayoutNode{Type: LayoutTypeRow, Children: []LayoutNode{testBar("only")}}},
			want: "row needs at least two children",
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
				{Type: LayoutTypeRow, ID: "composer", Hidden: true, Children: []LayoutNode{
					{Type: LayoutTypePane, Name: OutputPaneName}, {Type: LayoutTypeInput},
				}},
				testBar("status"),
			}}},
			want: `region "composer" contains input and cannot be hidden`,
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
	if visible, found := original.RegionVisible("sidebar"); !found || visible {
		t.Fatalf("initial RegionVisible(sidebar) = %v, %v; want false, true", visible, found)
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
	if _, found := updated.RegionVisible(""); found {
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
	if visible, found := original.PaneVisible("chat"); !found || visible {
		t.Fatalf("initial PaneVisible(chat) = %v, %v; want false, true", visible, found)
	}
	if visible, found := original.PaneVisible(OutputPaneName); !found || !visible {
		t.Fatalf("initial PaneVisible(output) = %v, %v; want true, true", visible, found)
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
	if _, found := updated.PaneVisible(""); found {
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

func TestAllocateAxis(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		extent int
		gap    int
		tracks []AxisTrack
		want   []int
	}{
		{
			name:   "default fractions split an odd remainder toward the first child",
			extent: 10, gap: 1,
			tracks: []AxisTrack{{}, {}},
			want:   []int{5, 4},
		},
		{
			name:   "weighted fractions use largest remainder rounding",
			extent: 101,
			tracks: []AxisTrack{{Size: Fraction(7)}, {Size: Fraction(3)}},
			want:   []int{71, 30},
		},
		{
			name:   "fixed is removed before fractions",
			extent: 100, gap: 1,
			tracks: []AxisTrack{{Size: Cells(40)}, {Size: Fraction(1)}},
			want:   []int{40, 59},
		},
		{
			name:   "percentage uses extent after gaps",
			extent: 101, gap: 1,
			tracks: []AxisTrack{{Size: Percent(50)}, {Size: Percent(50)}},
			want:   []int{50, 50},
		},
		{
			name:   "fraction gets remainder after fixed and percentage",
			extent: 100,
			tracks: []AxisTrack{{Size: Percent(30)}, {Size: Cells(20)}, {Size: Fraction(1)}},
			want:   []int{30, 20, 50},
		},
		{
			name:   "auto uses measured preference",
			extent: 20,
			tracks: []AxisTrack{{Size: Fraction(1)}, {Size: AutoSize(), Auto: 3, Max: 5}},
			want:   []int{17, 3},
		},
		{
			name:   "fraction maximum redistributes remainder",
			extent: 100,
			tracks: []AxisTrack{{Size: Fraction(1), Max: 30}, {Size: Fraction(1)}},
			want:   []int{30, 70},
		},
		{
			name:   "all capped fractions leave a trailing tail",
			extent: 100,
			tracks: []AxisTrack{{Size: Fraction(1), Max: 30}, {Size: Fraction(1), Max: 40}},
			want:   []int{30, 40},
		},
		{
			name:   "fixed-only underfill leaves a trailing tail",
			extent: 50,
			tracks: []AxisTrack{{Size: Cells(20)}, {Size: Cells(10)}},
			want:   []int{20, 10},
		},
		{
			name:   "fraction minimum repins and redistributes",
			extent: 20,
			tracks: []AxisTrack{{Size: Fraction(1), Min: 15}, {Size: Fraction(1)}},
			want:   []int{15, 5},
		},
		{
			name:   "overcommit shrinks fractions before percentage and fixed",
			extent: 100,
			tracks: []AxisTrack{
				{Size: Percent(50)}, {Size: Cells(20)},
				{Size: Fraction(1), Min: 30}, {Size: Fraction(1)},
			},
			want: []int{50, 20, 30, 0},
		},
		{
			name:   "overcommit shrinks percentage before fixed",
			extent: 100,
			tracks: []AxisTrack{{Size: Percent(70), Min: 60}, {Size: Cells(50)}},
			want:   []int{60, 40},
		},
		{
			name:   "percentage rounding cannot overrun one cell",
			extent: 1,
			tracks: []AxisTrack{{Size: Percent(50)}, {Size: Percent(50)}, {Size: Percent(50)}},
			want:   []int{0, 0, 1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := AllocateAxis(test.extent, test.gap, test.tracks)
			if err != nil {
				t.Fatalf("AllocateAxis(): %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("AllocateAxis() = %v, want %v", got, test.want)
			}
			if sumInts(got)+(len(got)-1)*test.gap > test.extent {
				t.Fatalf("allocation %v plus gaps exceeds extent %d", got, test.extent)
			}
		})
	}
}

func TestAllocateAxisReportsImpossibleMinimums(t *testing.T) {
	t.Parallel()

	_, err := AllocateAxis(10, 1, []AxisTrack{{Min: 5}, {Min: 5}})
	if !errors.Is(err, ErrLayoutTooSmall) {
		t.Fatalf("AllocateAxis() error = %v, want ErrLayoutTooSmall", err)
	}
}

func TestAllocateAxisValidatesTracks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		extent int
		gap    int
		tracks []AxisTrack
		want   string
	}{
		{name: "negative extent", extent: -1, want: "extent must not be negative"},
		{name: "negative gap", extent: 10, gap: -1, want: "gap must be between"},
		{name: "invalid size", extent: 10, tracks: []AxisTrack{{Size: Fraction(0)}}, want: "layout size must be between"},
		{name: "negative minimum", extent: 10, tracks: []AxisTrack{{Min: -1}}, want: "min must be between"},
		{name: "minimum above maximum", extent: 10, tracks: []AxisTrack{{Min: 2, Max: 1}}, want: "min must not exceed max"},
		{name: "negative auto", extent: 10, tracks: []AxisTrack{{Size: AutoSize(), Auto: -1}}, want: "auto size must be between"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := AllocateAxis(test.extent, test.gap, test.tracks)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("AllocateAxis() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestAllocateAxisIgnoresAutoMeasurementOnOtherSizeKinds(t *testing.T) {
	t.Parallel()

	got, err := AllocateAxis(10, 0, []AxisTrack{
		{Size: Cells(4), Auto: -1},
		{Size: Fraction(1), Auto: MaxLayoutCells + 1},
	})
	if err != nil {
		t.Fatalf("AllocateAxis() rejected irrelevant auto measurements: %v", err)
	}
	if want := []int{4, 6}; !reflect.DeepEqual(got, want) {
		t.Fatalf("AllocateAxis() = %v, want %v", got, want)
	}
}

func TestAllocateAxisDeterministicProperties(t *testing.T) {
	t.Parallel()

	random := rand.New(rand.NewSource(7))
	for iteration := 0; iteration < 2000; iteration++ {
		extent := random.Intn(201)
		gap := random.Intn(4)
		tracks := make([]AxisTrack, 1+random.Intn(6))
		hasUncappedFraction := false
		for i := range tracks {
			track := AxisTrack{Min: random.Intn(11), Auto: random.Intn(31)}
			if random.Intn(3) == 0 {
				track.Max = track.Min + 1 + random.Intn(31)
			}
			switch random.Intn(5) {
			case 0:
				track.Size = LayoutSize{}
				hasUncappedFraction = hasUncappedFraction || track.Max == 0
			case 1:
				track.Size = Cells(1 + random.Intn(100))
			case 2:
				track.Size = Fraction(1 + random.Intn(10))
				hasUncappedFraction = hasUncappedFraction || track.Max == 0
			case 3:
				track.Size = Percent(1 + random.Intn(100))
			case 4:
				track.Size = AutoSize()
			}
			tracks[i] = track
		}

		got, err := AllocateAxis(extent, gap, tracks)
		if err != nil {
			if !errors.Is(err, ErrLayoutTooSmall) {
				t.Fatalf("iteration %d: AllocateAxis(%d, %d, %#v): %v", iteration, extent, gap, tracks, err)
			}
			continue
		}
		again, err := AllocateAxis(extent, gap, tracks)
		if err != nil || !reflect.DeepEqual(got, again) {
			t.Fatalf("iteration %d: allocation is not deterministic: %v/%v then %v/%v", iteration, got, err, again, err)
		}
		if len(got) != len(tracks) {
			t.Fatalf("iteration %d: got %d sizes for %d tracks", iteration, len(got), len(tracks))
		}
		for i, size := range got {
			if size < tracks[i].Min || (tracks[i].Max > 0 && size > tracks[i].Max) {
				t.Fatalf("iteration %d track %d: size %d violates [%d,%d]", iteration, i, size, tracks[i].Min, tracks[i].Max)
			}
		}
		used := sumInts(got) + gap*(len(got)-1)
		if used > extent {
			t.Fatalf("iteration %d: allocation uses %d cells in extent %d", iteration, used, extent)
		}
		if hasUncappedFraction && used != extent {
			t.Fatalf("iteration %d: uncapped fraction left %d of %d cells unused: %v", iteration, extent-used, extent, got)
		}
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
