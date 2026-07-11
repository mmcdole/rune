package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/widget"
)

// newTestModel builds a model with a sized window and enough
// scrollback to scroll.
func newTestModel(t *testing.T) *Model {
	t.Helper()

	inputChan := make(chan input.Submission, 16)
	outbound := make(chan ui.UIEvent, 64)
	m := NewModel(inputChan, outbound)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(*Model)

	// EchoLineMsg appends to the scrollback immediately and never
	// opens a batch window, so no tick bookkeeping is needed here.
	for i := 0; i < 100; i++ {
		next, _ = m.Update(ui.EchoLineMsg(fmt.Sprintf("line %d", i)))
		m = next.(*Model)
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
	m = next.(*Model)

	if m.viewport.Mode() == widget.ModeLive {
		t.Fatal("wheel up did not scroll the viewport")
	}

	// Wheel down returns toward the bottom
	wheelDown := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown}
	next, _ = m.Update(wheelDown)
	m = next.(*Model)

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
	m = next.(*Model)

	if m.viewport.Mode() != widget.ModeLive {
		t.Fatal("non-wheel mouse event moved the viewport")
	}
}

// newBareModel builds a sized model with an empty scrollback, for
// tests that assert on exact line counts and ordering.
func newBareModel(t *testing.T) *Model {
	t.Helper()

	inputChan := make(chan input.Submission, 16)
	outbound := make(chan ui.UIEvent, 64)
	m := NewModel(inputChan, outbound)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return next.(*Model)
}

// TestFirstLineRendersImmediately verifies the idle->hot transition: a
// server line arriving with no batch window open is appended right
// away (not parked until a tick) and opens a window for what follows.
func TestFirstLineRendersImmediately(t *testing.T) {
	m := newBareModel(t)

	next, cmd := m.Update(ui.PrintLineMsg("hello"))
	m = next.(*Model)

	if got := m.scrollback.Count(); got != 1 {
		t.Fatalf("expected first line appended immediately, scrollback has %d lines", got)
	}
	if cmd == nil {
		t.Fatal("expected first line to open a batch window (tick cmd)")
	}
}

// TestBurstCoalescesInBatchWindow verifies lines arriving inside an
// open batch window are held and flushed together on the tick.
func TestBurstCoalescesInBatchWindow(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.PrintLineMsg("line 1"))
	m = next.(*Model)
	next, _ = m.Update(ui.PrintLineMsg("line 2"))
	m = next.(*Model)
	next, _ = m.Update(ui.PrintLineMsg("line 3"))
	m = next.(*Model)

	if got := m.scrollback.Count(); got != 1 {
		t.Fatalf("expected burst lines batched, scrollback has %d lines", got)
	}

	next, _ = m.Update(tickMsg{})
	m = next.(*Model)

	if got := m.scrollback.Count(); got != 3 {
		t.Fatalf("expected tick to flush the batch, scrollback has %d lines", got)
	}
}

// TestTickStopsWhenOutputGoesQuiet is the no-perpetual-tick regression
// guard: a tick that flushed lines re-arms the window, and the first
// tick that finds nothing pending ends the chain, so an idle client
// has no standing timer.
func TestTickStopsWhenOutputGoesQuiet(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.PrintLineMsg("line 1"))
	m = next.(*Model)
	next, _ = m.Update(ui.PrintLineMsg("line 2"))
	m = next.(*Model)

	next, cmd := m.Update(tickMsg{})
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("expected tick with pending lines to re-arm the window")
	}

	next, cmd = m.Update(tickMsg{})
	m = next.(*Model)
	if cmd != nil {
		t.Fatal("expected tick with nothing pending to stop the chain")
	}
}

