package widget

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/style"
)

func newTestInput(width int) *Input {
	styles := style.DefaultStyles()
	in := NewInput(styles, NewSearch(NewScrollback(100), styles))
	in.SetSize(width, in.MeasureHeight(width, ui.MaxLayoutCells))
	return in
}

func TestModalPickerShowsResultsAboveItsQueryField(t *testing.T) {
	in := newTestInput(40)
	in.SetValue("unfinished command")
	in.ShowPicker(ui.ShowPickerMsg{Title: "Aliases", Items: []ui.PickerItem{{Text: "north"}, {Text: "south"}}})
	in.SetSize(40, in.MeasureHeight(in.width, 1<<14)+2)
	plan := in.layout(40, in.height)
	rows := strings.Split(text.StripANSI(in.View()), "\n")
	if len(rows) != in.height || !strings.Contains(rows[0], "north") || !strings.Contains(rows[in.height-1], "Aliases: █") {
		t.Fatalf("picker placement changed: %q", rows)
	}
	if row := in.SeparatorRow(40, in.height); row != plan.results.Max.Y {
		t.Fatalf("picker separator disagrees with content geometry: %d, %+v", row, plan)
	}
	if strings.Contains(in.View(), "─") {
		t.Fatal("content rendered renderer-owned rules")
	}
	if strings.Contains(in.View(), "unfinished command") || strings.Count(in.View(), "█") != 1 {
		t.Fatal("modal picker exposed the inactive command input")
	}
	in.HidePicker()
	in.SetSize(in.width, in.MeasureHeight(in.width, ui.MaxLayoutCells))
	if in.Value() != "unfinished command" || !strings.Contains(text.StripANSI(in.View()), "unfinished command") {
		t.Fatal("closing picker did not restore the command draft")
	}
}

func inputLabels(in *Input) string {
	var labels []string
	for _, label := range in.Labels() {
		labels = append(labels, text.StripANSI(label.Text))
	}
	return strings.Join(labels, "\n")
}

func TestInputRenderingHonorsAllocation(t *testing.T) {
	for _, mode := range []string{"normal", "draft", "inline", "modal", "search"} {
		t.Run(mode, func(t *testing.T) {
			in := newTestInput(80)
			in.SetValue("say 世界")
			if mode != "normal" {
				in.OpenDraftEditor("say 世界\nsay e\u0301", 0)
			}
			switch mode {
			case "inline", "modal":
				in.ShowPicker(ui.ShowPickerMsg{Inline: mode == "inline", Items: []ui.PickerItem{{Text: "世界"}}})
			case "search":
				in.ShowSearch("世界", SearchScope{})
			}
			for _, width := range []int{-1, 0, 1, 2, 3, 10, 80} {
				for _, height := range []int{-1, 0, 1, 2, 3, 10} {
					t.Run(fmt.Sprintf("%dx%d", width, height), func(t *testing.T) {
						in.SetSize(width, height)
						geometry := in.SeparatorRow(width, height)
						labels := in.Labels()
						if width <= 0 || height <= 0 {
							if view := in.View(); view != "" || geometry != -1 || len(labels) != 0 {
								t.Fatalf("unallocated input drew content=%q, rules=%v, labels=%v", view, geometry, labels)
							}
						} else if rows := len(strings.Split(in.View(), "\n")); rows != height {
							t.Fatalf("rendered %d rows, want allocated height %d", rows, height)
						}
						if (mode == "normal" || mode == "modal" || mode == "search" || width <= 0 || height <= 0) && len(labels) != 0 {
							t.Fatalf("inactive or unallocated draft has labels: %+v", labels)
						}
						for _, label := range labels {
							onRule := label.Position.Y == -1 || label.Position.Y == height ||
								(geometry >= 0 && label.Position.Y == geometry)
							if !onRule || label.Position.X < 0 || label.Position.X+lipgloss.Width(label.Text) > width {
								t.Fatalf("label %+v is outside its border or separator", label)
							}
						}
					})
				}
			}
		})
	}
}

func TestInputLabelsUseCurrentDraftAfterMeasurement(t *testing.T) {
	in := newTestInput(80)
	in.OpenDraftEditor("first\nsecond", 0)
	in.SetSize(80, in.MeasureHeight(80, 100))
	in.SeparatorRow(10, 3)
	if labels := inputLabels(in); !strings.Contains(labels, "2 lines") {
		t.Fatalf("provisional measurement affected final labels: %q", labels)
	}
	in.SetValue("first\nsecond\n世界")
	in.SeparatorRow(3, 1)
	if labels := inputLabels(in); !strings.Contains(labels, "3 lines") {
		t.Fatalf("draft edit left stale labels: %q", labels)
	}
}

