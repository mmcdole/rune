package tui

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/mmcdole/rune/input"
	runetext "github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/widget"
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

// newBareModel builds a sized model with an empty scrollback, for
// tests that assert on exact line counts and ordering.
func newBareModel(t *testing.T) *Model {
	t.Helper()

	events := make(chan ui.UIEvent, 64)
	m := NewModel(events)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return next.(*Model)
}

func TestOutputPaneImplementsPaneResourceLifecycle(t *testing.T) {
	m := newBareModel(t)

	output, ok := m.panes.Lookup(ui.OutputPaneName)
	if !ok {
		t.Fatal("output pane was not pre-created")
	}
	if output != m.output {
		t.Fatal("output registry entry does not preserve controller identity")
	}

	next, _ := m.Update(ui.PaneCreateMsg{Name: ui.OutputPaneName})
	m = next.(*Model)
	recreated, _ := m.panes.Lookup(ui.OutputPaneName)
	if recreated != output {
		t.Fatal("creating output replaced the reserved pane resource")
	}

	next, _ = m.Update(ui.PaneWriteMsg{Name: ui.OutputPaneName, Text: "visible"})
	m = next.(*Model)
	wantScrollback(t, m, "visible")

	// Visibility is placement state: hiding the output pane node in the
	// pushed tree unplaces it, while the buffer keeps accepting writes.
	hidden, found, changed := m.layout.WithPaneVisibility(ui.OutputPaneName, false)
	if !found || !changed {
		t.Fatalf("WithPaneVisibility(output, false) = found %v changed %v", found, changed)
	}
	next, _ = m.Update(ui.UpdateLayoutMsg(hidden))
	m = next.(*Model)
	if !m.layoutPlan.output.Empty() {
		t.Fatalf("hidden output remains placed: rect=%v", m.layoutPlan.output)
	}
	next, _ = m.Update(ui.PaneWriteMsg{Name: ui.OutputPaneName, Text: "hidden"})
	m = next.(*Model)
	wantScrollback(t, m, "visible", "hidden")

	shown, _, _ := m.layout.WithPaneVisibility(ui.OutputPaneName, true)
	next, _ = m.Update(ui.UpdateLayoutMsg(shown))
	m = next.(*Model)
	if m.layoutPlan.output.Empty() {
		t.Fatal("shown output is unplaced")
	}

	var rows []string
	for i := 0; i < 40; i++ {
		rows = append(rows, fmt.Sprintf("row %02d", i))
	}
	next, _ = m.Update(ui.PaneWriteMsg{Name: ui.OutputPaneName, Text: strings.Join(rows, "\n")})
	m = next.(*Model)
	next, _ = m.Update(ui.PaneScrollToTopMsg{Name: ui.OutputPaneName})
	m = next.(*Model)
	if m.output.viewport.Mode() != widget.ModeScrolled {
		t.Fatal("output pane did not honor pane scroll-to-top")
	}
	next, _ = m.Update(ui.PaneScrollToBottomMsg{Name: ui.OutputPaneName})
	m = next.(*Model)
	if m.output.viewport.Mode() != widget.ModeLive {
		t.Fatal("output pane did not honor pane scroll-to-bottom")
	}
}

func TestOutputBeforeFirstWindowSizeUsesBoundedStartupWidth(t *testing.T) {
	m := NewModel(make(chan ui.UIEvent, 16))
	line := strings.Repeat("x", defaultOutputWrapWidth+7)

	next, _ := m.Update(ui.PrintLineMsg(line))
	m = next.(*Model)
	if got := m.output.buffer.Count(); got != 2 {
		t.Fatalf("pre-size output rows = %d, want 2 at startup width", got)
	}
	if got := m.output.buffer.At(0); got != strings.Repeat("x", defaultOutputWrapWidth) {
		t.Fatalf("pre-size first row width = %d, want %d", len(got), defaultOutputWrapWidth)
	}
	if got := m.output.buffer.At(1); got != strings.Repeat("x", 7) {
		t.Fatalf("pre-size remainder = %q, want seven cells", got)
	}

	m = resizeModel(t, m, 120, 20)
	if got := m.output.buffer.Count(); got != 2 {
		t.Fatalf("first terminal size reflowed startup rows: got %d", got)
	}
}

