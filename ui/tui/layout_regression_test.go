package tui

import (
	"fmt"
	"image"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mmcdole/rune/ui"
)

func containedInputLayout(border ui.PaneBorder, inputSize ui.LayoutSize, gap int) ui.LayoutNode {
	none := ""
	return ui.LayoutNode{Type: ui.LayoutTypeRow, Dividers: true, Gap: gap, Children: []ui.LayoutNode{
		{Type: ui.LayoutTypePane, Name: "left", Border: ui.PaneBorderNone, Size: ui.Cells(5)},
		{Type: ui.LayoutTypeColumn, Gap: gap, Children: []ui.LayoutNode{
			{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Border: border, Title: &none},
			{Type: ui.LayoutTypeInput, Size: inputSize},
		}},
		{Type: ui.LayoutTypePane, Name: "right", Border: ui.PaneBorderNone, Size: ui.Cells(5)},
	}}
}

func TestContainedInputSharesBordersAndUsesAssignedHeight(t *testing.T) {
	for _, border := range []ui.PaneBorder{ui.PaneBorderNone, ui.PaneBorderHorizontal, ui.PaneBorderFull} {
		for _, height := range []int{3, 5} {
			t.Run(fmt.Sprintf("%s/%d", border, height), func(t *testing.T) {
				m := resizeModel(t, NewModel(make(chan ui.UIEvent, 100)), 32, 10)
				setLayout(m, containedInputLayout(border, ui.Cells(height), 0))
				input := findLeaf(t, m.layoutPlan, "", ui.LayoutTypeInput)
				output := findLeaf(t, m.layoutPlan, ui.LayoutTypePane, ui.OutputPaneName)
				if border != ui.PaneBorderNone && output.outer.Max.Y-1 != input.outer.Min.Y {
					t.Fatalf("output and input have separate borders: %v %v", output.outer, input.outer)
				}
				if m.layoutPlan.output.Dx() != input.content.Dx() {
					t.Fatalf("output/input content widths differ: %v %v", output.content, input.content)
				}
				rows := strings.Split(ansi.Strip(m.View().Content), "\n")
				for y, row := range rows {
					if strings.Contains(row, "││") {
						t.Fatalf("doubled divider at row %d: %q", y, row)
					}
				}
				left, right := input.content.Min.X-1, input.content.Max.X
				top, bottom := []rune(rows[input.content.Min.Y]), []rune(rows[9])
				if top[left] != '├' || top[right] != '┤' || bottom[left] != '└' || bottom[right] != '┘' {
					t.Fatalf("input rules do not meet side dividers:\n%s", strings.Join(rows, "\n"))
				}
			})
		}
	}
}

func TestExplicitGapsRemainBetweenOutputAndInput(t *testing.T) {
	for _, gap := range []int{1, 3} {
		m := resizeModel(t, NewModel(make(chan ui.UIEvent, 100)), 60, 20)
		setLayout(m, containedInputLayout(ui.PaneBorderFull, ui.AutoSize(), gap))
		input := findLeaf(t, m.layoutPlan, "", ui.LayoutTypeInput)
		output := findLeaf(t, m.layoutPlan, ui.LayoutTypePane, ui.OutputPaneName)
		if got := input.outer.Min.Y - output.outer.Max.Y; got != gap {
			t.Fatalf("gap = %d, want %d", got, gap)
		}
	}
}

