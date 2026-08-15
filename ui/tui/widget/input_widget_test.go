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
	if in.PreferredHeight() != 3 {
		t.Errorf("PreferredHeight = %d, want 3", in.PreferredHeight())
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
	if in.PreferredHeight() <= 3 {
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
	if in.PreferredHeight() != 3 {
		t.Errorf("PreferredHeight after hide = %d, want 3", in.PreferredHeight())
	}
	if strings.Contains(text.StripANSI(in.View()), "midgaard") {
		t.Error("hidden picker must not render")
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

	if got := in.PickerQuery(); got != "rel" {
		t.Errorf("inline picker query = %q, want %q", got, "rel")
	}
	sel, ok := in.PickerSelected()
	if !ok || sel.Text != "reload" {
		t.Errorf("inline selection = %v (%v), want reload", sel, ok)
	}
	if view := text.StripANSI(in.View()); strings.Contains(view, "disconnect") {
		t.Errorf("non-matching items should be filtered out, got %q", view)
	}

	// Typing more re-filters from the input value.
	in.SetValue("re")
	in.UpdatePickerFilter()
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
	if got, want := in.PreferredHeight(), search.PreferredHeight(); got != want {
		t.Fatalf("PreferredHeight = %d, want search-only height %d", got, want)
	}

	in.HideSearch()
	if view := text.StripANSI(in.View()); !strings.Contains(view, "COMMAND-DRAFT") {
		t.Fatalf("command draft did not return after search closed:\n%s", view)
	}
}