func TestOutputPaneBuffersWhileHiddenOrUnplacedAtRetainedWidth(t *testing.T) {
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 64)), 20, 8)

	next, _ := m.Update(ui.PaneCreateMsg{Name: "side"})
	m = next.(*Model)
	next, _ = m.Update(ui.UpdateLayoutMsg(ui.LayoutTree{Root: ui.LayoutNode{
		Type: ui.LayoutTypeRow,
		Children: []ui.LayoutNode{
			{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Size: ui.Cells(12), Border: ui.PaneBorderNone},
			{Type: ui.LayoutTypePane, Name: "side", Size: ui.Cells(8), Border: ui.PaneBorderNone},
		},
	}}))
	m = next.(*Model)
	if got := m.layoutPlan.output.Dx(); got != 12 {
		t.Fatalf("placed output width = %d, want 12", got)
	}
	if got := m.output.wrapWidth; got != 12 {
		t.Fatalf("append width after placement = %d, want 12", got)
	}

	next, _ = m.Update(ui.PaneClearMsg{Name: ui.OutputPaneName})
	m = next.(*Model)
	hiddenOutput, _, _ := m.layout.WithPaneVisibility(ui.OutputPaneName, false)
	next, _ = m.Update(ui.UpdateLayoutMsg(hiddenOutput))
	m = next.(*Model)
	next, _ = m.Update(ui.PaneWriteMsg{Name: ui.OutputPaneName, Text: strings.Repeat("a", 14)})
	m = next.(*Model)
	if got := m.output.wrapWidth; got != 12 {
		t.Fatalf("hidden output changed retained width to %d", got)
	}
	wantScrollback(t, m, strings.Repeat("a", 12), "aa")

	next, _ = m.Update(ui.UpdateLayoutMsg(ui.LayoutTree{Root: ui.LayoutNode{
		Type: ui.LayoutTypeColumn,
		Children: []ui.LayoutNode{
			{Type: ui.LayoutTypeInput, Size: ui.AutoSize()},
		},
	}}))
	m = next.(*Model)
	if !m.layoutPlan.output.Empty() {
		t.Fatalf("layout without output placed it at %v", m.layoutPlan.output)
	}
	next, _ = m.Update(ui.PaneWriteMsg{Name: ui.OutputPaneName, Text: strings.Repeat("b", 14)})
	m = next.(*Model)
	if got := m.output.wrapWidth; got != 12 {
		t.Fatalf("unplaced output changed retained width to %d", got)
	}
	wantScrollback(t, m, strings.Repeat("a", 12), "aa", strings.Repeat("b", 12), "bb")

	next, _ = m.Update(ui.UpdateLayoutMsg(ui.LayoutTree{Root: ui.LayoutNode{
		Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Border: ui.PaneBorderNone,
	}}))
	m = next.(*Model)
	if got := m.output.wrapWidth; got != 20 {
		t.Fatalf("new output placement retained stale width %d, want 20", got)
	}
	next, _ = m.Update(ui.PaneWriteMsg{Name: ui.OutputPaneName, Text: strings.Repeat("c", 18)})
	m = next.(*Model)
	wantScrollback(t, m,
		strings.Repeat("a", 12), "aa", strings.Repeat("b", 12), "bb", strings.Repeat("c", 18))
}

func TestClearOutputPaneResetsTranscriptSearchAndViewportButPreservesPrompt(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.SetPromptMsg("HP> "))
	m = next.(*Model)
	var rows []string
	for i := 0; i < 40; i++ {
		rows = append(rows, fmt.Sprintf("row %02d", i))
	}
	rows[8] = "hidden thief"
	next, _ = m.Update(ui.PaneWriteMsg{Name: ui.OutputPaneName, Text: strings.Join(rows, "\n")})
	m = next.(*Model)
	next, _ = m.Update(ui.PaneScrollToTopMsg{Name: ui.OutputPaneName})
	m = next.(*Model)
	if m.output.viewport.Mode() != widget.ModeScrolled {
		t.Fatal("test setup did not scroll output")
	}
	next, _ = m.Update(ui.ShowSearchMsg{Query: "thief"})
	m = next.(*Model)
	if !m.input.SearchActive() || m.searchView.focus == nil {
		t.Fatal("test setup did not establish an active output search")
	}

	next, _ = m.Update(ui.PaneClearMsg{Name: ui.OutputPaneName})
	m = next.(*Model)
	if got := m.output.buffer.Count(); got != 0 {
		t.Fatalf("clear left %d transcript rows", got)
	}
	if m.input.SearchActive() || m.searchView.focus != nil || m.searchView.priorFocus != nil {
		t.Fatalf("clear retained search state: active=%v state=%+v", m.input.SearchActive(), m.searchView)
	}
	if mode := m.output.viewport.Mode(); mode != widget.ModeLive {
		t.Fatalf("clear left viewport mode %v, want live", mode)
	}
	if got := m.output.viewport.NewLineCount(); got != 0 {
		t.Fatalf("clear left new-line count %d", got)
	}
	if got := m.output.promptText; got != "HP> " {
		t.Fatalf("clear changed prompt to %q", got)
	}
	if got := runetext.StripANSI(m.output.viewport.View()); !strings.Contains(got, "HP> ") {
		t.Fatalf("preserved prompt is absent from output view: %q", got)
	}
}