func TestContainedPickerBordersUseColumnBoundary(t *testing.T) {
	for _, inline := range []bool{false, true} {
		for _, gap := range []int{0, 2} {
			t.Run(fmt.Sprintf("inline=%t/gap=%d", inline, gap), func(t *testing.T) {
				m := resizeModel(t, NewModel(make(chan ui.UIEvent, 100)), 80, 24)
				setLayout(m, containedInputLayout(ui.PaneBorderFull, ui.AutoSize(), gap))
				m.Update(ui.ShowPickerMsg{Title: "Aliases", Inline: inline, Items: []ui.PickerItem{{Text: "north"}, {Text: "south"}}})
				input := findLeaf(t, m.layoutPlan, "", ui.LayoutTypeInput)
				rows := strings.Split(ansi.Strip(m.View().Content), "\n")
				// The picker remains above the three-row command field.
				pickerBottom := input.content.Max.Y - 4
				for y := input.content.Min.Y + 1; y < pickerBottom; y++ {
					row := []rune(rows[y])
					for _, x := range []int{input.outer.Min.X, input.outer.Max.X - 1} {
						if row[x] != '│' {
							t.Fatalf("missing column edge at (%d,%d): %q", x, y, rows[y])
						}
					}
					for _, x := range []int{input.content.Min.X, input.content.Max.X - 1} {
						if x > input.outer.Min.X && x < input.outer.Max.X-1 && row[x] == '│' {
							t.Fatalf("inset picker border at (%d,%d): %q", x, y, rows[y])
						}
					}
				}
				bottom := []rune(rows[pickerBottom])
				left, right := '└', '┘'
				if input.content.Min.X > input.outer.Min.X {
					left = '├'
				}
				if input.content.Max.X < input.outer.Max.X {
					right = '┤'
				}
				if bottom[input.outer.Min.X] != left || bottom[input.outer.Max.X-1] != right {
					t.Fatalf("picker bottom does not join column edges: %q", rows[pickerBottom])
				}
			})
		}
	}
}

func TestContainedInputModesPreserveLabelsAndCursor(t *testing.T) {
	for _, mode := range []string{"normal", "composer", "picker", "search"} {
		for _, size := range []image.Point{image.Pt(100, 30), image.Pt(20, 8), image.Pt(3, 3), image.Pt(1, 1)} {
			t.Run(fmt.Sprintf("%s/%dx%d", mode, size.X, size.Y), func(t *testing.T) {
				m := resizeModel(t, NewModel(make(chan ui.UIEvent, 100)), size.X, size.Y)
				setLayout(m, containedInputLayout(ui.PaneBorderFull, ui.AutoSize(), 0))
				switch mode {
				case "normal":
					m.Update(ui.SetInputMsg("north"))
				case "composer":
					m.Update(ui.SetInputMsg("first\nsecond"))
				case "picker":
					m.Update(ui.ShowPickerMsg{Title: "Aliases", Items: []ui.PickerItem{{Text: "north"}, {Text: "south"}}})
				case "search":
					m.Update(ui.ShowSearchMsg{Query: "query"})
				}
				input := findLeaf(t, m.layoutPlan, "", ui.LayoutTypeInput)
				if input.content.Empty() {
					t.Fatal("input content was consumed by borders")
				}
				view := m.View().Content
				assertExactBlock(t, view, size.X, size.Y)
				plain := ansi.Strip(view)
				if (mode == "search" || mode == "picker") && !strings.Contains(plain, "█") {
					t.Fatalf("focused %s cursor missing:\n%s", mode, plain)
				}
				if size.X == 100 {
					var labels []string
					switch mode {
					case "normal":
						labels = []string{"north"}
					case "composer":
						labels = []string{"VERBATIM", "Enter send", "first", "second"}
					case "picker":
						labels = []string{"Aliases:", "north", "south", "> "}
					}
					for _, label := range labels {
						if !strings.Contains(plain, label) {
							t.Errorf("missing %q:\n%s", label, plain)
						}
					}
				}
			})
		}
	}
}

type countedPane struct {
	paneResource
	applications int
}

func (p *countedPane) SetSize(width, height int) {
	p.applications++
	p.paneResource.SetSize(width, height)
}

func TestUpdateAppliesGeometryOnce(t *testing.T) {
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 100)), 80, 30)
	probe := &countedPane{paneResource: m.panes.Create("probe")}
	m.panes.byName["probe"] = probe
	setLayout(m, ui.LayoutNode{Type: ui.LayoutTypeColumn, Children: []ui.LayoutNode{
		{Type: ui.LayoutTypePane, Name: "probe", Size: ui.Cells(3)},
		{Type: ui.LayoutTypePane, Name: ui.OutputPaneName},
		{Type: ui.LayoutTypeInput, Size: ui.AutoSize()},
	}})
	for _, msg := range []tea.Msg{
		tea.WindowSizeMsg{Width: 90, Height: 30},
		ui.UpdateBarsMsg{},
		ui.ShowSearchMsg{Query: "test"},
		tea.KeyPressMsg{Code: tea.KeyEsc},
		ui.SetInputMsg("first\nsecond"),
	} {
		probe.applications = 0
		m.Update(msg)
		m.View()
		if probe.applications != 1 {
			t.Fatalf("%T applied geometry %d times", msg, probe.applications)
		}
	}
}

