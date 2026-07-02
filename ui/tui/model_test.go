package tui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/widget"
)

// newTestModel builds a model with a sized window and enough
// scrollback to scroll.
func newTestModel(t *testing.T) Model {
	t.Helper()

	inputChan := make(chan string, 16)
	outbound := make(chan ui.UIEvent, 64)
	m := NewModel(inputChan, outbound)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(Model)

	// EchoLineMsg appends to the scrollback immediately (PrintLineMsg
	// batches until the next render tick).
	for i := 0; i < 100; i++ {
		next, _ = m.Update(ui.EchoLineMsg(fmt.Sprintf("line %d", i)))
		m = next.(Model)
	}
	return m
}

// TestMouseWheelScrollsViewport verifies wheel events scroll the main
// viewport - the reason the terminal mouse is captured at all.
func TestMouseWheelScrollsViewport(t *testing.T) {
	m := newTestModel(t)

	if m.viewport.Mode() != widget.ModeLive {
		t.Fatal("expected viewport to start at bottom")
	}

	wheelUp := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp}
	next, _ := m.Update(wheelUp)
	m = next.(Model)

	if m.viewport.Mode() == widget.ModeLive {
		t.Fatal("wheel up did not scroll the viewport")
	}

	// Wheel down returns toward the bottom
	wheelDown := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown}
	next, _ = m.Update(wheelDown)
	m = next.(Model)

	if m.viewport.Mode() != widget.ModeLive {
		t.Fatal("wheel down did not scroll back to bottom")
	}
}

// TestMouseNonWheelEventsIgnored verifies clicks and motion do not
// disturb the viewport.
func TestMouseNonWheelEventsIgnored(t *testing.T) {
	m := newTestModel(t)

	click := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	next, _ := m.Update(click)
	m = next.(Model)

	if m.viewport.Mode() != widget.ModeLive {
		t.Fatal("non-wheel mouse event moved the viewport")
	}
}
