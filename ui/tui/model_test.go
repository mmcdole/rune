package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mmcdole/rune/input"
	runetext "github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/widget"
	"github.com/muesli/termenv"
)

// newTestModel builds a model with a sized window and enough
// scrollback to scroll.
func newTestModel(t *testing.T) *Model {
	t.Helper()

	events := make(chan ui.UIEvent, 256)
	m := NewModel(events)

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

	events := make(chan ui.UIEvent, 64)
	m := NewModel(events)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return next.(*Model)
}

func TestAcceptedSubmissionFollowsDraftChangeOnOneUIEventLane(t *testing.T) {
	events := make(chan ui.UIEvent, 4)
	m := NewModel(events)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("look")})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = next.(*Model)

	changed, ok := (<-events).(ui.InputChangedMsg)
	if !ok || changed.Text != "look" {
		t.Fatalf("first event = %#v, want draft change to look", changed)
	}
	submitted, ok := (<-events).(ui.SubmissionMsg)
	if !ok || submitted.Submission != input.Command("look") {
		t.Fatalf("second event = %#v, want command submission", submitted)
	}
	select {
	case event := <-events:
		t.Fatalf("accepted submission emitted redundant event %#v", event)
	default:
	}
}

func TestFullUIEventQueueRejectsSubmissionWithoutLosingDraft(t *testing.T) {
	events := make(chan ui.UIEvent, 1)
	m := NewModel(events)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("look")})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(*Model)

	if got := m.inputCtl.input.Value(); got != "look" {
		t.Fatalf("rejected submission changed draft to %q", got)
	}
	if got := m.scrollback.Count(); got != 1 {
		t.Fatalf("warning rows = %d, want exactly one", got)
	}
	if warning := runetext.StripANSI(m.scrollback.At(0)); !strings.Contains(warning, "Input not sent - engine lagging") {
		t.Fatalf("warning = %q", warning)
	}
	if _, ok := (<-events).(ui.InputChangedMsg); !ok {
		t.Fatal("queue no longer contains the accepted draft change")
	}
}

func TestFullUIEventQueueReportsDroppedOrdinaryEvent(t *testing.T) {
	events := make(chan ui.UIEvent, 1)
	events <- ui.InputChangedMsg{Text: "queued", Cursor: 6}
	m := NewModel(events)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(*Model)

	if got := m.scrollback.Count(); got != 1 {
		t.Fatalf("warning rows = %d, want exactly one", got)
	}
	if warning := runetext.StripANSI(m.scrollback.At(0)); !strings.Contains(warning, "UI event dropped - engine lagging") {
		t.Fatalf("warning = %q", warning)
	}
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

// TestTickStopsWhenOutputGoesQuiet verifies that a flush re-arms the batching
// window once, while the first tick with no pending lines ends the chain. An
// idle client must have no standing timer.
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

	_, cmd = m.Update(tickMsg{})
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

func TestPromptCommitPrecedesFollowingRows(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.PrintLineMsg("line 1")) // immediate, opens window
	m = next.(*Model)
	next, _ = m.Update(ui.PrintLineMsg("line 2")) // batched
	m = next.(*Model)
	next, _ = m.Update(ui.PromptMsg("Username:"))
	m = next.(*Model)
	next, _ = m.Update(ui.PromptCommitMsg("Username:"))
	m = next.(*Model)
	next, _ = m.Update(ui.EchoLineMsg("> player"))
	m = next.(*Model)
	next, _ = m.Update(ui.PrintLineMsg("login hook sent username"))
	m = next.(*Model)
	next, _ = m.Update(tickMsg{})
	m = next.(*Model)

	wantScrollback(t, m,
		"line 1", "line 2", "Username:", "> player", "login hook sent username")
	if m.promptText != "" {
		t.Fatalf("prompt overlay = %q after commit, want empty", m.promptText)
	}
}

func TestOrderedPromptCommitThenLocalSubmissionOutput(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.PromptMsg("HP>"))
	m = next.(*Model)
	next, _ = m.Update(ui.PromptCommitMsg("HP>"))
	m = next.(*Model)
	next, _ = m.Update(ui.EchoLineMsg("> /help"))
	m = next.(*Model)
	next, _ = m.Update(ui.PrintLineMsg("local help"))
	m = next.(*Model)

	wantScrollback(t, m, "HP>", "> /help", "local help")
	if got := m.promptText; got != "" {
		t.Fatalf("prompt overlay = %q after commit, want empty", got)
	}
}