func TestDraftEditorMeasurementAndViewDoNotChangeNavigation(t *testing.T) {
	in := newTestInput(40)
	in.SetValue(strings.Repeat("a\nb\n", 12))
	in.SetSize(40, 7)
	before := in.draftEditor.topRow
	in.MeasureHeight(5, 24)
	in.SeparatorRow(5, 24)
	in.Labels()
	in.View()
	if in.draftEditor.topRow != before || in.width != 40 || in.height != 7 {
		t.Fatal("measurement or rendering changed applied geometry")
	}
	in.draftEditor.SetCursor(0)
	in.View()
	if in.draftEditor.topRow != before {
		t.Fatal("View applied a navigation change")
	}
	in.SetSize(40, 7)
	if in.draftEditor.topRow != 0 {
		t.Fatal("SetSize did not bring the cursor into view")
	}
}

func TestSearchKeepsCursorVisibleAtNarrowWidths(t *testing.T) {
	for _, width := range []int{1, 3, 4, 5, 10, 40} {
		in := newTestInput(width)
		in.search.Open(strings.Repeat("query", 20), SearchScope{})
		in.overlay = overlaySearch
		in.SetSize(width, in.MeasureHeight(in.width, 1<<14))
		view := text.StripANSI(in.View())
		if !strings.Contains(view, "█") {
			t.Errorf("width %d lost search cursor: %q", width, view)
		}
		for _, row := range strings.Split(view, "\n") {
			if lipgloss.Width(row) > width {
				t.Errorf("width %d overflows: %q", width, row)
			}
		}
	}
}

func TestInputViewContainsOnlyEditableContent(t *testing.T) {
	in := newTestInput(40)
	in.SetValue("kill goblin")

	rows := strings.Split(in.View(), "\n")
	if len(rows) != 1 {
		t.Fatalf("expected one content row, got %d rows: %q", len(rows), rows)
	}
	if !strings.Contains(rows[0], "kill goblin") {
		t.Errorf("input row should show the typed text, got %q", rows[0])
	}
	if in.MeasureHeight(in.width, 1<<14) != 1 {
		t.Errorf("PreferredHeight = %d, want 1", in.MeasureHeight(in.width, 1<<14))
	}
}

func TestInputValueAndCursorRoundTrip(t *testing.T) {
	in := newTestInput(40)

	in.SetValue("hello world")
	if in.Value() != "hello world" {
		t.Errorf("Value = %q", in.Value())
	}
	in.SetCursor(5)
	if in.Position() != 5 {
		t.Errorf("Position = %d, want 5", in.Position())
	}
	in.CursorEnd()
	if in.Position() != len("hello world") {
		t.Errorf("CursorEnd position = %d, want %d", in.Position(), len("hello world"))
	}
	in.Reset()
	if in.Value() != "" {
		t.Errorf("Reset should clear, got %q", in.Value())
	}
}

func TestInputSetCursorReleasesWholeLineSelection(t *testing.T) {
	in := newTestInput(40)
	in.SetValue("north")
	in.SelectAll()

	in.SetCursor(2)

	if in.Selected() {
		t.Fatal("moving the cursor left the whole line selected")
	}
	if got := in.Value(); got != "north" {
		t.Fatalf("moving the cursor changed input to %q", got)
	}
}

func TestInputSelectionReplacementUsesExactDeleteChords(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyPressMsg
		want string
	}{
		{name: "backspace", key: tea.KeyPressMsg{Code: tea.KeyBackspace}, want: ""},
		{name: "delete", key: tea.KeyPressMsg{Code: tea.KeyDelete}, want: ""},
		{name: "shift backspace", key: tea.KeyPressMsg{Code: tea.KeyBackspace, Mod: tea.ModShift}, want: ""},
		{name: "alt backspace", key: tea.KeyPressMsg{Code: tea.KeyBackspace, Mod: tea.ModAlt}, want: "north "},
		{name: "shift delete", key: tea.KeyPressMsg{Code: tea.KeyDelete, Mod: tea.ModShift}, want: "north east"},
		{name: "alt delete", key: tea.KeyPressMsg{Code: tea.KeyDelete, Mod: tea.ModAlt}, want: "north east"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := newTestInput(40)
			in.SetValue("north east")
			in.SelectAll()

			in.UpdateTextInput(tt.key)

			if got := in.Value(); got != tt.want {
				t.Fatalf("input value = %q, want %q", got, tt.want)
			}
			if in.Selected() {
				t.Fatal("editing key left the input selected")
			}
		})
	}
}

