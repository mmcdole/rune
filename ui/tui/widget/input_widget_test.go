package widget

import (
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
	in := NewInput(styles, NewSearch(NewScrollbackBuffer(100), styles))
	in.SetSize(width, 0)
	return in
}

func TestModalPickerShowsResultsAboveItsOnlyEditor(t *testing.T) {
	in := newTestInput(40)
	in.SetValue("unfinished command")
	in.ShowPicker(ui.ShowPickerMsg{Title: "Aliases", Items: []ui.PickerItem{{Text: "north"}, {Text: "south"}}})
	in.SetSize(40, in.MeasureHeight(in.width, 1<<14)+2)
	plan := in.layout(40, in.height)
	rows := strings.Split(text.StripANSI(in.View()), "\n")
	if len(rows) != in.height || !strings.Contains(rows[1], "north") || !strings.Contains(rows[in.height-2], "Aliases: █") {
		t.Fatalf("picker placement changed: %q", rows)
	}
	horizontal := make(map[int]bool)
	for _, rule := range in.Rules(40, in.height) {
		if rule.Vertical {
			t.Fatal("picker must not draw a separate side wall")
		}
		if !rule.Vertical && rule.From == 0 && rule.To == 40 {
			horizontal[rule.At] = true
		}
	}
	if len(horizontal) != 3 || !horizontal[0] || !horizontal[plan.pickerHeight] || !horizontal[in.height-1] {
		t.Fatalf("picker/field boundaries disagree with assigned geometry:\n%s", strings.Join(rows, "\n"))
	}
	if strings.Contains(in.View(), "─") {
		t.Fatal("content painted compositor-owned rules")
	}
	if strings.Contains(in.View(), "unfinished command") || strings.Count(in.View(), "█") != 1 {
		t.Fatal("modal picker exposed the inactive command editor")
	}
	in.HidePicker()
	if in.Value() != "unfinished command" || !strings.Contains(text.StripANSI(in.View()), "unfinished command") {
		t.Fatal("closing picker did not restore the command draft")
	}
}

func inputLabels(in *Input) string {
	var labels []string
	for _, rule := range in.Rules(in.width, in.MeasureHeight(in.width, 1<<14)) {
		labels = append(labels, rule.Label)
	}
	return strings.Join(labels, "\n")
}

func TestComposerMeasurementAndViewDoNotChangeNavigation(t *testing.T) {
	in := newTestInput(40)
	in.SetValue(strings.Repeat("a\nb\n", 12))
	in.SetSize(40, 7)
	before := in.composer.topRow
	in.MeasureHeight(5, 24)
	in.Rules(5, 24)
	in.View()
	if in.composer.topRow != before || in.width != 40 || in.height != 7 {
		t.Fatal("measurement or rendering changed applied geometry")
	}
	in.composer.SetCursor(0)
	in.View()
	if in.composer.topRow != before {
		t.Fatal("View applied a navigation change")
	}
	in.SetSize(40, 7)
	if in.composer.topRow != 0 {
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

func TestInputViewIsBorderedField(t *testing.T) {
	in := newTestInput(40)
	in.SetValue("kill goblin")

	rows := strings.Split(in.View(), "\n")
	if len(rows) != 3 {
		t.Fatalf("expected border/input/border, got %d rows: %q", len(rows), rows)
	}
	if !strings.Contains(rows[1], "kill goblin") {
		t.Errorf("input row should show the typed text, got %q", rows[1])
	}
	if in.MeasureHeight(in.width, 1<<14) != 3 {
		t.Errorf("PreferredHeight = %d, want 3", in.MeasureHeight(in.width, 1<<14))
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
		{name: "shift backspace", key: tea.KeyPressMsg{Code: tea.KeyBackspace, Mod: tea.ModShift}, want: "north east"},
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
	if len(rows) != 3 {
		t.Fatalf("selected input rows = %d, want 3", len(rows))
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
	if rows[1] != want {
		t.Fatalf("selected input row = %q, want exact text and plain padding %q",
			rows[1], want)
	}
	if got := lipgloss.Width(rows[1]); got != in.width {
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
	if in.MeasureHeight(in.width, 1<<14) != 3 {
		t.Errorf("PreferredHeight after hide = %d, want 3", in.MeasureHeight(in.width, 1<<14))
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

func TestConstrainedComposerKeepsEditableBodyVisible(t *testing.T) {
	in := newTestInput(24)
	in.BeginCompose("say north\nsay south", 4)
	in.SetSize(24, 1)

	rows := strings.Split(text.StripANSI(in.View()), "\n")
	if len(rows) != 1 || !strings.Contains(rows[0], "say north") {
		t.Fatalf("one-row composer hid editable body: %q", rows)
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
	in.SetSize(60, 0)
	in.SetValue("COMMAND-DRAFT")
	in.ShowSearch("thief", SearchScope{})

	view := text.StripANSI(in.View())
	if !strings.Contains(view, "Search:") || !strings.Contains(view, "a thief passes") {
		t.Fatalf("search navigator missing from input view:\n%s", view)
	}
	if strings.Contains(view, "COMMAND-DRAFT") {
		t.Fatalf("inactive command field remained visible during search:\n%s", view)
	}
	if got, want := in.MeasureHeight(in.width, 1<<14), 6; got != want {
		t.Fatalf("PreferredHeight = %d, want search-only height %d", got, want)
	}
	rows := strings.Split(view, "\n")
	if !strings.HasPrefix(rows[len(rows)-2], "Search: thief") || strings.Count(view, "█") != 1 {
		t.Fatalf("search must have one bottom editor: %q", rows)
	}
	if !strings.Contains(view, "↑ older") || !strings.Contains(view, "1/1") {
		t.Fatalf("search lost help or match count: %q", rows)
	}

	in.HideSearch()
	if view := text.StripANSI(in.View()); !strings.Contains(view, "COMMAND-DRAFT") {
		t.Fatalf("command draft did not return after search closed:\n%s", view)
	}
}