func TestPromptClearClearsOverlay(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.PromptMsg("User"))
	m = next.(*Model)
	next, _ = m.Update(ui.PromptMsg("Username:"))
	m = next.(*Model)

	wantScrollback(t, m)
	if got := m.promptText; got != "Username:" {
		t.Fatalf("prompt overlay = %q, want %q", got, "Username:")
	}

	next, _ = m.Update(ui.PromptMsg(""))
	m = next.(*Model)

	wantScrollback(t, m)
	if m.promptText != "" {
		t.Fatalf("prompt overlay = %q after clear, want empty", m.promptText)
	}
}

func wantScrollback(t *testing.T, m *Model, want ...string) {
	t.Helper()
	if got := m.scrollback.Count(); got != len(want) {
		t.Fatalf("scrollback has %d rows, want %d", got, len(want))
	}
	for i, w := range want {
		if got := m.scrollback.At(i); got != w {
			t.Fatalf("scrollback[%d] = %q, want %q", i, got, w)
		}
	}
}

// TestMultiLinePrintSplitsIntoRows pins issue #49: a Print carrying
// embedded newlines must become one scrollback row per line, with
// lone CR and CRLF treated as line breaks.
func TestMultiLinePrintSplitsIntoRows(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.PrintLineMsg("row 1\rrow 2\r\nrow 3"))
	m = next.(*Model)

	wantScrollback(t, m, "row 1", "row 2", "row 3")
}

// TestMultiLinePrintSplitsInsideBatchWindow verifies the batched path
// splits too: a multi-line Print arriving inside an open window lands
// as individual rows when the tick flushes.
func TestMultiLinePrintSplitsInsideBatchWindow(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.PrintLineMsg("first")) // immediate, opens window
	m = next.(*Model)
	next, _ = m.Update(ui.PrintLineMsg("row 1\nrow 2")) // batched
	m = next.(*Model)
	next, _ = m.Update(tickMsg{})
	m = next.(*Model)

	wantScrollback(t, m, "first", "row 1", "row 2")
}

// TestOverlongPrintWordWrapsToWidth pins issue #49: a line wider than
// the terminal word-wraps into multiple rows at the last space rather
// than being clipped. The model is 80 columns wide (newBareModel).
func TestOverlongPrintWordWrapsToWidth(t *testing.T) {
	m := newBareModel(t)

	head := strings.Repeat("x", 60)
	tail := strings.Repeat("y", 30)
	next, _ := m.Update(ui.PrintLineMsg(head + " " + tail))
	m = next.(*Model)

	wantScrollback(t, m, head, tail)
}

// TestOverlongUnbreakableWordHardWraps verifies a single word wider
// than the terminal is broken at the width rather than clipped.
func TestOverlongUnbreakableWordHardWraps(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.EchoLineMsg(strings.Repeat("z", 100)))
	m = next.(*Model)

	wantScrollback(t, m, strings.Repeat("z", 80), strings.Repeat("z", 20))
}

// TestMultiLineEchoSplitsIntoRows verifies the echo path splits like
// Print, and that tab columns restart on each row rather than carrying
// across the whole message.
func TestMultiLineEchoSplitsIntoRows(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.EchoLineMsg("> dump\na\tb"))
	m = next.(*Model)

	wantScrollback(t, m, "> dump", "a       b")
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

func TestOversizedVerbatimSubmissionIsRejectedAtomically(t *testing.T) {
	m := newBareModel(t)

	tooManyLines := input.Verbatim(strings.Repeat("\n", maxVerbatimLines))
	if m.submit(tooManyLines) {
		t.Fatal("over-line-limit verbatim submission was accepted")
	}
	tooManyBytes := input.Verbatim(strings.Repeat("x", maxVerbatimBytes+1))
	if m.submit(tooManyBytes) {
		t.Fatal("over-byte-limit verbatim submission was accepted")
	}
	tooManyCRLines := input.Verbatim(strings.Repeat("\r", maxVerbatimLines))
	if m.submit(tooManyCRLines) {
		t.Fatal("over-line-limit bare-CR verbatim submission was accepted")
	}

	if got := m.scrollback.Count(); got != 3 {
		t.Fatalf("warning count = %d, want 3", got)
	}
	for n := 0; n < m.scrollback.Count(); n++ {
		if warning := m.scrollback.At(n); !strings.Contains(warning, "Verbatim input not sent") {
			t.Fatalf("warning %d = %q", n, warning)
		}
	}
}

