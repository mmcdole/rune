package tui

import (
	"image"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/widget"
)

func addPane(t *testing.T, m *Model, name string, lines ...string) {
	t.Helper()
	pane := m.panes.Create(name)
	for _, line := range lines {
		pane.Write(line)
	}
	m.invalidateLayout()
}

func resizeModel(t *testing.T, m *Model, width, height int) *Model {
	t.Helper()
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return next.(*Model)
}

func setLayout(m *Model, root ui.LayoutNode) {
	m.layout = ui.LayoutTree{Root: root}
	m.invalidateLayout()
	m.syncViewportSize()
}

func findLeaf(t *testing.T, plan layoutPlan, kind leafKind, name string) placedLeaf {
	t.Helper()
	for _, leaf := range plan.leaves {
		if leaf.kind != kind {
			continue
		}
		if kind == leafPane {
			if leaf.pane.Name() == name {
				return leaf
			}
		} else {
			leafName := leaf.node.Type
			if leaf.node.Type == ui.LayoutTypeBar || leaf.node.Type == ui.LayoutTypeLegacyReference {
				leafName = leaf.node.Name
			}
			if leafName == name {
				return leaf
			}
		}
	}
	t.Fatalf("leaf %q was not placed", name)
	return placedLeaf{}
}

func assertExactBlock(t *testing.T, rendered string, width, height int) []string {
	t.Helper()
	rows := strings.Split(rendered, "\n")
	if len(rows) != height {
		t.Fatalf("rendered height = %d, want %d", len(rows), height)
	}
	for row, content := range rows {
		if got := ansi.StringWidth(content); got != width {
			t.Fatalf("row %d width = %d, want %d: %q", row, got, width, content)
		}
	}
	return rows
}

func TestDefaultTreeProducesOneExactTerminalBlock(t *testing.T) {
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 8)), 32, 12)
	m.syncBars(map[string]ui.BarContent{"status": {Left: "ready"}})
	m.invalidateLayout()

	plan := m.ensureLayout()
	if got, want := plan.output, image.Rect(0, 0, 32, 8); got != want {
		t.Fatalf("output rect = %v, want %v", got, want)
	}
	input := findLeaf(t, plan, leafWidget, ui.LayoutTypeInput)
	status := findLeaf(t, plan, leafWidget, "status")
	if input.outer != image.Rect(0, 8, 32, 11) {
		t.Fatalf("input rect = %v, want (0,8)-(32,11)", input.outer)
	}
	if status.outer != image.Rect(0, 11, 32, 12) {
		t.Fatalf("status rect = %v, want (0,11)-(32,12)", status.outer)
	}
	assertExactBlock(t, m.View().Content, 32, 12)
}

func TestLegacyReferencesAreExplicitLateBoundResources(t *testing.T) {
	t.Run("bar wins over same-named pane", func(t *testing.T) {
		m := resizeModel(t, NewModel(make(chan ui.UIEvent, 8)), 20, 4)
		const alias = "prompt"
		m.syncBars(map[string]ui.BarContent{alias: {Left: "bar"}})
		addPane(t, m, alias)
		setLayout(m, ui.LayoutNode{Type: ui.LayoutTypeLegacyReference, Name: alias})

		plan := m.ensureLayout()
		bar := findLeaf(t, plan, leafWidget, alias)
		if bar.outer != image.Rect(0, 0, 20, 4) {
			t.Fatalf("legacy bar rect = %v, want full terminal", bar.outer)
		}
		for _, leaf := range plan.leaves {
			if leaf.kind == leafPane && leaf.pane.Name() == alias {
				t.Fatal("same-named pane won over registered bar")
			}
		}
	})

	t.Run("reserved structural name can identify a pane", func(t *testing.T) {
		m := resizeModel(t, NewModel(make(chan ui.UIEvent, 8)), 20, 4)
		addPane(t, m, ui.LayoutTypeRow)
		setLayout(m, ui.LayoutNode{Type: ui.LayoutTypeLegacyReference, Name: ui.LayoutTypeRow})

		pane := findLeaf(t, m.ensureLayout(), leafPane, ui.LayoutTypeRow)
		if pane.outer != image.Rect(0, 0, 20, 4) {
			t.Fatalf("reserved-name pane rect = %v, want full terminal", pane.outer)
		}
	})

	t.Run("empty bar owns a colliding v1 resource name", func(t *testing.T) {
		m := resizeModel(t, NewModel(make(chan ui.UIEvent, 8)), 20, 4)
		const alias = "quiet"
		m.syncBars(map[string]ui.BarContent{alias: {}})
		addPane(t, m, alias, "must not render")
		setLayout(m, ui.LayoutNode{Type: ui.LayoutTypeLegacyReference, Name: alias})

		if got := len(m.ensureLayout().leaves); got != 0 {
			t.Fatalf("empty colliding widget resolved %d leaves, want none", got)
		}
	})

	t.Run("arbitrary type is not a registry lookup", func(t *testing.T) {
		m := resizeModel(t, NewModel(make(chan ui.UIEvent, 8)), 20, 4)
		m.syncBars(map[string]ui.BarContent{"vitals": {Left: "HP"}})
		addPane(t, m, "vitals")
		setLayout(m, ui.LayoutNode{Type: "vitals"})
		if got := len(m.ensureLayout().leaves); got != 0 {
			t.Fatalf("invalid arbitrary type resolved %d leaves, want none", got)
		}
	})
}

