package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/mmcdole/rune/ui"
)

// newInlinePickerModel builds a model with an inline picker open over a
// command-style item list and the input seeded with text, returning the
// event channel so tests can observe picker cancel messages.
func newInlinePickerModel(t *testing.T, dismissOnSpace bool, initial string) (*Model, chan ui.UIEvent) {
	t.Helper()

	events := make(chan ui.UIEvent, 64)
	m := NewModel(events)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(*Model)

	next, _ = m.Update(ui.ShowPickerMsg{
		Items: []ui.PickerItem{
			{Text: "/connect", Value: "/connect"},
			{Text: "/disconnect", Value: "/disconnect"},
		},
		CallbackID:     "cb1",
		Inline:         true,
		DismissOnSpace: dismissOnSpace,
	})
	m = next.(*Model)

	next, _ = m.Update(ui.SetInputMsg(initial))
	m = next.(*Model)

	if m.inputCtl.mode() != modePickerInline {
		t.Fatalf("expected inline picker mode after setup, got %v", m.inputCtl.mode())
	}
	drainPickerCancels(events) // discard setup noise
	return m, events
}

// drainPickerCancels empties the event channel and returns any
// picker cancellation messages (Accepted == false) it contained.
func drainPickerCancels(events chan ui.UIEvent) []ui.PickerSelectMsg {
	var cancels []ui.PickerSelectMsg
	for {
		select {
		case ev := <-events:
			if sel, ok := ev.(ui.PickerSelectMsg); ok && !sel.Accepted {
				cancels = append(cancels, sel)
			}
		default:
			return cancels
		}
	}
}

// TestInlinePickerDismissesOnSpace verifies that a dismiss_on_space picker
// resets its mode and cancels its callback when an argument separator is typed.
func TestInlinePickerDismissesOnSpace(t *testing.T) {
	m, events := newInlinePickerModel(t, true, "/connect")

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	m = next.(*Model)

	if m.inputCtl.mode() != modeNormal {
		t.Fatalf("expected picker to dismiss on space, mode = %v", m.inputCtl.mode())
	}
	cancels := drainPickerCancels(events)
	if len(cancels) != 1 || cancels[0].CallbackID != "cb1" {
		t.Fatalf("expected one cancel for cb1, got %v", cancels)
	}
	if got := m.input.Value(); got != "/connect " {
		t.Fatalf("expected input to keep the typed space, got %q", got)
	}
}

// TestInlinePickerWithoutDismissOnSpaceKeepsFiltering verifies the
// space behavior is opt-in: a plain inline picker stays open.
func TestInlinePickerWithoutDismissOnSpaceKeepsFiltering(t *testing.T) {
	m, events := newInlinePickerModel(t, false, "/connect")

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	m = next.(*Model)

	if m.inputCtl.mode() != modePickerInline {
		t.Fatalf("expected picker to stay open, mode = %v", m.inputCtl.mode())
	}
	if cancels := drainPickerCancels(events); len(cancels) != 0 {
		t.Fatalf("expected no cancel, got %v", cancels)
	}
}

// TestInlinePickerNormalTypingKeepsFiltering verifies ordinary
// characters do not close the picker.
func TestInlinePickerNormalTypingKeepsFiltering(t *testing.T) {
	m, events := newInlinePickerModel(t, true, "/con")

	next, _ := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = next.(*Model)

	if m.inputCtl.mode() != modePickerInline {
		t.Fatalf("expected picker to stay open, mode = %v", m.inputCtl.mode())
	}
	if cancels := drainPickerCancels(events); len(cancels) != 0 {
		t.Fatalf("expected no cancel, got %v", cancels)
	}
}

// TestInlinePickerClosesCleanlyOnEmptiedInput verifies that emptying the input
// closes the picker, resets its mode, and cancels its Lua callback.
func TestInlinePickerClosesCleanlyOnEmptiedInput(t *testing.T) {
	m, events := newInlinePickerModel(t, true, "/")

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = next.(*Model)

	if m.input.Value() != "" {
		t.Fatalf("expected empty input after backspace, got %q", m.input.Value())
	}
	if m.inputCtl.mode() != modeNormal {
		t.Fatalf("expected mode reset after input emptied, mode = %v", m.inputCtl.mode())
	}
	cancels := drainPickerCancels(events)
	if len(cancels) != 1 || cancels[0].CallbackID != "cb1" {
		t.Fatalf("expected one cancel for cb1, got %v", cancels)
	}
}

// TestInlinePickerDismissesOnLuaEditWithSpace verifies Lua-driven input
// edits (rune.input.set) honor dismiss_on_space too.
func TestInlinePickerDismissesOnLuaEditWithSpace(t *testing.T) {
	m, events := newInlinePickerModel(t, true, "/connect")

	next, _ := m.Update(ui.SetInputMsg("/connect vikingmud.org 2001"))
	m = next.(*Model)

	if m.inputCtl.mode() != modeNormal {
		t.Fatalf("expected picker to dismiss, mode = %v", m.inputCtl.mode())
	}
	cancels := drainPickerCancels(events)
	if len(cancels) != 1 || cancels[0].CallbackID != "cb1" {
		t.Fatalf("expected one cancel for cb1, got %v", cancels)
	}
}