func TestNestedConstrainedInputDoesNotShareItsEditableRow(t *testing.T) {
	for _, height := range []int{1, 2, 3} {
		m := resizeModel(t, NewModel(make(chan ui.UIEvent, 100)), 30, 10)
		setLayout(m, ui.LayoutNode{Type: ui.LayoutTypeColumn, Children: []ui.LayoutNode{
			{Type: ui.LayoutTypeColumn, Children: []ui.LayoutNode{
				{Type: ui.LayoutTypePane, Name: ui.OutputPaneName},
				{Type: ui.LayoutTypeInput, Size: ui.Cells(height)},
			}},
			{Type: ui.LayoutTypePane, Name: "below", Size: ui.Cells(3)},
		}})
		m.Update(ui.SetInputMsg("EDIT"))
		if !strings.Contains(ansi.Strip(m.View().Content), "EDIT") {
			t.Fatalf("height %d: neighboring frame erased input:\n%s", height, ansi.Strip(m.View().Content))
		}
	}
}

// The upper and lower splits intentionally differ, as in the reported layout.
func staggeredLayout() ui.LayoutNode {
	pane := func(name string, size ui.LayoutSize) ui.LayoutNode {
		return ui.LayoutNode{Type: ui.LayoutTypePane, Name: name, Border: ui.PaneBorderNone, Size: size}
	}
	return ui.LayoutNode{Type: ui.LayoutTypeColumn, Children: []ui.LayoutNode{
		{Type: ui.LayoutTypeRow, Size: ui.Cells(6), Dividers: true, Children: []ui.LayoutNode{
			pane("map", ui.Cells(12)), pane("chat", ui.Fraction(1)), pane("targets", ui.Cells(22)),
		}},
		{Type: ui.LayoutTypeSeparator, Size: ui.AutoSize()},
		{Type: ui.LayoutTypeRow, Dividers: true, Children: []ui.LayoutNode{
			pane("stats", ui.Cells(16)),
			{Type: ui.LayoutTypeColumn, Children: []ui.LayoutNode{
				pane(ui.OutputPaneName, ui.Fraction(1)),
				{Type: ui.LayoutTypeInput, Size: ui.AutoSize()},
			}},
			pane("group", ui.Cells(18)),
		}},
	}}
}

func TestStaggeredBandsAndInputJoinDividers(t *testing.T) {
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 100)), 80, 24)
	setLayout(m, staggeredLayout())
	input := findLeaf(t, m.layoutPlan, "", ui.LayoutTypeInput)
	points := map[image.Point]string{
		image.Pt(12, 6): "┴", // Upper divider terminates on band separator.
		image.Pt(16, 6): "┬", // Lower divider starts on band separator.
		image.Pt(input.outer.Min.X-1, input.outer.Min.Y): "├",
		image.Pt(input.outer.Max.X, input.outer.Min.Y):   "┤",
	}
	rows := assertExactBlock(t, m.View().Content, 80, 24)
	for point, want := range points {
		if got := string([]rune(ansi.Strip(rows[point.Y]))[point.X]); got != want {
			t.Errorf("junction %v = %q, want %q", point, got, want)
		}
	}
}

func TestResolveAndViewDoNotResizeSurfaces(t *testing.T) {
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 100)), 80, 24)
	m.Update(ui.SetInputMsg(strings.Repeat("a\nb\n", 12)))
	before := m.layoutPlan
	mode, count := m.output.viewport.Mode(), m.output.viewport.NewLineCount()
	for range 3 {
		m.resolveLayout()
		m.View()
	}
	if before.leaves[0] != m.layoutPlan.leaves[0] || before.output != m.layoutPlan.output ||
		mode != m.output.viewport.Mode() || count != m.output.viewport.NewLineCount() {
		t.Fatal("measurement or painting changed applied geometry/navigation")
	}
}