func TestOutputPaneAndSameNamedBarRemainSeparateResources(t *testing.T) {
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 8)), 20, 5)
	m.syncBars(map[string]ui.BarContent{ui.OutputPaneName: {Left: "bar output"}})
	setLayout(m, ui.LayoutNode{
		Type: ui.LayoutTypeColumn,
		Children: []ui.LayoutNode{
			{Type: ui.LayoutTypeBar, Name: ui.OutputPaneName, Size: ui.AutoSize()},
			{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Border: ui.PaneBorderNone},
		},
	})

	plan := m.ensureLayout()
	bar := findLeaf(t, plan, leafWidget, ui.OutputPaneName)
	if got, want := bar.outer, image.Rect(0, 0, 20, 1); got != want {
		t.Fatalf("same-named bar rect = %v, want %v", got, want)
	}
	if got, want := plan.output, image.Rect(0, 1, 20, 5); got != want {
		t.Fatalf("reserved output pane rect = %v, want %v", got, want)
	}
}

func TestRecursiveRowAndColumnGeometry(t *testing.T) {
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 8)), 100, 30)
	addPane(t, m, "chat", "hello")
	addPane(t, m, "map", "@--+")
	setLayout(m, ui.LayoutNode{
		Type: ui.LayoutTypeColumn,
		Children: []ui.LayoutNode{
			{
				Type: ui.LayoutTypeRow,
				Children: []ui.LayoutNode{
					{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Size: ui.Fraction(7), Border: ui.PaneBorderNone},
					{
						Type: ui.LayoutTypeColumn,
						Size: ui.Fraction(3),
						Children: []ui.LayoutNode{
							{Type: ui.LayoutTypePane, Name: "chat", Size: ui.Fraction(2)},
							{Type: ui.LayoutTypePane, Name: "map", Size: ui.Fraction(1)},
						},
					},
				},
			},
			{Type: ui.LayoutTypeInput, Size: ui.AutoSize()},
		},
	})

	plan := m.ensureLayout()
	if got, want := plan.output, image.Rect(0, 0, 70, 27); got != want {
		t.Fatalf("output rect = %v, want %v", got, want)
	}
	chat := findLeaf(t, plan, leafPane, "chat")
	mapPane := findLeaf(t, plan, leafPane, "map")
	if got, want := chat.outer, image.Rect(70, 0, 100, 19); got != want {
		t.Fatalf("chat outer = %v, want %v", got, want)
	}
	if got, want := mapPane.outer, image.Rect(70, 18, 100, 27); got != want {
		t.Fatalf("map outer = %v, want %v", got, want)
	}
	if got, want := chat.content, image.Rect(71, 1, 99, 18); got != want {
		t.Fatalf("chat content = %v, want %v", got, want)
	}
	if got, want := mapPane.content, image.Rect(71, 19, 99, 26); got != want {
		t.Fatalf("map content = %v, want %v", got, want)
	}
	assertExactBlock(t, m.View().Content, 100, 30)
}

type measuringWidget struct {
	width, height  int
	preferred      func(width int) int
	preferredCalls int
	text           string
}

var _ widget.Widget = (*measuringWidget)(nil)

func (w *measuringWidget) SetSize(width, height int) { w.width, w.height = width, height }
func (w *measuringWidget) PreferredHeight() int {
	w.preferredCalls++
	return w.preferred(max(1, w.width))
}
func (w *measuringWidget) View() string { return w.text }

