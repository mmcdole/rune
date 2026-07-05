package widget

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/style"
)

func newTestPicker(maxVisible int, texts ...string) *Picker {
	p := NewPicker(PickerConfig{MaxVisible: maxVisible}, style.DefaultStyles())
	items := make([]ui.PickerItem, len(texts))
	for i, txt := range texts {
		items[i] = ui.PickerItem{Text: txt, Value: txt}
	}
	p.SetItems(items)
	p.SetWidth(60)
	return p
}

func TestPickerFilterNarrowsMatches(t *testing.T) {
	p := newTestPicker(10, "apple", "banana", "cherry")

	p.Filter("ban")
	sel, ok := p.Selected()
	if !ok || sel.Text != "banana" {
		t.Fatalf("Selected = %v (%v), want banana", sel, ok)
	}
	view := p.View()
	if strings.Contains(view, "apple") || strings.Contains(view, "cherry") {
		t.Errorf("filtered view should only show matches, got %q", view)
	}

	// Clearing the query restores every item.
	p.Filter("")
	view = p.View()
	for _, want := range []string{"apple", "banana", "cherry"} {
		if !strings.Contains(view, want) {
			t.Errorf("unfiltered view missing %q", want)
		}
	}
}

func TestPickerNoMatchesShowsEmptyText(t *testing.T) {
	p := newTestPicker(10, "apple", "banana")

	p.Filter("zzz")
	if _, ok := p.Selected(); ok {
		t.Error("Selected should report no item when nothing matches")
	}
	if view := p.View(); !strings.Contains(view, "No matches") {
		t.Errorf("empty view should show placeholder, got %q", view)
	}
}

func TestPickerSelectionWrapsBothWays(t *testing.T) {
	p := newTestPicker(10, "one", "two", "three")

	p.SelectUp() // from 0 wraps to the end
	if sel, _ := p.Selected(); sel.Text != "three" {
		t.Errorf("SelectUp from top = %q, want three", sel.Text)
	}
	p.SelectDown() // from the end wraps back to 0
	if sel, _ := p.Selected(); sel.Text != "one" {
		t.Errorf("SelectDown from bottom = %q, want one", sel.Text)
	}
}

func TestPickerScrollWindowFollowsSelection(t *testing.T) {
	var texts []string
	for i := 1; i <= 8; i++ {
		texts = append(texts, fmt.Sprintf("item%02d", i))
	}
	p := newTestPicker(3, texts...)

	for i := 0; i < 5; i++ {
		p.SelectDown()
	}
	// Selection is item06; the 3-row window must have scrolled to it.
	view := p.View()
	if !strings.Contains(view, "item06") {
		t.Errorf("window should follow the selection, got %q", view)
	}
	if strings.Contains(view, "item01") {
		t.Errorf("scrolled-past items should leave the window, got %q", view)
	}
	if !strings.Contains(view, "> ") {
		t.Errorf("selected row should be marked, got %q", view)
	}
}

func TestPickerFilterClampsSelection(t *testing.T) {
	p := newTestPicker(10, "alpha", "beta", "gamma")
	p.SelectDown()
	p.SelectDown() // on gamma

	p.Filter("alp") // one match; old index 2 is out of range
	sel, ok := p.Selected()
	if !ok || sel.Text != "alpha" {
		t.Fatalf("Selected after narrowing = %v (%v), want alpha", sel, ok)
	}
}

func TestPickerPreferredHeight(t *testing.T) {
	p := newTestPicker(5, "a", "b", "c")
	// 3 items + 2 border rows.
	if got := p.PreferredHeight(); got != 5 {
		t.Errorf("PreferredHeight = %d, want 5", got)
	}

	p.Filter("zzz") // empty: placeholder row + border
	if got := p.PreferredHeight(); got != 3 {
		t.Errorf("PreferredHeight when empty = %d, want 3", got)
	}

	p.SetHeader("Pick: ")
	p.Filter("")
	// 3 items + header + border.
	if got := p.PreferredHeight(); got != 6 {
		t.Errorf("PreferredHeight with header = %d, want 6", got)
	}
}
