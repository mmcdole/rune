package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/widget"
)

func TestTerminalStateIsDeclaredOnInitialView(t *testing.T) {
	m := NewModel(make(chan ui.UIEvent, 1))
	if cmd := m.Init(); cmd != nil {
		t.Fatal("Init returned an imperative terminal-state command")
	}

	view := m.View()
	if view.Content != "Loading..." {
		t.Fatalf("initial content = %q, want Loading...", view.Content)
	}
	if !view.AltScreen {
		t.Fatal("initial view does not request the alternate screen")
	}
	if view.MouseMode != tea.MouseModeNone {
		t.Fatalf("initial mouse mode = %v, want none", view.MouseMode)
	}
}

func TestMouseModeFollowsRuntimeConfig(t *testing.T) {
	m := NewModel(make(chan ui.UIEvent, 1))

	next, _ := m.Update(ui.UpdateConfigMsg{Mouse: true})
	m = next.(*Model)
	view := m.View()
	if !view.AltScreen {
		t.Fatal("mouse-enabled view does not request the alternate screen")
	}
	if view.MouseMode != tea.MouseModeCellMotion {
		t.Fatalf("enabled mouse mode = %v, want cell motion", view.MouseMode)
	}

	next, _ = m.Update(ui.UpdateConfigMsg{Mouse: false})
	m = next.(*Model)
	view = m.View()
	if !view.AltScreen {
		t.Fatal("mouse-disabled view does not request the alternate screen")
	}
	if view.MouseMode != tea.MouseModeNone {
		t.Fatalf("disabled mouse mode = %v, want none", view.MouseMode)
	}
}

// TestKeyboardEnhancementsFollowNumpadConfig verifies the numpad setting
// requests the kitty keyboard flags that make NumLock-on keypad digits
// distinguishable from the number row, and releases them when turned off.
func TestKeyboardEnhancementsFollowNumpadConfig(t *testing.T) {
	m := NewModel(make(chan ui.UIEvent, 1))
	if ke := m.View().KeyboardEnhancements; ke.ReportAllKeysAsEscapeCodes || ke.ReportAssociatedText {
		t.Fatal("numpad-off view enables enhanced key mode")
	}

	next, _ := m.Update(ui.UpdateConfigMsg{Numpad: true})
	m = next.(*Model)
	ke := m.View().KeyboardEnhancements
	if !ke.ReportAllKeysAsEscapeCodes || !ke.ReportAssociatedText {
		t.Fatalf("numpad-on view enhancements = %+v, want all keys as escape codes with associated text", ke)
	}

	next, _ = m.Update(ui.UpdateConfigMsg{Numpad: false})
	m = next.(*Model)
	if ke := m.View().KeyboardEnhancements; ke.ReportAllKeysAsEscapeCodes || ke.ReportAssociatedText {
		t.Fatal("numpad-off view still enables enhanced key mode")
	}
}

// TestMouseWheelScrollsViewport verifies wheel events scroll the output
// viewport - the reason the terminal mouse is captured at all.
func TestMouseWheelScrollsViewport(t *testing.T) {
	m := newTestModel(t)
	next, _ := m.Update(ui.UpdateConfigMsg{Mouse: true})
	m = next.(*Model)

	if m.output.viewport.Mode() != widget.ModeLive {
		t.Fatal("expected viewport to start at bottom")
	}
	liveBottom := m.output.viewport.SaveScroll().BottomSeq

	wheelUp := tea.MouseWheelMsg{Button: tea.MouseWheelUp}
	next, _ = m.Update(wheelUp)
	m = next.(*Model)

	if m.output.viewport.Mode() == widget.ModeLive {
		t.Fatal("wheel up did not scroll the viewport")
	}
	if got := liveBottom - m.output.viewport.SaveScroll().BottomSeq; got != wheelScrollLines {
		t.Fatalf("wheel up scrolled %d lines, want %d", got, wheelScrollLines)
	}

	// Wheel down returns toward the bottom
	wheelDown := tea.MouseWheelMsg{Button: tea.MouseWheelDown}
	next, _ = m.Update(wheelDown)
	m = next.(*Model)

	if m.output.viewport.Mode() != widget.ModeLive {
		t.Fatal("wheel down did not scroll back to bottom")
	}
}

// TestMouseNonWheelEventsIgnored verifies clicks and motion do not
// disturb the viewport.
func TestMouseNonWheelEventsIgnored(t *testing.T) {
	m := newTestModel(t)
	next, _ := m.Update(ui.UpdateConfigMsg{Mouse: true})
	m = next.(*Model)

	click := tea.MouseClickMsg{Button: tea.MouseLeft}
	next, _ = m.Update(click)
	m = next.(*Model)

	if m.output.viewport.Mode() != widget.ModeLive {
		t.Fatal("non-wheel mouse event moved the viewport")
	}
}