func TestVerbatimSubmissionAtLimitsIsAccepted(t *testing.T) {
	events := make(chan ui.UIEvent, 1)
	m := NewModel(events)
	text := strings.Repeat("x", maxVerbatimBytes-(maxVerbatimLines-1)) +
		strings.Repeat("\n", maxVerbatimLines-1)
	submission := input.Verbatim(text)

	if len(text) != maxVerbatimBytes {
		t.Fatalf("test setup bytes = %d, want %d", len(text), maxVerbatimBytes)
	}
	if !m.submit(submission) {
		t.Fatal("at-limit verbatim submission was rejected")
	}
	got, ok := (<-events).(ui.SubmissionMsg)
	if !ok || got.Submission != submission {
		t.Fatalf("queued event = %#v, want submission %+v", got, submission)
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

// TestLayoutEntryOptsReachWidget verifies layoutDock hands each
// entry's option bag to Configurable widgets — and that a second
// separator entry without options resets the shared instance instead
// of inheriting the first entry's char.
func TestLayoutEntryOptsReachWidget(t *testing.T) {
	m := newTestModel(t)

	// No "input" entry: the input widget draws its own default rule,
	// which would mask a separator that failed to reset.
	next, _ := m.Update(ui.UpdateLayoutMsg{
		Top:    []ui.LayoutEntry{{Name: "separator", Opts: map[string]string{"char": "═"}}},
		Bottom: []ui.LayoutEntry{{Name: "separator"}},
	})
	m = next.(*Model)

	view := m.View()
	if !strings.Contains(view, strings.Repeat("═", m.width)) {
		t.Error("configured separator rule missing from view")
	}
	if !strings.Contains(view, strings.Repeat("─", m.width)) {
		t.Error("option-less separator entry did not reset to the default rule")
	}
}

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

	if m.inputCtl.mode != ModePickerInline {
		t.Fatalf("expected inline picker mode after setup, got %v", m.inputCtl.mode)
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

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	m = next.(*Model)

	if m.inputCtl.mode != ModeNormal {
		t.Fatalf("expected picker to dismiss on space, mode = %v", m.inputCtl.mode)
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

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	m = next.(*Model)

	if m.inputCtl.mode != ModePickerInline {
		t.Fatalf("expected picker to stay open, mode = %v", m.inputCtl.mode)
	}
	if cancels := drainPickerCancels(events); len(cancels) != 0 {
		t.Fatalf("expected no cancel, got %v", cancels)
	}
}

// TestInlinePickerNormalTypingKeepsFiltering verifies ordinary
// characters do not close the picker.
func TestInlinePickerNormalTypingKeepsFiltering(t *testing.T) {
	m, events := newInlinePickerModel(t, true, "/con")

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(*Model)

	if m.inputCtl.mode != ModePickerInline {
		t.Fatalf("expected picker to stay open, mode = %v", m.inputCtl.mode)
	}
	if cancels := drainPickerCancels(events); len(cancels) != 0 {
		t.Fatalf("expected no cancel, got %v", cancels)
	}
}

// TestInlinePickerClosesCleanlyOnEmptiedInput verifies that emptying the input
// closes the picker, resets its mode, and cancels its Lua callback.
func TestInlinePickerClosesCleanlyOnEmptiedInput(t *testing.T) {
	m, events := newInlinePickerModel(t, true, "/")

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = next.(*Model)

	if m.input.Value() != "" {
		t.Fatalf("expected empty input after backspace, got %q", m.input.Value())
	}
	if m.inputCtl.mode != ModeNormal {
		t.Fatalf("expected mode reset after input emptied, mode = %v", m.inputCtl.mode)
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

	if m.inputCtl.mode != ModeNormal {
		t.Fatalf("expected picker to dismiss, mode = %v", m.inputCtl.mode)
	}
	cancels := drainPickerCancels(events)
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
	if got := m.promptText; got != "HP      > " {
		t.Errorf("prompt = %q, want tab expanded", got)
	}
}

// TestHomeEndEditInputWhileCtrlVariantsScroll pins the default key
// split: with no binds registered, bare Home/End fall through to the
// input widget as cursor movement, while Ctrl+Home/Ctrl+End hit the Go
// scroll fallback (the path that keeps degraded mode navigable).
func TestHomeEndEditInputWhileCtrlVariantsScroll(t *testing.T) {
	m := newTestModel(t)

	typed := "say hello"
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(typed)})
	m = next.(*Model)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyHome})
	m = next.(*Model)
	if m.viewport.Mode() != widget.ModeLive {
		t.Fatal("Home scrolled the viewport instead of reaching the input")
	}
	if pos := m.inputCtl.input.Position(); pos != 0 {
		t.Fatalf("Home left cursor at %d, want 0", pos)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = next.(*Model)
	if m.viewport.Mode() != widget.ModeLive {
		t.Fatal("End scrolled the viewport instead of reaching the input")
	}
	if pos := m.inputCtl.input.Position(); pos != len(typed) {
		t.Fatalf("End left cursor at %d, want %d", pos, len(typed))
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlHome})
	m = next.(*Model)
	if m.viewport.Mode() == widget.ModeLive {
		t.Fatal("Ctrl+Home did not scroll the viewport to the top")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlEnd})
	m = next.(*Model)
	if m.viewport.Mode() != widget.ModeLive {
		t.Fatal("Ctrl+End did not return the viewport to live")
	}
	if got := m.inputCtl.input.Value(); got != typed {
		t.Fatalf("input draft = %q, want %q", got, typed)
	}
}

