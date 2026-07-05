package widget

import (
	"strings"
	"testing"

	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/style"
)

func newTestInput(width int) *Input {
	in := NewInput(style.DefaultStyles())
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