func TestAutoPaneMeasuresWrappedContentAndBounds(t *testing.T) {
	for _, test := range []struct{ width, limit, want int }{
		{12, 20, 4}, // 20 cells at inner width 10, plus two frame rows.
		{7, 20, 6},
		{7, 4, 4},
	} {
		m := resizeModel(t, NewModel(make(chan ui.UIEvent, 100)), test.width, 24)
		addPane(t, m, "chat", strings.Repeat("a", 20))
		setLayout(m, ui.LayoutNode{Type: ui.LayoutTypeColumn, Children: []ui.LayoutNode{
			{Type: ui.LayoutTypePane, Name: "chat", Size: ui.AutoSize(), MaxSize: &test.limit},
			{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Border: ui.PaneBorderNone},
			{Type: ui.LayoutTypeInput, Size: ui.AutoSize()},
		}})
		if got := findLeaf(t, m.layoutPlan, ui.LayoutTypePane, "chat").outer.Dy(); got != test.want {
			t.Errorf("width %d, limit %d: auto pane height %d, want %d", test.width, test.limit, got, test.want)
		}
	}
}

func TestTitleCannotCoverStaggeredJunction(t *testing.T) {
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 100)), 30, 14)
	title, none := strings.Repeat("title", 20), ""
	setLayout(m, ui.LayoutNode{Type: ui.LayoutTypeColumn, Children: []ui.LayoutNode{
		{Type: ui.LayoutTypeRow, Size: ui.Cells(5), Children: []ui.LayoutNode{
			{Type: ui.LayoutTypePane, Name: "left", Title: &none},
			{Type: ui.LayoutTypePane, Name: "right", Title: &none},
		}},
		{Type: ui.LayoutTypePane, Name: "bottom", Title: &title},
		{Type: ui.LayoutTypeInput, Size: ui.AutoSize()},
	}})
	left := findLeaf(t, m.layoutPlan, ui.LayoutTypePane, "left")
	bottom := findLeaf(t, m.layoutPlan, ui.LayoutTypePane, "bottom")
	row := []rune(ansi.Strip(strings.Split(m.View().Content, "\n")[bottom.outer.Min.Y]))
	if got := row[left.outer.Max.X-1]; got != '┴' {
		t.Fatalf("title covered junction: %q", string(row))
	}
}

func TestOutputTitleDefaultsToEmpty(t *testing.T) {
	custom, empty := "Transcript", ""
	for _, border := range []ui.PaneBorder{ui.PaneBorderFull, ui.PaneBorderHorizontal} {
		for _, tc := range []struct {
			name  string
			title *string
			want  string
		}{
			{"default", nil, ""},
			{"explicit", &custom, custom},
			{"empty", &empty, ""},
		} {
			t.Run(string(border)+"/"+tc.name, func(t *testing.T) {
				m := resizeModel(t, NewModel(make(chan ui.UIEvent, 100)), 40, 10)
				setLayout(m, ui.LayoutNode{Type: ui.LayoutTypeColumn, Children: []ui.LayoutNode{
					{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Border: border, Title: tc.title},
					{Type: ui.LayoutTypeInput},
				}})
				check := func() {
					t.Helper()
					row := ansi.Strip(strings.Split(m.View().Content, "\n")[0])
					if got := strings.Trim(row, "─┌┐ "); got != tc.want {
						t.Fatalf("header text = %q, want %q", got, tc.want)
					}
				}
				check()
				m.output.Write(strings.Repeat("line\n", 30))
				m.output.ScrollToTop()
				m.output.Write("new line")
				check()
			})
		}
	}
}

func TestShortPaneTitlesPreserveRemainingTopBorder(t *testing.T) {
	for _, border := range []ui.PaneBorder{ui.PaneBorderFull, ui.PaneBorderHorizontal} {
		m := resizeModel(t, NewModel(make(chan ui.UIEvent, 100)), 40, 10)
		setLayout(m, ui.LayoutNode{Type: ui.LayoutTypeRow, Dividers: true, Children: []ui.LayoutNode{
			{Type: ui.LayoutTypePane, Name: "social", Border: border},
			{Type: ui.LayoutTypePane, Name: "map", Border: border},
		}})
		row := []rune(ansi.Strip(strings.Split(m.View().Content, "\n")[0]))
		for _, name := range []string{"social", "map"} {
			leaf := findLeaf(t, m.layoutPlan, ui.LayoutTypePane, name)
			if got := row[leaf.outer.Max.X-2]; got != '─' {
				t.Fatalf("%s %s erased top border: %q", border, name, string(row))
			}
		}
	}
}