func TestGrowingOutputViewportPublishesLiveState(t *testing.T) {
	events := make(chan ui.UIEvent, 128)
	m := resizeModel(t, NewModel(events), 30, 6)
	setLayout(m, ui.LayoutNode{
		Type: ui.LayoutTypeColumn,
		Children: []ui.LayoutNode{
			{Type: ui.LayoutTypePane, Name: ui.OutputPaneName},
			{Type: ui.LayoutTypeInput, Size: ui.AutoSize()},
		},
	})
	for i := 1; i <= 10; i++ {
		next, _ := m.Update(ui.EchoLineMsg(fmt.Sprintf("line %d", i)))
		m = next.(*Model)
	}
	next, _ := m.Update(ui.PaneScrollToTopMsg{Name: ui.OutputPaneName})
	m = next.(*Model)
	if m.output.viewport.Mode() != widget.ModeScrolled {
		t.Fatal("test setup did not scroll the constrained output pane")
	}
	if view := runetext.StripANSI(m.View().Content); strings.Contains(view, "output · scroll") {
		t.Fatalf("scrolled output has an unexpected default title: %q", view)
	}
	for len(events) > 0 {
		<-events
	}

	next, _ = m.Update(tea.WindowSizeMsg{Width: 30, Height: 20})
	m = next.(*Model)
	if m.output.viewport.Mode() != widget.ModeLive || m.output.viewport.NewLineCount() != 0 {
		t.Fatalf("expanded output remained scrolled: mode=%v new=%d",
			m.output.viewport.Mode(), m.output.viewport.NewLineCount())
	}
	if view := runetext.StripANSI(m.View().Content); strings.Contains(view, "output · scroll") {
		t.Fatalf("expanded output retained stale scroll title: %q", view)
	}

	foundLive := false
	for len(events) > 0 {
		if state, ok := (<-events).(ui.ScrollStateChangedMsg); ok && state.Mode == "live" && state.NewLines == 0 {
			foundLive = true
		}
	}
	if !foundLive {
		t.Fatal("resize did not publish the geometry-induced return to live mode")
	}
}

func TestClosingTallSearchPublishesGeometryInducedLiveState(t *testing.T) {
	events := make(chan ui.UIEvent, 128)
	m := resizeModel(t, NewModel(events), 30, 12)
	setLayout(m, ui.LayoutNode{
		Type: ui.LayoutTypeColumn,
		Children: []ui.LayoutNode{
			{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Border: ui.PaneBorderHorizontal},
			{Type: ui.LayoutTypeInput, Size: ui.AutoSize()},
		},
	})
	for i := 1; i <= 5; i++ {
		next, _ := m.Update(ui.EchoLineMsg(fmt.Sprintf("line %d", i)))
		m = next.(*Model)
	}
	next, _ := m.Update(ui.ShowSearchMsg{Query: "line"})
	m = next.(*Model)
	if !m.input.SearchActive() || m.layoutPlan.output.Dy() != 2 {
		t.Fatalf("search setup = active %v output height %d, want true and 2 (shared border)",
			m.input.SearchActive(), m.layoutPlan.output.Dy())
	}
	next, _ = m.Update(ui.PaneScrollToTopMsg{Name: ui.OutputPaneName})
	m = next.(*Model)
	if m.output.viewport.Mode() != widget.ModeScrolled {
		t.Fatal("test setup did not scroll the search-constrained output")
	}
	for len(events) > 0 {
		<-events
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = next.(*Model)
	if m.input.SearchActive() {
		t.Fatal("Escape did not close search")
	}
	if m.output.viewport.Mode() != widget.ModeLive || m.output.viewport.NewLineCount() != 0 {
		t.Fatalf("post-search output = mode %v new %d, want live zero state",
			m.output.viewport.Mode(), m.output.viewport.NewLineCount())
	}
	if view := runetext.StripANSI(m.View().Content); strings.Contains(view, "output · scroll") {
		t.Fatalf("post-search output retained stale title: %q", view)
	}

	foundLive := false
	for len(events) > 0 {
		if state, ok := (<-events).(ui.ScrollStateChangedMsg); ok && state.Mode == "live" && state.NewLines == 0 {
			foundLive = true
		}
	}
	if !foundLive {
		t.Fatal("closing search did not publish the geometry-induced live state")
	}
}

func TestStaleOutputBatchTickCannotDisturbPostClearBatch(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.PrintLineMsg("old immediate"))
	m = next.(*Model)
	next, _ = m.Update(ui.PrintLineMsg("old pending"))
	m = next.(*Model)
	staleGeneration := m.output.batchGeneration

	next, _ = m.Update(ui.PaneClearMsg{Name: ui.OutputPaneName})
	m = next.(*Model)
	next, _ = m.Update(ui.PrintLineMsg("new immediate"))
	m = next.(*Model)
	next, _ = m.Update(ui.PrintLineMsg("new pending"))
	m = next.(*Model)
	currentGeneration := m.output.batchGeneration
	if currentGeneration == staleGeneration {
		t.Fatal("clear did not invalidate the in-flight batch generation")
	}
	wantScrollback(t, m, "new immediate")

	next, cmd := m.Update(tickMsg{generation: staleGeneration})
	m = next.(*Model)
	if cmd != nil {
		t.Fatal("stale tick re-armed output batching")
	}
	if !m.output.flushScheduled || len(m.output.pendingRows) != 1 {
		t.Fatalf("stale tick disturbed current batch: scheduled=%v pending=%v", m.output.flushScheduled, m.output.pendingRows)
	}
	wantScrollback(t, m, "new immediate")

	next, cmd = m.Update(tickMsg{generation: currentGeneration})
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("current tick did not re-arm after flushing pending output")
	}
	wantScrollback(t, m, "new immediate", "new pending")
}