func TestOnlyAutoTracksRequestIntrinsicMeasurement(t *testing.T) {
	tests := []struct {
		name  string
		size  ui.LayoutSize
		calls int
	}{
		{name: "fixed", size: ui.Cells(2), calls: 1},
		{name: "fraction", size: ui.Fraction(1), calls: 1},
		{name: "percent", size: ui.Percent(25), calls: 1},
		{name: "auto", size: ui.AutoSize(), calls: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := NewModel(make(chan ui.UIEvent, 8))
			measured := &measuringWidget{preferred: func(int) int { return 2 }}
			node := ui.LayoutNode{Type: ui.LayoutTypeBar, Name: "measured", Size: test.size}
			child, ok := m.resolveWidget(node, measured, 20)
			if !ok {
				t.Fatal("measuring widget did not resolve")
			}
			root := &resolvedNode{
				node:     ui.LayoutNode{Type: ui.LayoutTypeColumn},
				children: []*resolvedNode{child},
			}
			m.allocateChildren(root, 12, axisVertical, 20)
			if measured.preferredCalls != test.calls {
				t.Fatalf("PreferredHeight calls = %d, want %d", measured.preferredCalls, test.calls)
			}
		})
	}
}

func TestAutoRowMeasuresChildrenAtAllocatedWidths(t *testing.T) {
	m := NewModel(make(chan ui.UIEvent, 8))
	wrapped := &measuringWidget{
		preferred: func(width int) int { return (24 + width - 1) / width },
		text:      "wrapped",
	}
	wrappedNode := ui.LayoutNode{Type: ui.LayoutTypeBar, Name: "wrapped"}
	wrappedResolved, ok := m.resolveWidget(wrappedNode, wrapped, 20)
	if !ok {
		t.Fatal("wrapped widget did not resolve")
	}
	inputNode := ui.LayoutNode{Type: ui.LayoutTypeInput}
	inputResolved, ok := m.resolveWidget(inputNode, m.input, 20)
	if !ok {
		t.Fatal("input widget did not resolve")
	}
	row := &resolvedNode{
		node:     ui.LayoutNode{Type: ui.LayoutTypeRow},
		children: []*resolvedNode{wrappedResolved, inputResolved},
		hasInput: true,
	}
	if got := m.preferred(row, axisVertical, 20); got != 3 {
		t.Fatalf("auto row preferred height = %d, want 3", got)
	}
	plan := layoutPlan{}
	m.placeNode(row, image.Rect(0, 0, 20, 3), &plan)
	measured := findLeaf(t, plan, leafWidget, "wrapped")
	input := findLeaf(t, plan, leafWidget, ui.LayoutTypeInput)
	if got, want := measured.outer, image.Rect(0, 0, 10, 3); got != want {
		t.Fatalf("wrapped rect = %v, want %v", got, want)
	}
	if got, want := input.outer, image.Rect(10, 0, 20, 3); got != want {
		t.Fatalf("input rect = %v, want %v", got, want)
	}
}

func TestAutoColumnSumsNestedPreferredHeights(t *testing.T) {
	m := NewModel(make(chan ui.UIEvent, 8))
	twoRows := &measuringWidget{preferred: func(int) int { return 2 }, text: "two"}
	twoNode := ui.LayoutNode{Type: ui.LayoutTypeBar, Name: "two", Size: ui.AutoSize()}
	twoResolved, ok := m.resolveWidget(twoNode, twoRows, 30)
	if !ok {
		t.Fatal("two-row widget did not resolve")
	}
	inputNode := ui.LayoutNode{Type: ui.LayoutTypeInput, Size: ui.AutoSize()}
	inputResolved, ok := m.resolveWidget(inputNode, m.input, 30)
	if !ok {
		t.Fatal("input widget did not resolve")
	}
	column := &resolvedNode{
		node:     ui.LayoutNode{Type: ui.LayoutTypeColumn},
		children: []*resolvedNode{twoResolved, inputResolved},
		hasInput: true,
	}
	if got := m.preferred(column, axisVertical, 30); got != 5 {
		t.Fatalf("auto column preferred height = %d, want 5", got)
	}
}

func TestOutputPaneWrapsAtItsResolvedWidth(t *testing.T) {
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 8)), 20, 8)
	addPane(t, m, "map")
	setLayout(m, ui.LayoutNode{
		Type: ui.LayoutTypeRow,
		Children: []ui.LayoutNode{
			{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Border: ui.PaneBorderNone},
			{Type: ui.LayoutTypePane, Name: "map", Size: ui.Cells(8)},
		},
	})

	next, _ := m.Update(ui.EchoLineMsg("abcdefghijklmn"))
	m = next.(*Model)
	if got := m.output.buffer.Count(); got != 2 {
		t.Fatalf("scrollback row count = %d, want 2", got)
	}
	if got := m.output.buffer.At(0); got != "abcdefghijkl" {
		t.Fatalf("first physical row = %q, want twelve cells", got)
	}
	if got := m.output.buffer.At(1); got != "mn" {
		t.Fatalf("second physical row = %q, want remainder", got)
	}
}

