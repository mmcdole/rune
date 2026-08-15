package widget

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

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

func TestInputSelectionStylesTextWithoutFillingRow(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })

	in := newTestInput(40)
	in.SetValue("north")
	in.SelectAll()

	rows := strings.Split(in.View(), "\n")
	if len(rows) != 3 {
		t.Fatalf("selected input rows = %d, want 3", len(rows))
	}
	selectedText := in.styles.InputSelected.Inline(true).Render("north")
	prefix := in.textinput.PromptStyle.Render(in.textinput.Prompt) + selectedText
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
	view := in.View()
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
	if strings.Contains(in.View(), "midgaard") {
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
	if view := in.View(); strings.Contains(view, "disconnect") {
		t.Errorf("non-matching items should be filtered out, got %q", view)
	}

	// Typing more re-filters from the input value.
	in.SetValue("re")
	in.UpdatePickerFilter()
	view := in.View()
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

	view := in.View()
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
	if view := in.View(); !strings.Contains(view, "COMMAND-DRAFT") {
		t.Fatalf("command draft did not return after search closed:\n%s", view)
	}
}