func TestOrdinaryPaneLifecycle(t *testing.T) {
	m := newBareModel(t)
	next, _ := m.Update(ui.UpdateLayoutMsg(ui.LayoutTree{Root: ui.LayoutNode{
		Type: ui.LayoutTypeRow,
		Children: []ui.LayoutNode{
			{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Border: ui.PaneBorderNone},
			{Type: ui.LayoutTypePane, Name: "chat", Border: ui.PaneBorderNone},
		},
	}}))
	m = next.(*Model)

	next, _ = m.Update(ui.PaneCreateMsg{Name: "chat"})
	m = next.(*Model)
	chat, ok := m.panes.Lookup("chat")
	if !ok {
		t.Fatal("ordinary pane was not created")
	}
	findLeaf(t, m.layoutPlan, ui.LayoutTypePane, "chat")

	// A hidden placement is pruned from the plan while the buffer keeps
	// accepting writes.
	hiddenChat, found, _ := m.layout.WithPaneVisibility("chat", false)
	if !found {
		t.Fatal("chat placement was not found in the installed tree")
	}
	next, _ = m.Update(ui.UpdateLayoutMsg(hiddenChat))
	m = next.(*Model)
	next, _ = m.Update(ui.PaneWriteMsg{Name: "chat", Text: "oldest\nmiddle\nnewest"})
	m = next.(*Model)
	chat.SetSize(20, 3)
	if got := chat.View(); got != "oldest\nmiddle\nnewest" {
		t.Fatalf("hidden ordinary pane did not buffer writes: %q", got)
	}
	for _, leaf := range m.layoutPlan.leaves {
		if leaf.node.Type == ui.LayoutTypePane && leaf.surface == chat {
			t.Fatal("hidden pane placement still resolved")
		}
	}

	shownChat, _, _ := m.layout.WithPaneVisibility("chat", true)
	next, _ = m.Update(ui.UpdateLayoutMsg(shownChat))
	m = next.(*Model)
	findLeaf(t, m.layoutPlan, ui.LayoutTypePane, "chat")
	next, _ = m.Update(ui.PaneScrollToTopMsg{Name: "chat"})
	m = next.(*Model)
	if !strings.Contains(chat.Title(), "scroll") {
		t.Fatalf("ordinary pane title does not expose scrolled state: %q", chat.Title())
	}
	next, _ = m.Update(ui.PaneScrollToBottomMsg{Name: "chat"})
	m = next.(*Model)
	if got := chat.Title(); got != "chat" {
		t.Fatalf("ordinary pane did not return to live state: %q", got)
	}

	_, _ = m.Update(ui.PaneClearMsg{Name: "chat"})
	chat.SetSize(20, 1)
	if got := chat.View(); got != "" {
		t.Fatalf("ordinary pane clear left content %q", got)
	}
}

func TestPasteMessageRoutesAtomicallyToComposer(t *testing.T) {
	events := make(chan ui.UIEvent, 4)
	m := NewModel(events)

	next, _ := m.Update(tea.PasteMsg{Content: "say hello\nsay goodbye"})
	m = next.(*Model)

	if m.inputCtl.mode() != modeCompose {
		t.Fatalf("paste mode = %v, want compose", m.inputCtl.mode())
	}
	if got := m.input.Value(); got != "say hello\nsay goodbye" {
		t.Fatalf("pasted input = %q", got)
	}
	changed, ok := (<-events).(ui.InputChangedMsg)
	if !ok || changed.Text != "say hello\nsay goodbye" {
		t.Fatalf("paste event = %#v, want one atomic input change", changed)
	}
}