func TestResizeReallocatesTreeWithoutReflowingExistingOutputRows(t *testing.T) {
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 8)), 40, 8)
	addPane(t, m, "map")
	setLayout(m, ui.LayoutNode{
		Type: ui.LayoutTypeRow,
		Children: []ui.LayoutNode{
			{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Size: ui.Fraction(3), Border: ui.PaneBorderNone},
			{Type: ui.LayoutTypePane, Name: "map", Size: ui.Fraction(1)},
		},
	})

	next, _ := m.Update(ui.EchoLineMsg(strings.Repeat("a", 35)))
	m = next.(*Model)
	if got := m.output.buffer.Count(); got != 2 {
		t.Fatalf("rows appended at thirty-cell output width = %d, want 2", got)
	}

	m = resizeModel(t, m, 80, 8)
	if got := m.ensureLayout().output.Dx(); got != 60 {
		t.Fatalf("resized output width = %d, want 60", got)
	}
	if got := m.output.buffer.Count(); got != 2 {
		t.Fatalf("resize reflowed existing rows: count = %d, want 2", got)
	}
	next, _ = m.Update(ui.EchoLineMsg(strings.Repeat("b", 50)))
	m = next.(*Model)
	if got := m.output.buffer.Count(); got != 3 {
		t.Fatalf("new fifty-cell line at resized width added count = %d, want 3", got)
	}
}

func TestConstrainedBarAndInputRenderInTheirOwnRowSlots(t *testing.T) {
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 8)), 20, 12)
	m.syncBars(map[string]ui.BarContent{"vitals": {Left: "abcdefghijklmnop"}})
	m.invalidateLayout()
	setLayout(m, ui.LayoutNode{
		Type: ui.LayoutTypeColumn,
		Children: []ui.LayoutNode{
			{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Border: ui.PaneBorderNone},
			{
				Type: ui.LayoutTypeRow,
				Size: ui.AutoSize(),
				Children: []ui.LayoutNode{
					{Type: ui.LayoutTypeBar, Name: "vitals"},
					{Type: ui.LayoutTypeInput},
				},
			},
		},
	})

	plan := m.ensureLayout()
	bar := findLeaf(t, plan, leafWidget, "vitals")
	input := findLeaf(t, plan, leafWidget, ui.LayoutTypeInput)
	if bar.outer != image.Rect(0, 9, 10, 12) || input.outer != image.Rect(10, 9, 20, 12) {
		t.Fatalf("row slots = bar %v input %v", bar.outer, input.outer)
	}
	rows := assertExactBlock(t, m.View().Content, 20, 12)
	plain := ansi.Strip(rows[9])
	if plain[:10] != "abcdefghij" || plain[10:] != strings.Repeat("─", 10) {
		t.Fatalf("first constrained row = %q", plain)
	}
}

func TestHiddenPaneIsPrunedAndOutputReclaimsItsTrack(t *testing.T) {
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 8)), 40, 10)
	addPane(t, m, "map")
	setLayout(m, ui.LayoutNode{
		Type: ui.LayoutTypeRow,
		Children: []ui.LayoutNode{
			{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Border: ui.PaneBorderNone},
			{Type: ui.LayoutTypePane, Name: "map"},
		},
	})
	if got := m.ensureLayout().output.Dx(); got != 20 {
		t.Fatalf("output width with map = %d, want 20", got)
	}

	hidden, found, changed := m.layout.WithPaneVisibility("map", false)
	if !found || !changed {
		t.Fatalf("WithPaneVisibility(map, false) = found %v changed %v", found, changed)
	}
	next, _ := m.Update(ui.UpdateLayoutMsg(hidden))
	m = next.(*Model)
	if got := m.ensureLayout().output.Dx(); got != 40 {
		t.Fatalf("output width without map = %d, want 40", got)
	}
}

func TestSmallTerminalProtectsInputThenOutputWithoutOverflow(t *testing.T) {
	tests := []struct {
		name         string
		height       int
		outputHeight int
		inputTop     int
	}{
		{name: "one row belongs to input", height: 1, outputHeight: 0, inputTop: 0},
		{name: "two rows preserve both", height: 2, outputHeight: 1, inputTop: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := resizeModel(t, NewModel(make(chan ui.UIEvent, 8)), 12, test.height)
			plan := m.ensureLayout()
			if got := plan.output.Dy(); got != test.outputHeight {
				t.Fatalf("output height = %d, want %d", got, test.outputHeight)
			}
			input := findLeaf(t, plan, leafWidget, ui.LayoutTypeInput)
			if input.outer != image.Rect(0, test.inputTop, 12, test.height) {
				t.Fatalf("input rect = %v, want y=%d..%d", input.outer, test.inputTop, test.height)
			}
			for _, leaf := range plan.leaves {
				if leaf.outer.Min.X < 0 || leaf.outer.Min.Y < 0 ||
					leaf.outer.Max.X > 12 || leaf.outer.Max.Y > test.height {
					t.Fatalf("leaf escaped terminal bounds: %v", leaf.outer)
				}
			}
			rows := assertExactBlock(t, m.View().Content, 12, test.height)
			if got := ansi.Strip(rows[test.inputTop]); !strings.Contains(got, "> ") {
				t.Fatalf("protected input row = %q, want editable prompt", got)
			}
		})
	}
}