func TestInputSelectionStylesTextWithoutFillingRow(t *testing.T) {
	in := newTestInput(40)
	in.SetValue("north")
	in.SelectAll()

	rows := strings.Split(in.View(), "\n")
	if len(rows) != 1 {
		t.Fatalf("selected input rows = %d, want 1", len(rows))
	}
	selectedText := in.styles.InputSelected.Inline(true).Render("north")
	promptStyle := in.textinput.Styles().Blurred.Prompt
	prefix := promptStyle.Render(in.textinput.Prompt) + selectedText
	padding := in.width - lipgloss.Width(prefix)
	if padding < 0 {
		t.Fatalf("selected input prefix width = %d, exceeds row width %d",
			lipgloss.Width(prefix), in.width)
	}
	want := prefix + strings.Repeat(" ", padding)
	if rows[0] != want {
		t.Fatalf("selected input row = %q, want exact text and plain padding %q",
			rows[0], want)
	}
	if got := lipgloss.Width(rows[0]); got != in.width {
		t.Fatalf("selected input row width = %d, want %d", got, in.width)
	}
}

func TestInputPickerOverlayGrowsView(t *testing.T) {
	in := newTestInput(40)
	items := []ui.PickerItem{
		{Text: "midgaard", Value: "midgaard"},
		{Text: "arctic", Value: "arctic"},
	}

	in.ShowPicker(ui.ShowPickerMsg{Title: "Worlds", Items: items})
	in.SetSize(in.width, in.MeasureHeight(in.width, ui.MaxLayoutCells))
	if in.MeasureHeight(in.width, 1<<14) <= 3 {
		t.Error("active picker must add to the preferred height")
	}
	view := text.StripANSI(in.View())
	if !strings.Contains(view, "midgaard") || !strings.Contains(view, "arctic") {
		t.Errorf("picker overlay should list items, got %q", view)
	}
	if !strings.Contains(view, "Worlds") {
		t.Errorf("modal picker should show its title, got %q", view)
	}

	in.HidePicker()
	in.SetSize(in.width, in.MeasureHeight(in.width, ui.MaxLayoutCells))
	if in.MeasureHeight(in.width, 1<<14) != 1 {
		t.Errorf("PreferredHeight after hide = %d, want 1", in.MeasureHeight(in.width, 1<<14))
	}
	if strings.Contains(text.StripANSI(in.View()), "midgaard") {
		t.Error("hidden picker must not render")
	}
}

func TestConstrainedPickerKeepsEditableInputVisible(t *testing.T) {
	in := newTestInput(20)
	in.SetValue("nor")
	in.ShowPicker(ui.ShowPickerMsg{
		Inline: true,
		Items:  []ui.PickerItem{{Text: "north", Value: "north"}},
	})

	for _, height := range []int{1, 2} {
		in.SetSize(20, height)
		rows := strings.Split(text.StripANSI(in.View()), "\n")
		if len(rows) != height {
			t.Fatalf("height %d rendered %d rows: %q", height, len(rows), rows)
		}
		if !strings.Contains(rows[len(rows)-1], "> nor") {
			t.Fatalf("height %d hid editable input behind picker: %q", height, rows)
		}
		if height == 2 && !strings.Contains(rows[0], "north") {
			t.Fatalf("spare row should show selected completion: %q", rows)
		}
	}
}

func TestConstrainedModalPickerPreservesFocus(t *testing.T) {
	for _, title := range []string{"", "Aliases"} {
		in := newTestInput(30)
		items := make([]ui.PickerItem, 10)
		for n := range items {
			items[n].Text = strings.Repeat("x", n+1)
		}
		in.ShowPicker(ui.ShowPickerMsg{Title: title, Items: items})
		in.Picker().SelectUp() // wrap to the last result, outside the initial window
		for _, height := range []int{1, 2, 3, 5, 8} {
			in.SetSize(30, height)
			rows := strings.Split(text.StripANSI(in.View()), "\n")
			if len(rows) != height || !strings.Contains(strings.Join(rows, "\n"), "█") {
				t.Fatalf("height %d lost focused query: %q", height, rows)
			}
			if height >= 2 && !strings.Contains(strings.Join(rows, "\n"), "xxxxxxxxxx") {
				t.Fatalf("height %d lost selected result: %q", height, rows)
			}
		}
	}
}