// Search changes the input widget's intrinsic height as its result list
// grows and collapses. The viewport must be sized for that final geometry
// before centering, or the selected source row can land just outside the
// visible window even though it is selected in the overlay.
func TestSearchFocusUsesFinalLayoutGeometry(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })

	events := make(chan ui.UIEvent, 64)
	m := NewModel(events)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = next.(*Model)
	next, _ = m.Update(ui.UpdateBarsMsg{"status": {Left: "status"}})
	m = next.(*Model)

	for i := 0; i < 9; i++ {
		next, _ = m.Update(ui.EchoLineMsg(fmt.Sprintf("match %d thief", i)))
		m = next.(*Model)
	}
	next, _ = m.Update(ui.EchoLineMsg("SELECTED thief"))
	m = next.(*Model)
	for i := 0; i < 20; i++ {
		next, _ = m.Update(ui.EchoLineMsg(fmt.Sprintf("quiet %d", i)))
		m = next.(*Model)
	}

	// Establish the normal layout, then the shorter no-match navigator. Typing
	// the query expands it to its five-result maximum in one update.
	m.View()
	next, _ = m.Update(ui.ShowSearchMsg{})
	m = next.(*Model)
	m.View()
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("thief")})
	m = next.(*Model)
	m.View()

	assertViewportRowCentered(t, m.viewport.View(), "SELECTED thief")

	// Enter removes the overlay and expands the viewport. The accepted row
	// must be centered again using that post-close height.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(*Model)
	m.View()
	assertViewportRowCentered(t, m.viewport.View(), "SELECTED thief")
	if m.searchView.focus == nil {
		t.Fatal("accepted search should retain its active-result marker")
	}
	committedSeq := m.searchView.focus.Seq
	assertViewportRowHighlighted(t, m.viewport.View(), "SELECTED thief")

	// A replacement search may preview another row, but cancelling it restores
	// the previously committed focus from the grouped search lifecycle state.
	next, _ = m.Update(ui.ShowSearchMsg{})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = next.(*Model)
	if m.searchView.focus == nil || m.searchView.focus.Seq == committedSeq {
		t.Fatal("replacement search did not preview an older result")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(*Model)
	if m.searchView.focus == nil || m.searchView.focus.Seq != committedSeq {
		t.Fatal("cancelled replacement search did not restore committed focus")
	}
	assertViewportRowHighlighted(t, m.viewport.View(), "SELECTED thief")

	// Deliberate viewport navigation retires the accepted marker.
	next, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	m = next.(*Model)
	if m.searchView.focus != nil {
		t.Fatal("manual scrolling should clear the accepted search marker")
	}
}