func TestTinyFallbackProtectsNestedInputAndOutputInTheSameTrack(t *testing.T) {
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 8)), 12, 2)
	addPane(t, m, "extra")
	setLayout(m, ui.LayoutNode{
		Type: ui.LayoutTypeColumn,
		Children: []ui.LayoutNode{
			{
				Type: ui.LayoutTypeColumn,
				Children: []ui.LayoutNode{
					{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Border: ui.PaneBorderNone},
					{Type: ui.LayoutTypeInput, Size: ui.AutoSize()},
				},
			},
			{Type: ui.LayoutTypePane, Name: "extra"},
		},
	})

	plan := m.ensureLayout()
	if got, want := plan.output, image.Rect(0, 0, 12, 1); got != want {
		t.Fatalf("nested output rect = %v, want %v", got, want)
	}
	input := findLeaf(t, plan, leafWidget, ui.LayoutTypeInput)
	if got, want := input.outer, image.Rect(0, 1, 12, 2); got != want {
		t.Fatalf("nested input rect = %v, want %v", got, want)
	}
	for _, leaf := range plan.leaves {
		if leaf.kind == leafPane && leaf.pane.Name() == "extra" {
			t.Fatalf("ordinary pane received a protected row: %v", leaf.outer)
		}
	}
}

func TestFallbackDropsOversizedGapsInsideParent(t *testing.T) {
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 8)), 16, 2)
	setLayout(m, ui.LayoutNode{
		Type: ui.LayoutTypeColumn,
		Gap:  100,
		Children: []ui.LayoutNode{
			{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Border: ui.PaneBorderNone},
			{Type: ui.LayoutTypeInput, Size: ui.AutoSize()},
		},
	})

	plan := m.ensureLayout()
	if got, want := plan.output, image.Rect(0, 0, 16, 1); got != want {
		t.Fatalf("output rect = %v, want %v", got, want)
	}
	if got, want := findLeaf(t, plan, leafWidget, ui.LayoutTypeInput).outer,
		image.Rect(0, 1, 16, 2); got != want {
		t.Fatalf("input rect = %v, want %v", got, want)
	}
	assertExactBlock(t, m.View().Content, 16, 2)
}

func TestHorizontalPaneFramePreservesLegacyContentWidth(t *testing.T) {
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 8)), 20, 4)
	addPane(t, m, "chat", "12345678901234567890", "second")
	setLayout(m, ui.LayoutNode{
		Type:   ui.LayoutTypePane,
		Name:   "chat",
		Border: ui.PaneBorderHorizontal,
	})

	plan := m.ensureLayout()
	pane := findLeaf(t, plan, leafPane, "chat")
	if got, want := pane.content, image.Rect(0, 1, 20, 3); got != want {
		t.Fatalf("horizontal-frame content = %v, want %v", got, want)
	}
	rows := assertExactBlock(t, m.View().Content, 20, 4)
	plain := make([]string, len(rows))
	for i, row := range rows {
		plain[i] = ansi.Strip(row)
	}
	if !strings.Contains(plain[0], " chat ") || strings.ContainsAny(plain[0], "┌┐") {
		t.Fatalf("header = %q, want titled horizontal rule without corners", plain[0])
	}
	if plain[1] != "12345678901234567890" || !strings.HasPrefix(plain[2], "second") {
		t.Fatalf("content rows = %q, want full twenty-cell width", plain[1:3])
	}
	if plain[3] != strings.Repeat("─", 20) {
		t.Fatalf("closing rule = %q", plain[3])
	}
}