func BenchmarkLayoutFrame(b *testing.B) {
	m := NewModel(make(chan ui.UIEvent, 100))
	m.width, m.height, m.initialized = 160, 50, true
	m.layout = ui.LayoutTree{Root: staggeredLayout()}
	m.applyLayout()
	for _, name := range []string{"map", "chat", "targets", "stats", "group", ui.OutputPaneName} {
		m.panes.Write(name, strings.Repeat("sample text\n", 30))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		m.applyLayout()
		m.View()
	}
}

func TestSearchToComposerUsesFinalGeometry(t *testing.T) {
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 100)), 80, 30)
	m.Update(ui.ShowSearchMsg{Query: "missing"})
	m.Update(ui.SetInputMsg("one\ntwo\nthree\nfour\nfive"))
	leaf := findLeaf(t, m.layoutPlan, "", ui.LayoutTypeInput)
	if got, want := leaf.outer.Dy(), m.input.MeasureHeight(leaf.content.Dx(), ui.MaxLayoutCells); got != want {
		t.Fatalf("search -> composer allocated height %d, preferred %d", got, want)
	}
}

func TestFixedNestedFramesShareBoundary(t *testing.T) {
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 100)), 30, 11)
	setLayout(m, ui.LayoutNode{Type: ui.LayoutTypeColumn, Children: []ui.LayoutNode{
		{Type: ui.LayoutTypeRow, Children: []ui.LayoutNode{
			{Type: ui.LayoutTypeColumn, Children: []ui.LayoutNode{
				{Type: ui.LayoutTypePane, Name: "a", Size: ui.Cells(4)},
				{Type: ui.LayoutTypePane, Name: "b", Size: ui.Cells(5)},
			}},
			{Type: ui.LayoutTypePane, Name: "c"},
		}},
		{Type: ui.LayoutTypeInput, Size: ui.AutoSize()},
	}})
	plan := m.layoutPlan
	a := findLeaf(t, plan, ui.LayoutTypePane, "a")
	c := findLeaf(t, plan, ui.LayoutTypePane, "c")
	if a.outer.Max.X-1 != c.outer.Min.X {
		t.Fatalf("nested panes do not share boundary: a=%v c=%v", a.outer, c.outer)
	}
}

func TestHorizontalPressureKeepsInputReachable(t *testing.T) {
	for _, width := range []int{100, 40, 3, 1} {
		m := resizeModel(t, NewModel(make(chan ui.UIEvent, 100)), width, 20)
		setLayout(m, ui.LayoutNode{Type: ui.LayoutTypeRow, Children: []ui.LayoutNode{
			{Type: ui.LayoutTypeColumn, Children: []ui.LayoutNode{
				{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Border: ui.PaneBorderNone},
				{Type: ui.LayoutTypeInput, Size: ui.AutoSize()},
			}},
			{Type: ui.LayoutTypePane, Name: "sidebar", Size: ui.Cells(100), Border: ui.PaneBorderNone},
		}})
		leaf := findLeaf(t, m.layoutPlan, "", ui.LayoutTypeInput)
		if leaf.outer.Empty() {
			t.Fatalf("input disappeared at width %d", width)
		}
		assertExactBlock(t, m.View().Content, width, 20)
	}
}

func TestNestedInputSurvivesConstrainedGeometryInEveryMode(t *testing.T) {
	for _, width := range []int{1, 2, 3, 40, 100} {
		for _, height := range []int{1, 2, 3, 8, 30} {
			for _, mode := range []string{"normal", "composer", "search"} {
				m := resizeModel(t, NewModel(make(chan ui.UIEvent, 100)), width, height)
				setLayout(m, staggeredLayout())
				switch mode {
				case "normal":
					m.Update(ui.SetInputMsg("edit"))
				case "composer":
					m.Update(ui.SetInputMsg("one\ntwo\nthree\nfour\nfive"))
				case "search":
					m.Update(ui.ShowSearchMsg{Query: "query"})
				}
				input := findLeaf(t, m.layoutPlan, "", ui.LayoutTypeInput)
				if input.content.Empty() || !input.content.In(image.Rect(0, 0, width, height)) {
					t.Fatalf("%s at %dx%d: input outside terminal: %v", mode, width, height, input.content)
				}
				view := m.View().Content
				assertExactBlock(t, view, width, height)
				if mode == "search" && !strings.Contains(ansi.Strip(view), "█") {
					t.Fatalf("search at %dx%d lost its cursor: %q", width, height, view)
				}
			}
		}
	}
}
