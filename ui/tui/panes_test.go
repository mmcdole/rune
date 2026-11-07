package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/style"
	"github.com/mmcdole/rune/ui/tui/widget"
)

func TestPaneLookupPreservesOutputAndContent(t *testing.T) {
	m := NewModel(nil)
	output := m.output

	reserved, ok := m.panes[ui.OutputPaneName]
	if !ok || reserved != output {
		t.Fatalf("reserved output = (%v, %v), want original Output", reserved, ok)
	}
	if created := m.pane(ui.OutputPaneName); created != output {
		t.Fatal("creating output replaced the Output widget")
	}

	chat := m.pane("chat")
	if again := m.pane("chat"); again != chat {
		t.Fatal("re-creating a pane replaced the existing buffer")
	}
	if _, ok := m.panes["missing"]; ok {
		t.Fatal("unknown pane exists without create or write")
	}

	written := m.pane("log")
	written.Write("one\ntwo")
	written.SetSize(20, 2)
	if got := written.View(); got != "one\ntwo" {
		t.Fatalf("written pane view = %q, want two stored rows", got)
	}
}

func TestOutputRetainsLastPlacementGeometryWhileHidden(t *testing.T) {
	output := widget.NewOutput(100000, style.DefaultStyles())
	output.SetFallbackSize(80, 24)
	output.SetFallbackSize(100, 30)
	if output.Width() != 100 {
		t.Fatalf("unplaced fallback width = %d, want latest terminal width 100", output.Width())
	}

	output.SetSize(24, 6)
	output.SetFallbackSize(120, 40)
	if output.Width() != 24 {
		t.Fatalf("hidden output width = %d, want retained placement width 24", output.Width())
	}
	output.Write(strings.Repeat("x", 30))
	if got := output.MeasureHeight(output.Width(), 10); got != 2 {
		t.Fatalf("hidden write measures %d rows at retained width, want 2", got)
	}
}

func TestMouseWheelScrollsPaneUnderPointer(t *testing.T) {
	m := NewModel(make(chan ui.UIEvent, 1024))
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(updateConfigMsg{Mouse: true})
	m.Update(updateLayoutMsg{Root: ui.LayoutNode{Type: ui.LayoutTypeColumn, Children: []ui.LayoutNode{
		{Type: ui.LayoutTypeRow, Size: ui.Fraction(1), Dividers: true, Children: []ui.LayoutNode{
			{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Size: ui.Fraction(1), Border: ui.PaneBorderNone},
			{Type: ui.LayoutTypePane, Name: "chat", Size: ui.Cells(30), Border: ui.PaneBorderNone},
		}},
		{Type: ui.LayoutTypeInput, Size: ui.AutoSize()},
	}}})
	for i := range 100 {
		m.Update(printLineMsg(fmt.Sprintf("line %d", i)))
		m.Update(paneWriteMsg{Name: "chat", Text: fmt.Sprintf("chat %d", i)})
	}
	m.View()

	chat := m.panes["chat"]
	before := chat.View()
	m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp, X: 70, Y: 5})
	if chat.View() == before {
		t.Fatal("a wheel tick over the chat pane should scroll it")
	}
	if m.output.Mode() != widget.ModeLive {
		t.Fatal("a wheel tick over the chat pane should leave the output live")
	}

	scrolled := chat.View()
	m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp, X: 5, Y: 5})
	if m.output.Mode() == widget.ModeLive {
		t.Fatal("a wheel tick over the output should scroll the output")
	}
	if chat.View() != scrolled {
		t.Fatal("a wheel tick over the output should leave the chat pane alone")
	}

	m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: 70, Y: 5})
	if chat.View() == scrolled {
		t.Fatal("wheel down over the chat pane should scroll it forward")
	}
}