func TestPaneFrameEdgesAreLogicalTrackMinima(t *testing.T) {
	tests := []struct {
		name   string
		root   string
		border ui.PaneBorder
		want   int
	}{
		{name: "full frame height", root: ui.LayoutTypeColumn, want: 2},
		{name: "full frame width", root: ui.LayoutTypeRow, want: 2},
		{name: "horizontal frame height", root: ui.LayoutTypeColumn, border: ui.PaneBorderHorizontal, want: 2},
		{name: "horizontal frame has no width minimum", root: ui.LayoutTypeRow, border: ui.PaneBorderHorizontal, want: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := resizeModel(t, NewModel(make(chan ui.UIEvent, 8)), 10, 10)
			addPane(t, m, "chat")
			setLayout(m, ui.LayoutNode{
				Type: test.root,
				Children: []ui.LayoutNode{
					{Type: ui.LayoutTypePane, Name: "chat", Size: ui.Cells(1), Border: test.border},
					{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Border: ui.PaneBorderNone},
				},
			})

			pane := findLeaf(t, m.ensureLayout(), leafPane, "chat")
			got := pane.outer.Dy()
			if test.root == ui.LayoutTypeRow {
				got = pane.outer.Dx()
			}
			if got != test.want {
				t.Fatalf("pane track = %d, want %d", got, test.want)
			}
		})
	}
}

func TestExplicitMaximumCanClipPaneChromeWithoutFallingBackParent(t *testing.T) {
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 8)), 20, 10)
	addPane(t, m, "chat")
	one := 1
	setLayout(m, ui.LayoutNode{
		Type: ui.LayoutTypeColumn,
		Children: []ui.LayoutNode{
			{Type: ui.LayoutTypePane, Name: "chat", Size: ui.Cells(1), MaxSize: &one},
			{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Border: ui.PaneBorderNone},
		},
	})

	plan := m.ensureLayout()
	pane := findLeaf(t, plan, leafPane, "chat")
	if pane.outer.Dy() != 1 {
		t.Fatalf("pane height = %d, want explicit maximum 1", pane.outer.Dy())
	}
	if plan.output != image.Rect(0, 1, 20, 10) {
		t.Fatalf("output rect = %v, want remaining nine rows", plan.output)
	}
}

func TestTinyFallbackStillRespectsHardMaximum(t *testing.T) {
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 8)), 20, 10)
	addPane(t, m, "chat")
	one := 1
	setLayout(m, ui.LayoutNode{
		Type: ui.LayoutTypeColumn,
		Gap:  20,
		Children: []ui.LayoutNode{
			{Type: ui.LayoutTypePane, Name: "chat", MaxSize: &one},
			{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Border: ui.PaneBorderNone},
		},
	})

	plan := m.ensureLayout()
	pane := findLeaf(t, plan, leafPane, "chat")
	if pane.outer.Dy() != 1 {
		t.Fatalf("fallback pane height = %d, want hard maximum 1", pane.outer.Dy())
	}
	if plan.output != image.Rect(0, 1, 20, 10) {
		t.Fatalf("fallback output rect = %v, want remaining nine rows", plan.output)
	}
}

func TestNestedMinimumIncludesGapsAndSharedFrameSeams(t *testing.T) {
	for _, test := range []struct {
		name string
		gap  int
		want int
	}{
		{name: "shared seam", gap: 0, want: 3},
		{name: "explicit gap", gap: 2, want: 6},
	} {
		t.Run(test.name, func(t *testing.T) {
			m := resizeModel(t, NewModel(make(chan ui.UIEvent, 8)), 20, 10)
			addPane(t, m, "one")
			addPane(t, m, "two")
			setLayout(m, ui.LayoutNode{
				Type: ui.LayoutTypeColumn,
				Children: []ui.LayoutNode{
					{
						Type: ui.LayoutTypeColumn,
						Size: ui.Cells(1),
						Gap:  test.gap,
						Children: []ui.LayoutNode{
							{Type: ui.LayoutTypePane, Name: "one"},
							{Type: ui.LayoutTypePane, Name: "two"},
						},
					},
					{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Border: ui.PaneBorderNone},
				},
			})

			if got := m.ensureLayout().output.Min.Y; got != test.want {
				t.Fatalf("output starts at row %d, want nested minimum %d", got, test.want)
			}
		})
	}
}