func TestSearchReportsInteractionStateSeparatelyFromScrollState(t *testing.T) {
	events := make(chan ui.UIEvent, 32)
	m := NewModel(events)
	m.initialized = true
	m.width = 80
	m.height = 24

	next, _ := m.Update(ui.ShowSearchMsg{})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	_ = next.(*Model)

	var states []bool
	for len(events) > 0 {
		if state, ok := (<-events).(ui.SearchStateChangedMsg); ok {
			states = append(states, bool(state))
		}
	}
	if len(states) != 2 || !states[0] || states[1] {
		t.Fatalf("search active states = %v, want [true false]", states)
	}
}

func TestManualViewportEntryPointsClearCommittedSearchFocus(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.Msg
	}{
		{
			name: "fallback scroll key",
			msg:  tea.KeyMsg{Type: tea.KeyPgUp},
		},
		{
			name: "mouse wheel",
			msg:  tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp},
		},
		{
			name: "Lua main-pane navigation",
			msg:  ui.PaneScrollUpMsg{Name: "main", Lines: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			focus := widget.SearchMatch{Seq: m.scrollback.Seq(50)}
			m.searchView.focus = &focus
			m.viewport.SetHighlight(focus.Seq, focus.Ranges)

			next, _ := m.Update(tt.msg)
			m = next.(*Model)
			if m.searchView.focus != nil {
				t.Fatal("manual viewport navigation retained committed search focus")
			}
		})
	}
}

func TestMouseWheelNavigatesActiveSearchMatches(t *testing.T) {
	m := newBareModel(t)
	for _, line := range []string{"thief oldest", "quiet", "thief middle", "quiet", "thief newest"} {
		m.appendMessage(line)
	}

	m.inputCtl.ShowSearch(ui.ShowSearchMsg{Query: "thief"})
	newest, ok := m.input.SearchSelected()
	if !ok || newest.Stripped != "thief newest" {
		t.Fatalf("initial selection = (%q, %v), want newest match", newest.Stripped, ok)
	}

	next, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	m = next.(*Model)
	middle, ok := m.input.SearchSelected()
	if !ok || middle.Stripped != "thief middle" {
		t.Fatalf("wheel up selection = (%q, %v), want older middle match", middle.Stripped, ok)
	}
	if !m.input.SearchActive() || m.searchView.focus == nil || m.searchView.focus.Seq != middle.Seq {
		t.Fatal("wheel navigation must keep search active and preview the selected match")
	}

	next, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	m = next.(*Model)
	selected, ok := m.input.SearchSelected()
	if !ok || selected.Seq != newest.Seq {
		t.Fatalf("wheel down selection = (%q, %v), want newer match", selected.Stripped, ok)
	}
}

func assertViewportRowCentered(t *testing.T, view, want string) {
	t.Helper()
	rows := strings.Split(view, "\n")
	for i, row := range rows {
		if runetext.StripANSI(row) != want {
			continue
		}
		center := (len(rows) - 1) / 2
		if i < center-1 || i > center+1 {
			t.Fatalf("row %q rendered at viewport row %d of %d, want it centered near %d\n%s",
				want, i, len(rows), center, runetext.StripANSI(view))
		}
		return
	}
	t.Fatalf("row %q is outside the viewport:\n%s", want, runetext.StripANSI(view))
}

func assertViewportRowHighlighted(t *testing.T, view, want string) {
	t.Helper()
	for _, row := range strings.Split(view, "\n") {
		if runetext.StripANSI(row) != want {
			continue
		}
		if !strings.Contains(row, "\x1b[") {
			t.Fatalf("row %q is visible but not highlighted:\n%s", want, runetext.StripANSI(view))
		}
		return
	}
	t.Fatalf("row %q is outside the viewport:\n%s", want, runetext.StripANSI(view))
}