// TestEchoFlushesPendingServerLines verifies a local echo cannot render
// ahead of server output that arrived before it: batched PrintLineMsg
// lines must be flushed to the scrollback before the echo is appended,
// and the now-empty trailing tick must not re-arm.
func TestEchoFlushesPendingServerLines(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.PrintLineMsg("line 1")) // immediate, opens window
	m = next.(*Model)
	next, _ = m.Update(ui.PrintLineMsg("line 2")) // batched
	m = next.(*Model)
	next, _ = m.Update(ui.EchoLineMsg("> look"))
	m = next.(*Model)

	if got := m.scrollback.Count(); got != 3 {
		t.Fatalf("expected 3 scrollback lines, got %d", got)
	}
	for i, want := range []string{"line 1", "line 2", "> look"} {
		if got := m.scrollback.At(i); got != want {
			t.Fatalf("scrollback[%d] = %q, want %q (echo reordered?)", i, got, want)
		}
	}

	next, cmd := m.Update(tickMsg{})
	m = next.(*Model)
	if cmd != nil {
		t.Fatal("expected trailing tick after eager echo flush to stop the chain")
	}
	if got := m.scrollback.Count(); got != 3 {
		t.Fatalf("trailing tick changed scrollback, got %d lines", got)
	}
}

func TestEchoExpandsPreservedTabsBeforeScrollback(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.EchoLineMsg("> a\tb"))
	m = next.(*Model)

	got := m.scrollback.At(0)
	if strings.ContainsRune(got, '\t') {
		t.Fatalf("raw tab reached scrollback: %q", got)
	}
	if !strings.Contains(got, "b") || len(got) <= len("> a b") {
		t.Fatalf("tab was not expanded for display: %q", got)
	}
}

// TestBarCannotClobberBuiltinWidget verifies a Lua bar named after a
// built-in widget ("input", "separator") neither replaces it nor
// deletes it when the bar is later removed.
func TestBarCannotClobberBuiltinWidget(t *testing.T) {
	m := newTestModel(t)

	next, _ := m.Update(ui.UpdateBarsMsg{"input": {Left: "hijack"}})
	m = next.(*Model)

	if _, isInput := m.widgets["input"].(*widget.Input); !isInput {
		t.Fatal("bar named \"input\" replaced the input widget")
	}

	next, _ = m.Update(ui.UpdateBarsMsg{})
	m = next.(*Model)

	if _, isInput := m.widgets["input"].(*widget.Input); !isInput {
		t.Fatal("removing the colliding bar deleted the input widget")
	}
}

// newInlinePickerModel builds a model with an inline picker open over a
// command-style item list and the input seeded with text, returning the
// outbound channel so tests can observe picker cancel messages.
func newInlinePickerModel(t *testing.T, dismissOnSpace bool, initial string) (*Model, chan ui.UIEvent) {
	t.Helper()

	inputChan := make(chan input.Submission, 16)
	outbound := make(chan ui.UIEvent, 64)
	m := NewModel(inputChan, outbound)

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

	if m.inputCtl.mode != ModePickerInline {
		t.Fatalf("expected inline picker mode after setup, got %v", m.inputCtl.mode)
	}
	drainPickerCancels(outbound) // discard setup noise
	return m, outbound
}

// drainPickerCancels empties the outbound channel and returns any
// picker cancellation messages (Accepted == false) it contained.
func drainPickerCancels(outbound chan ui.UIEvent) []ui.PickerSelectMsg {
	var cancels []ui.PickerSelectMsg
	for {
		select {
		case ev := <-outbound:
			if sel, ok := ev.(ui.PickerSelectMsg); ok && !sel.Accepted {
				cancels = append(cancels, sel)
			}
		default:
			return cancels
		}
	}
}

// TestInlinePickerDismissesOnSpace verifies a dismiss_on_space picker
// closes (mode reset + callback cancelled) as soon as the user types a
// space to start arguments - the fix for issue #3.
func TestInlinePickerDismissesOnSpace(t *testing.T) {
	m, outbound := newInlinePickerModel(t, true, "/connect")

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	m = next.(*Model)

	if m.inputCtl.mode != ModeNormal {
		t.Fatalf("expected picker to dismiss on space, mode = %v", m.inputCtl.mode)
	}
	cancels := drainPickerCancels(outbound)
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
	m, outbound := newInlinePickerModel(t, false, "/connect")

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	m = next.(*Model)

	if m.inputCtl.mode != ModePickerInline {
		t.Fatalf("expected picker to stay open, mode = %v", m.inputCtl.mode)
	}
	if cancels := drainPickerCancels(outbound); len(cancels) != 0 {
		t.Fatalf("expected no cancel, got %v", cancels)
	}
}