func TestPaneBordersOwnFourEdgesAndMergeNestedJunctions(t *testing.T) {
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 8)), 20, 8)
	for _, name := range []string{"west", "north", "south"} {
		addPane(t, m, name)
	}
	setLayout(m, ui.LayoutNode{
		Type: ui.LayoutTypeRow,
		Children: []ui.LayoutNode{
			{Type: ui.LayoutTypePane, Name: "west"},
			{
				Type: ui.LayoutTypeColumn,
				Children: []ui.LayoutNode{
					{Type: ui.LayoutTypePane, Name: "north"},
					{Type: ui.LayoutTypePane, Name: "south"},
				},
			},
		},
	})

	rows := assertExactBlock(t, m.View().Content, 20, 8)
	plain := make([][]rune, len(rows))
	for i, row := range rows {
		plain[i] = []rune(ansi.Strip(row))
	}
	if plain[0][0] != '┌' || plain[0][10] != '┬' || plain[0][19] != '┐' {
		t.Fatalf("top border junctions = %q", string(plain[0]))
	}
	if plain[4][10] != '├' || plain[4][19] != '┤' {
		t.Fatalf("nested shared boundary = %q, want left and right T junctions", string(plain[4]))
	}
	if plain[7][0] != '└' || plain[7][10] != '┴' || plain[7][19] != '┘' {
		t.Fatalf("bottom border junctions = %q", string(plain[7]))
	}
	west := findLeaf(t, m.ensureLayout(), leafPane, "west")
	if got, want := west.content, image.Rect(1, 1, 10, 7); got != want {
		t.Fatalf("west content = %v, want %v", got, want)
	}
}

func TestPaneTitleClipsWideTextAndVisualizesControlsInsideCorners(t *testing.T) {
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 8)), 12, 4)
	addPane(t, m, "map")
	title := "界界界界界界\nignored"
	setLayout(m, ui.LayoutNode{
		Type:  ui.LayoutTypePane,
		Name:  "map",
		Title: &title,
	})

	rows := assertExactBlock(t, m.View().Content, 12, 4)
	top := []rune(ansi.Strip(rows[0]))
	if top[0] != '┌' || top[len(top)-1] != '┐' {
		t.Fatalf("wide clipped title overwrote pane corners: %q", string(top))
	}
	if strings.Contains(string(top), "\n") {
		t.Fatalf("title injected a physical newline: %q", string(top))
	}
}

func TestSearchUsesTheSameResolvedOutputRectangleAsView(t *testing.T) {
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 16)), 40, 16)
	if got := m.ensureLayout().output.Dy(); got != 13 {
		t.Fatalf("normal output height = %d, want 13", got)
	}

	next, _ := m.Update(ui.ShowSearchMsg{})
	m = next.(*Model)
	plan := m.ensureLayout()
	if got := plan.output.Dy(); got != 11 {
		t.Fatalf("search output height = %d, want 11", got)
	}
	if got := len(strings.Split(m.output.viewport.View(), "\n")); got != plan.output.Dy() {
		t.Fatalf("viewport height = %d, want resolved output height %d", got, plan.output.Dy())
	}
	assertExactBlock(t, m.View().Content, 40, 16)
}

func TestContainerDividersDrawBetweenActiveChildren(t *testing.T) {
	noTitle := ""
	band := func(hideSocial bool) ui.LayoutNode {
		return ui.LayoutNode{
			Type: ui.LayoutTypeColumn,
			Children: []ui.LayoutNode{
				{
					Type: ui.LayoutTypeRow, Dividers: true, Size: ui.Cells(4),
					Children: []ui.LayoutNode{
						{Type: ui.LayoutTypePane, Name: "social", Size: ui.Fraction(2),
							Border: ui.PaneBorderHorizontal, Title: &noTitle, Hidden: hideSocial},
						{Type: ui.LayoutTypePane, Name: "map", Size: ui.Fraction(1),
							Border: ui.PaneBorderHorizontal, Title: &noTitle},
					},
				},
				{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Border: ui.PaneBorderNone},
			},
		}
	}

	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 8)), 21, 8)
	addPane(t, m, "social")
	addPane(t, m, "map")
	setLayout(m, band(false))

	plan := m.ensureLayout()
	social := findLeaf(t, plan, leafPane, "social")
	mapLeaf := findLeaf(t, plan, leafPane, "map")
	if social.outer != image.Rect(0, 0, 13, 4) || mapLeaf.outer != image.Rect(14, 0, 21, 4) {
		t.Fatalf("band rects = %v and %v, want the divider cell between x=13 and x=14",
			social.outer, mapLeaf.outer)
	}

	rows := assertExactBlock(t, m.View().Content, 21, 8)
	plain := make([][]rune, len(rows))
	for i, row := range rows {
		plain[i] = []rune(ansi.Strip(row))
	}
	if plain[0][0] != '─' || plain[0][20] != '─' {
		t.Fatalf("horizontal panes grew outer walls: %q", string(plain[0]))
	}
	if plain[0][13] != '┬' || plain[1][13] != '│' || plain[2][13] != '│' || plain[3][13] != '┴' {
		t.Fatalf("divider column = %q and %q, want tee, bar, bar, tee at x=13",
			string(plain[0]), string(plain[3]))
	}

	setLayout(m, band(true))
	rows = assertExactBlock(t, m.View().Content, 21, 8)
	for i, row := range rows {
		if strings.ContainsRune(ansi.Strip(row), '│') {
			t.Fatalf("row %d still draws a divider with one active child: %q", i, row)
		}
	}
}