func TestAcceptedSubmissionFollowsDraftChangeOnOneUIEventLane(t *testing.T) {
	events := make(chan ui.UIEvent, 4)
	m := NewModel(events)

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyExtended, Text: "look"})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_ = next.(*Model)

	changed, ok := (<-events).(ui.InputChangedMsg)
	if !ok || changed.Text != "look" {
		t.Fatalf("first event = %#v, want draft change to look", changed)
	}
	submitted, ok := (<-events).(ui.InputSubmittedMsg)
	if !ok || submitted.Submission != input.Command("look") {
		t.Fatalf("second event = %#v, want command submission", submitted)
	}
	if submitted.NextDraft != "" {
		t.Fatalf("next draft = %q, want empty", submitted.NextDraft)
	}
	select {
	case event := <-events:
		t.Fatalf("accepted submission emitted redundant event %#v", event)
	default:
	}
}

func TestKeptSubmissionCarriesPostSubmitDraftInOneAcceptedEvent(t *testing.T) {
	events := make(chan ui.UIEvent, 1)
	m := NewModel(events)

	next, _ := m.Update(ui.UpdateConfigMsg{KeepInput: true})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyExtended, Text: "north"})
	m = next.(*Model)

	// Drain the ordinary edit so the capacity-one queue can accept Enter.
	if changed, ok := (<-events).(ui.InputChangedMsg); !ok || changed.Text != "north" {
		t.Fatalf("draft event = %#v, want north", changed)
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(*Model)

	got, ok := (<-events).(ui.InputSubmittedMsg)
	if !ok {
		t.Fatalf("event = %T, want InputSubmittedMsg", got)
	}
	if got.Submission != input.Command("north") {
		t.Fatalf("submission = %+v, want north command", got.Submission)
	}
	if got.NextDraft != "north" {
		t.Fatalf("next draft = %q, want north", got.NextDraft)
	}
	if got := m.input.Value(); got != "north" || !m.input.Selected() {
		t.Fatalf("local input = %q selected=%v, want kept selection", got, m.input.Selected())
	}
	if got := m.output.buffer.Count(); got != 0 {
		t.Fatalf("warning rows = %d, want none", got)
	}
	select {
	case event := <-events:
		t.Fatalf("kept submit emitted a second event %#v", event)
	default:
	}
}

func TestFullUIEventQueueRejectsSubmissionWithoutLosingDraft(t *testing.T) {
	events := make(chan ui.UIEvent, 1)
	m := NewModel(events)

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyExtended, Text: "look"})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(*Model)

	if got := m.inputCtl.input.Value(); got != "look" {
		t.Fatalf("rejected submission changed draft to %q", got)
	}
	if got := m.output.buffer.Count(); got != 1 {
		t.Fatalf("warning rows = %d, want exactly one", got)
	}
	if warning := runetext.StripANSI(m.output.buffer.At(0)); !strings.Contains(warning, "Input not sent - engine lagging") {
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

	if got := m.output.buffer.Count(); got != 1 {
		t.Fatalf("warning rows = %d, want exactly one", got)
	}
	if warning := runetext.StripANSI(m.output.buffer.At(0)); !strings.Contains(warning, "UI event dropped - engine lagging") {
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

	if got := m.output.buffer.Count(); got != 1 {
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

	if got := m.output.buffer.Count(); got != 1 {
		t.Fatalf("expected burst lines batched, scrollback has %d lines", got)
	}

	next, _ = m.Update(tickMsg{generation: m.output.batchGeneration})
	m = next.(*Model)

	if got := m.output.buffer.Count(); got != 3 {
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

	next, cmd := m.Update(tickMsg{generation: m.output.batchGeneration})
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("expected tick with pending lines to re-arm the window")
	}

	_, cmd = m.Update(tickMsg{generation: m.output.batchGeneration})
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

	if got := m.output.buffer.Count(); got != 3 {
		t.Fatalf("expected 3 scrollback lines, got %d", got)
	}
	for i, want := range []string{"line 1", "line 2", "> look"} {
		if got := m.output.buffer.At(i); got != want {
			t.Fatalf("scrollback[%d] = %q, want %q (echo reordered?)", i, got, want)
		}
	}

	next, cmd := m.Update(tickMsg{generation: m.output.batchGeneration})
	m = next.(*Model)
	if cmd != nil {
		t.Fatal("expected trailing tick after eager echo flush to stop the chain")
	}
	if got := m.output.buffer.Count(); got != 3 {
		t.Fatalf("trailing tick changed scrollback, got %d lines", got)
	}
}

func TestPromptCommitPrecedesFollowingRows(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.PrintLineMsg("line 1")) // immediate, opens window
	m = next.(*Model)
	next, _ = m.Update(ui.PrintLineMsg("line 2")) // batched
	m = next.(*Model)
	next, _ = m.Update(ui.SetPromptMsg("Username:"))
	m = next.(*Model)
	next, _ = m.Update(ui.CommitPromptMsg("Username:"))
	m = next.(*Model)
	next, _ = m.Update(ui.EchoLineMsg("> player"))
	m = next.(*Model)
	next, _ = m.Update(ui.PrintLineMsg("login hook sent username"))
	m = next.(*Model)
	next, _ = m.Update(tickMsg{generation: m.output.batchGeneration})
	m = next.(*Model)

	wantScrollback(t, m,
		"line 1", "line 2", "Username:", "> player", "login hook sent username")
	if m.output.promptText != "" {
		t.Fatalf("prompt overlay = %q after commit, want empty", m.output.promptText)
	}
}

func TestOrderedPromptCommitThenLocalSubmissionOutput(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.SetPromptMsg("HP>"))
	m = next.(*Model)
	next, _ = m.Update(ui.CommitPromptMsg("HP>"))
	m = next.(*Model)
	next, _ = m.Update(ui.EchoLineMsg("> /help"))
	m = next.(*Model)
	next, _ = m.Update(ui.PrintLineMsg("local help"))
	m = next.(*Model)

	wantScrollback(t, m, "HP>", "> /help", "local help")
	if got := m.output.promptText; got != "" {
		t.Fatalf("prompt overlay = %q after commit, want empty", got)
	}
}