func TestConstrainedSearchKeepsActiveQueryVisible(t *testing.T) {
	in := newTestInput(24)
	in.ShowSearch("thief", SearchScope{})

	for _, height := range []int{1, 2} {
		in.SetSize(24, height)
		rows := strings.Split(text.StripANSI(in.View()), "\n")
		if len(rows) != height {
			t.Fatalf("height %d rendered %d rows: %q", height, len(rows), rows)
		}
		if !strings.Contains(rows[len(rows)-1], "Search: thief") {
			t.Fatalf("height %d hid active search query: %q", height, rows)
		}
	}
}

func TestConstrainedDraftEditorKeepsEditableBodyVisible(t *testing.T) {
	in := newTestInput(24)
	in.OpenDraftEditor("say north\nsay south", 4)
	in.SetSize(24, 1)

	rows := strings.Split(text.StripANSI(in.View()), "\n")
	if len(rows) != 1 || !strings.Contains(rows[0], "say north") {
		t.Fatalf("one-row draft editor hid editable body: %q", rows)
	}
}

func TestInputInlinePickerSeedsFilterFromInput(t *testing.T) {
	in := newTestInput(40)
	items := []ui.PickerItem{
		{Text: "connect", Value: "connect"},
		{Text: "disconnect", Value: "disconnect"},
		{Text: "reload", Value: "reload"},
	}

	in.SetValue("rel")
	in.ShowPicker(ui.ShowPickerMsg{Items: items, Inline: true})
	in.SetSize(in.width, in.MeasureHeight(in.width, ui.MaxLayoutCells))

	if got := in.Picker().Query(); got != "rel" {
		t.Errorf("inline picker query = %q, want %q", got, "rel")
	}
	sel, ok := in.Picker().Selected()
	if !ok || sel.Text != "reload" {
		t.Errorf("inline selection = %v (%v), want reload", sel, ok)
	}
	if view := text.StripANSI(in.View()); strings.Contains(view, "disconnect") {
		t.Errorf("non-matching items should be filtered out, got %q", view)
	}

	// Typing more re-filters from the input value.
	in.SetValue("re")
	in.Picker().Filter(in.Value())
	in.SetSize(in.width, in.MeasureHeight(in.width, ui.MaxLayoutCells))
	view := text.StripANSI(in.View())
	if !strings.Contains(view, "reload") {
		t.Errorf("re-filtered view should keep matches, got %q", view)
	}
}

func TestInputSearchReplacesInactiveCommandField(t *testing.T) {
	buf := newTestBuffer("a thief passes")
	styles := style.DefaultStyles()
	search := NewSearch(buf, styles)
	in := NewInput(styles, search)
	in.SetValue("COMMAND-DRAFT")
	in.ShowSearch("thief", SearchScope{})
	in.SetSize(60, in.MeasureHeight(60, ui.MaxLayoutCells))

	view := text.StripANSI(in.View())
	if !strings.Contains(view, "Search:") || !strings.Contains(view, "a thief passes") {
		t.Fatalf("search navigator missing from input view:\n%s", view)
	}
	if strings.Contains(view, "COMMAND-DRAFT") {
		t.Fatalf("inactive command field remained visible during search:\n%s", view)
	}
	if got, want := in.MeasureHeight(in.width, 1<<14), 4; got != want {
		t.Fatalf("PreferredHeight = %d, want search-only height %d", got, want)
	}
	rows := strings.Split(view, "\n")
	if !strings.HasPrefix(rows[len(rows)-1], "Search: thief") || strings.Count(view, "█") != 1 {
		t.Fatalf("search must have one bottom query field: %q", rows)
	}
	if !strings.Contains(view, "↑ older") || !strings.Contains(view, "1/1") {
		t.Fatalf("search lost help or match count: %q", rows)
	}

	in.HideSearch()
	in.SetSize(in.width, in.MeasureHeight(in.width, ui.MaxLayoutCells))
	if view := text.StripANSI(in.View()); !strings.Contains(view, "COMMAND-DRAFT") {
		t.Fatalf("command draft did not return after search closed:\n%s", view)
	}
}