func TestContainerDividersReuseFramedPaneSeams(t *testing.T) {
	noTitle := ""
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 8)), 20, 6)
	addPane(t, m, "left")
	addPane(t, m, "right")
	setLayout(m, ui.LayoutNode{
		Type: ui.LayoutTypeRow, Dividers: true,
		Children: []ui.LayoutNode{
			{Type: ui.LayoutTypePane, Name: "left", Title: &noTitle},
			{Type: ui.LayoutTypePane, Name: "right", Title: &noTitle},
		},
	})

	plan := m.ensureLayout()
	left := findLeaf(t, plan, leafPane, "left")
	right := findLeaf(t, plan, leafPane, "right")
	if left.outer != image.Rect(0, 0, 11, 6) || right.outer != image.Rect(10, 0, 20, 6) {
		t.Fatalf("pane rects = %v and %v, want one shared boundary at x=10", left.outer, right.outer)
	}

	rows := assertExactBlock(t, m.View().Content, 20, 6)
	middle := []rune(ansi.Strip(rows[2]))
	for _, x := range []int{0, 10, 19} {
		if middle[x] != '│' {
			t.Fatalf("middle row = %q, want vertical rule at x=%d", string(middle), x)
		}
	}
	if middle[9] == '│' || middle[11] == '│' {
		t.Fatalf("divider added walls beside the shared seam: %q", string(middle))
	}
}

func TestColumnDividerDrawsInsideDeclaredGap(t *testing.T) {
	noTitle := ""
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 8)), 15, 9)
	addPane(t, m, "north")
	addPane(t, m, "south")
	setLayout(m, ui.LayoutNode{
		Type: ui.LayoutTypeColumn, Dividers: true, Gap: 3,
		Children: []ui.LayoutNode{
			{Type: ui.LayoutTypePane, Name: "north", Border: ui.PaneBorderNone, Title: &noTitle},
			{Type: ui.LayoutTypePane, Name: "south", Border: ui.PaneBorderNone, Title: &noTitle},
		},
	})

	plan := m.ensureLayout()
	north := findLeaf(t, plan, leafPane, "north")
	south := findLeaf(t, plan, leafPane, "south")
	if north.outer != image.Rect(0, 0, 15, 3) || south.outer != image.Rect(0, 6, 15, 9) {
		t.Fatalf("pane rects = %v and %v, want a three-row gap", north.outer, south.outer)
	}

	rows := assertExactBlock(t, m.View().Content, 15, 9)
	if got := ansi.Strip(rows[4]); got != strings.Repeat("─", 15) {
		t.Fatalf("middle gap row = %q, want divider", got)
	}
	if strings.ContainsRune(ansi.Strip(rows[3]), '─') || strings.ContainsRune(ansi.Strip(rows[5]), '─') {
		t.Fatalf("divider escaped the center of its gap: %q / %q", rows[3], rows[5])
	}
}

func TestDividerContainerPropagatesItsContinuousFrame(t *testing.T) {
	noTitle := ""
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 8)), 20, 8)
	for _, name := range []string{"left", "right", "bottom"} {
		addPane(t, m, name)
	}
	setLayout(m, ui.LayoutNode{
		Type: ui.LayoutTypeColumn,
		Children: []ui.LayoutNode{
			{
				Type: ui.LayoutTypeRow, Dividers: true,
				Children: []ui.LayoutNode{
					{Type: ui.LayoutTypePane, Name: "left", Border: ui.PaneBorderHorizontal, Title: &noTitle},
					{Type: ui.LayoutTypePane, Name: "right", Border: ui.PaneBorderHorizontal, Title: &noTitle},
				},
			},
			{Type: ui.LayoutTypePane, Name: "bottom", Title: &noTitle},
		},
	})

	plan := m.ensureLayout()
	left := findLeaf(t, plan, leafPane, "left")
	bottom := findLeaf(t, plan, leafPane, "bottom")
	if left.outer.Max.Y != bottom.outer.Min.Y+1 {
		t.Fatalf("nested seam does not overlap: top ends at %d, bottom starts at %d",
			left.outer.Max.Y, bottom.outer.Min.Y)
	}

	rows := assertExactBlock(t, m.View().Content, 20, 8)
	seam := ansi.Strip(rows[bottom.outer.Min.Y])
	if strings.Count(seam, "─") < 17 {
		t.Fatalf("nested seam is not continuous: %q", seam)
	}
}