func TestPromptClearClearsOverlay(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.SetPromptMsg("User"))
	m = next.(*Model)
	next, _ = m.Update(ui.SetPromptMsg("Username:"))
	m = next.(*Model)

	wantScrollback(t, m)
	if got := m.output.promptText; got != "Username:" {
		t.Fatalf("prompt overlay = %q, want %q", got, "Username:")
	}

	next, _ = m.Update(ui.SetPromptMsg(""))
	m = next.(*Model)

	wantScrollback(t, m)
	if m.output.promptText != "" {
		t.Fatalf("prompt overlay = %q after clear, want empty", m.output.promptText)
	}
}

func wantScrollback(t *testing.T, m *Model, want ...string) {
	t.Helper()
	if got := m.output.buffer.Count(); got != len(want) {
		t.Fatalf("scrollback has %d rows, want %d", got, len(want))
	}
	for i, w := range want {
		if got := m.output.buffer.At(i); got != w {
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
	next, _ = m.Update(tickMsg{generation: m.output.batchGeneration})
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

	got := m.output.buffer.At(0)
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
	if m.submit(ui.InputSubmittedMsg{Submission: tooManyLines}) {
		t.Fatal("over-line-limit verbatim submission was accepted")
	}
	tooManyBytes := input.Verbatim(strings.Repeat("x", maxVerbatimBytes+1))
	if m.submit(ui.InputSubmittedMsg{Submission: tooManyBytes}) {
		t.Fatal("over-byte-limit verbatim submission was accepted")
	}
	tooManyCRLines := input.Verbatim(strings.Repeat("\r", maxVerbatimLines))
	if m.submit(ui.InputSubmittedMsg{Submission: tooManyCRLines}) {
		t.Fatal("over-line-limit bare-CR verbatim submission was accepted")
	}

	if got := m.output.buffer.Count(); got != 3 {
		t.Fatalf("warning count = %d, want 3", got)
	}
	for n := 0; n < m.output.buffer.Count(); n++ {
		if warning := m.output.buffer.At(n); !strings.Contains(warning, "Verbatim input not sent") {
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
	if !m.submit(ui.InputSubmittedMsg{Submission: submission}) {
		t.Fatal("at-limit verbatim submission was rejected")
	}
	got, ok := (<-events).(ui.InputSubmittedMsg)
	if !ok || got.Submission != submission {
		t.Fatalf("queued event = %#v, want submission %+v", got, submission)
	}
}

// TestBarNameDoesNotReplaceBuiltinWidget verifies that bar and built-in names
// belong to independent resource namespaces.
func TestBarNameDoesNotReplaceBuiltinWidget(t *testing.T) {
	m := newTestModel(t)
	inputWidget := m.input

	next, _ := m.Update(ui.UpdateBarsMsg{"input": {Left: "hijack"}})
	m = next.(*Model)

	if m.input != inputWidget {
		t.Fatal("bar named \"input\" replaced the input widget")
	}
	if _, exists := m.bars["input"]; !exists {
		t.Fatal("bar named \"input\" was not retained in its own namespace")
	}

	next, _ = m.Update(ui.UpdateBarsMsg{})
	m = next.(*Model)

	if m.input != inputWidget {
		t.Fatal("removing the colliding bar deleted the input widget")
	}
}

// TestSeparatorLeafCharactersAreIndependent verifies that configured and
// default separator characters belong to their individual leaves.
func TestSeparatorLeafCharactersAreIndependent(t *testing.T) {
	m := newTestModel(t)

	// No "input" entry: the input widget draws its own default rule,
	// which would mask a separator that failed to reset.
	next, _ := m.Update(ui.UpdateLayoutMsg(ui.LayoutTree{Root: ui.LayoutNode{
		Type: ui.LayoutTypeColumn,
		Children: []ui.LayoutNode{
			{Type: ui.LayoutTypeSeparator, SeparatorChar: "═", Size: ui.AutoSize()},
			{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Border: ui.PaneBorderNone},
			{Type: ui.LayoutTypeSeparator, Size: ui.AutoSize()},
		},
	}}))
	m = next.(*Model)

	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, strings.Repeat("═", m.width)) {
		t.Error("configured separator rule missing from view")
	}
	if !strings.Contains(view, strings.Repeat("─", m.width)) {
		t.Error("default separator did not retain its own rule character")
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

func TestSetInputSubmissionMessageForcesVerbatimMode(t *testing.T) {
	m := newBareModel(t)
	next, _ := m.Update(ui.SetInputSubmissionMsg(input.Verbatim("one line;still data")))
	m = next.(*Model)

	if m.inputCtl.mode() != modeCompose || !m.input.IsComposing() {
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
	for i := 0; i < m.output.buffer.Count(); i++ {
		row := m.output.buffer.At(i)
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
	next, _ = m.Update(ui.SetPromptMsg("HP\t> "))
	m = next.(*Model)
	if got := m.output.promptText; got != "HP      > " {
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
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyExtended, Text: typed})
	m = next.(*Model)

	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	m = next.(*Model)
	if m.output.viewport.Mode() != widget.ModeLive {
		t.Fatal("Home scrolled the viewport instead of reaching the input")
	}
	if pos := m.inputCtl.input.Position(); pos != 0 {
		t.Fatalf("Home left cursor at %d, want 0", pos)
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	m = next.(*Model)
	if m.output.viewport.Mode() != widget.ModeLive {
		t.Fatal("End scrolled the viewport instead of reaching the input")
	}
	if pos := m.inputCtl.input.Position(); pos != len(typed) {
		t.Fatalf("End left cursor at %d, want %d", pos, len(typed))
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyHome, Mod: tea.ModCtrl})
	m = next.(*Model)
	if m.output.viewport.Mode() == widget.ModeLive {
		t.Fatal("Ctrl+Home did not scroll the viewport to the top")
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd, Mod: tea.ModCtrl})
	m = next.(*Model)
	if m.output.viewport.Mode() != widget.ModeLive {
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
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyExtended, Text: "thief"})
	m = next.(*Model)
	m.View()

	assertViewportRowCentered(t, m.output.viewport.View(), "SELECTED thief")

	// Enter removes the overlay and expands the viewport. The accepted row
	// must be centered again using that post-close height.
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(*Model)
	m.View()
	assertViewportRowCentered(t, m.output.viewport.View(), "SELECTED thief")
	if m.searchView.focus == nil {
		t.Fatal("accepted search should retain its active-result marker")
	}
	committedSeq := m.searchView.focus.Seq
	assertViewportRowHighlighted(t, m.output.viewport.View(), "SELECTED thief")

	// A replacement search may preview another row, but cancelling it restores
	// the previously committed focus from the grouped search lifecycle state.
	next, _ = m.Update(ui.ShowSearchMsg{})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = next.(*Model)
	if m.searchView.focus == nil || m.searchView.focus.Seq == committedSeq {
		t.Fatal("replacement search did not preview an older result")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = next.(*Model)
	if m.searchView.focus == nil || m.searchView.focus.Seq != committedSeq {
		t.Fatal("cancelled replacement search did not restore committed focus")
	}
	assertViewportRowHighlighted(t, m.output.viewport.View(), "SELECTED thief")

	// Deliberate viewport navigation retires the accepted marker.
	next, _ = m.Update(ui.UpdateConfigMsg{Mouse: true})
	m = next.(*Model)
	next, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
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
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
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
		name  string
		msg   tea.Msg
		mouse bool
	}{
		{
			name: "fallback scroll key",
			msg:  tea.KeyPressMsg{Code: tea.KeyPgUp},
		},
		{
			name:  "mouse wheel",
			msg:   tea.MouseWheelMsg{Button: tea.MouseWheelUp},
			mouse: true,
		},
		{
			name: "Lua output-pane navigation",
			msg:  ui.PaneScrollUpMsg{Name: ui.OutputPaneName, Lines: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			if tt.mouse {
				next, _ := m.Update(ui.UpdateConfigMsg{Mouse: true})
				m = next.(*Model)
			}
			focus := widget.SearchMatch{Seq: m.output.buffer.Seq(50)}
			m.searchView.focus = &focus
			m.output.viewport.SetHighlight(focus.Seq, focus.Ranges)

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
	next, _ := m.Update(ui.UpdateConfigMsg{Mouse: true})
	m = next.(*Model)
	for _, line := range []string{"thief oldest", "quiet", "thief middle", "quiet", "thief newest"} {
		m.appendMessage(line)
	}

	m.inputCtl.ShowSearch(ui.ShowSearchMsg{Query: "thief"})
	newest, ok := m.input.Search().Selected()
	if !ok || newest.Stripped != "thief newest" {
		t.Fatalf("initial selection = (%q, %v), want newest match", newest.Stripped, ok)
	}

	next, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	m = next.(*Model)
	middle, ok := m.input.Search().Selected()
	if !ok || middle.Stripped != "thief middle" {
		t.Fatalf("wheel up selection = (%q, %v), want older middle match", middle.Stripped, ok)
	}
	if !m.input.SearchActive() || m.searchView.focus == nil || m.searchView.focus.Seq != middle.Seq {
		t.Fatal("wheel navigation must keep search active and preview the selected match")
	}

	next, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	m = next.(*Model)
	selected, ok := m.input.Search().Selected()
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

// TestReplaceOutputPaneIsOneClearAndWrite: a replace behaves like clear
// (search dropped, viewport live, prompt kept, stale batch tick ignored) and
// then holds only the new rows, all within one Update.
func TestReplaceOutputPaneIsOneClearAndWrite(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.SetPromptMsg("HP> "))
	m = next.(*Model)
	var rows []string
	for i := 0; i < 40; i++ {
		rows = append(rows, fmt.Sprintf("row %02d", i))
	}
	rows[8] = "hidden thief"
	next, _ = m.Update(ui.PaneWriteMsg{Name: ui.OutputPaneName, Text: strings.Join(rows, "\n")})
	m = next.(*Model)
	next, _ = m.Update(ui.PaneScrollToTopMsg{Name: ui.OutputPaneName})
	m = next.(*Model)
	next, _ = m.Update(ui.ShowSearchMsg{Query: "thief"})
	m = next.(*Model)
	if !m.input.SearchActive() || m.output.viewport.Mode() != widget.ModeScrolled {
		t.Fatal("test setup did not scroll and search output")
	}
	generation, scheduled := m.output.printServer("batched")
	if !scheduled {
		t.Fatal("test setup did not open a batch window")
	}

	next, _ = m.Update(ui.PaneReplaceMsg{Name: ui.OutputPaneName, Text: "first\nsecond"})
	m = next.(*Model)
	if got := m.output.buffer.Count(); got != 2 {
		t.Fatalf("replace left %d transcript rows, want the two new rows", got)
	}
	if got := m.output.buffer.At(0) + "|" + m.output.buffer.At(1); got != "first|second" {
		t.Fatalf("replace rows = %q", got)
	}
	if m.input.SearchActive() || m.searchView.focus != nil {
		t.Fatal("replace retained search state")
	}
	if mode := m.output.viewport.Mode(); mode != widget.ModeLive {
		t.Fatalf("replace left viewport mode %v, want live", mode)
	}
	if got := m.output.promptText; got != "HP> " {
		t.Fatalf("replace dropped the live prompt: %q", got)
	}
	next, _ = m.Update(tickMsg{generation: generation})
	m = next.(*Model)
	if got := m.output.buffer.Count(); got != 2 {
		t.Fatalf("stale batch tick changed the transcript after replace: %d rows", got)
	}
}

// TestReplaceOrdinaryPaneCreatesAndSnapsToLive covers the non-output pane
// contract: replace creates a missing buffer like write and returns a
// scrolled pane to live tailing with only the new content.
func TestReplaceOrdinaryPaneCreatesAndSnapsToLive(t *testing.T) {
	m := newBareModel(t)
	next, _ := m.Update(ui.PaneReplaceMsg{Name: "status", Text: "HP 10"})
	m = next.(*Model)
	status, ok := m.panes.Lookup("status")
	if !ok {
		t.Fatal("replace did not create the pane")
	}
	status.SetSize(20, 1)
	if got := status.View(); got != "HP 10" {
		t.Fatalf("created pane content = %q", got)
	}

	next, _ = m.Update(ui.PaneWriteMsg{Name: "status", Text: "a\nb\nc"})
	m = next.(*Model)
	next, _ = m.Update(ui.PaneScrollToTopMsg{Name: "status"})
	m = next.(*Model)
	if !strings.Contains(status.Title(), "scroll") {
		t.Fatal("test setup did not scroll the pane")
	}
	next, _ = m.Update(ui.PaneReplaceMsg{Name: "status", Text: "HP 11\nMP 5"})
	m = next.(*Model)
	if got := status.Title(); got != "status" {
		t.Fatalf("replace left the pane scrolled: %q", got)
	}
	status.SetSize(20, 2)
	if got := status.View(); got != "HP 11\nMP 5" {
		t.Fatalf("replaced pane content = %q", got)
	}
}