// TestInlinePickerNormalTypingKeepsFiltering verifies ordinary
// characters do not close the picker.
func TestInlinePickerNormalTypingKeepsFiltering(t *testing.T) {
	m, outbound := newInlinePickerModel(t, true, "/con")

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(*Model)

	if m.inputCtl.mode != ModePickerInline {
		t.Fatalf("expected picker to stay open, mode = %v", m.inputCtl.mode)
	}
	if cancels := drainPickerCancels(outbound); len(cancels) != 0 {
		t.Fatalf("expected no cancel, got %v", cancels)
	}
}

// TestInlinePickerClosesCleanlyOnEmptiedInput is a regression test for
// the stuck-mode bug: backspacing the input to empty used to hide the
// picker at the widget level while leaving the model in
// ModePickerInline with the Lua callback never cancelled.
func TestInlinePickerClosesCleanlyOnEmptiedInput(t *testing.T) {
	m, outbound := newInlinePickerModel(t, true, "/")

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = next.(*Model)

	if m.input.Value() != "" {
		t.Fatalf("expected empty input after backspace, got %q", m.input.Value())
	}
	if m.inputCtl.mode != ModeNormal {
		t.Fatalf("expected mode reset after input emptied, mode = %v", m.inputCtl.mode)
	}
	cancels := drainPickerCancels(outbound)
	if len(cancels) != 1 || cancels[0].CallbackID != "cb1" {
		t.Fatalf("expected one cancel for cb1, got %v", cancels)
	}
}

// TestInlinePickerDismissesOnLuaEditWithSpace verifies Lua-driven input
// edits (rune.input.set) honor dismiss_on_space too.
func TestInlinePickerDismissesOnLuaEditWithSpace(t *testing.T) {
	m, outbound := newInlinePickerModel(t, true, "/connect")

	next, _ := m.Update(ui.SetInputMsg("/connect vikingmud.org 2001"))
	m = next.(*Model)

	if m.inputCtl.mode != ModeNormal {
		t.Fatalf("expected picker to dismiss, mode = %v", m.inputCtl.mode)
	}
	cancels := drainPickerCancels(outbound)
	if len(cancels) != 1 || cancels[0].CallbackID != "cb1" {
		t.Fatalf("expected one cancel for cb1, got %v", cancels)
	}
}

func TestSetInputSubmissionMessageForcesVerbatimMode(t *testing.T) {
	m := newBareModel(t)
	next, _ := m.Update(ui.SetInputSubmissionMsg(input.Verbatim("one line;still data")))
	m = next.(*Model)

	if m.inputCtl.mode != ModeCompose || !m.input.IsComposing() {
		t.Fatal("explicit verbatim message did not enter composer")
	}
	if got := m.input.Value(); got != "one line;still data" {
		t.Fatalf("input = %q", got)
	}
}

// Regression #16: raw tabs must never reach the renderer. Bubbletea
// repaints only changed rows; a row starting with \t makes the terminal
// skip cells without erasing them, resurrecting the previous frame
// (ghost columns). True paint verification is the manual tmux route -
// this pins the model-layer guarantee that scrollback rows are tab-free.
func TestPrintedTabsAreExpanded(t *testing.T) {
	m := newTestModel(t)
	next, _ := m.Update(ui.PrintLineMsg("\tDead-file cleanup"))
	m = next.(*Model)
	found := false
	for i := 0; i < m.scrollback.Count(); i++ {
		row := m.scrollback.At(i)
		if row == "        Dead-file cleanup" {
			found = true
		}
		if strings.Contains(row, "\t") {
			t.Errorf("raw tab reached scrollback row %d: %q", i, row)
		}
	}
	if !found {
		t.Errorf("expanded row not found in scrollback")
	}
	next, _ = m.Update(ui.PromptMsg("HP\t> "))
	m = next.(*Model)
	if got := m.lastPrompt; got != "HP      > " {
		t.Errorf("prompt = %q, want tab expanded", got)
	}
}
