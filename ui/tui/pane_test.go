package tui

import (
	"fmt"
	"strings"
	"testing"

	runetext "github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/widget"
)

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
	m.Update(ui.PaneReplaceMsg{Name: "status", Text: "HP 11\nMP 5"})
	if got := status.Title(); got != "status" {
		t.Fatalf("replace left the pane scrolled: %q", got)
	}
	status.SetSize(20, 2)
	if got := status.View(); got != "HP 11\nMP 5" {
		t.Fatalf("replaced pane content = %q", got)
	}
}
