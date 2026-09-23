package tui

import (
	"fmt"
	"strings"
	"testing"

	runetext "github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/widget"
)

func TestOutputPaneImplementsPaneLifecycle(t *testing.T) {
	m := newBareModel(t)

	output, ok := m.panes[ui.OutputPaneName]
	if !ok {
		t.Fatal("output pane was not pre-created")
	}
	if output != m.output {
		t.Fatal("output registry entry does not preserve controller identity")
	}

	next, _ := m.Update(paneCreateMsg{Name: ui.OutputPaneName})
	m = next.(*Model)
	recreated := m.panes[ui.OutputPaneName]
	if recreated != output {
		t.Fatal("creating output replaced the reserved pane")
	}

	next, _ = m.Update(paneWriteMsg{Name: ui.OutputPaneName, Text: "visible"})
	m = next.(*Model)
	wantScrollback(t, m, "visible")

	// Visibility is placement state: hiding the output pane node in the
	// pushed tree unplaces it, while the buffer keeps accepting writes.
	hidden, found, changed := m.layout.WithPaneVisibility(ui.OutputPaneName, false)
	if !found || !changed {
		t.Fatalf("WithPaneVisibility(output, false) = found %v changed %v", found, changed)
	}
	next, _ = m.Update(updateLayoutMsg(hidden))
	m = next.(*Model)
	if !m.layoutPlan.output.Empty() {
		t.Fatalf("hidden output remains placed: rect=%v", m.layoutPlan.output)
	}
	next, _ = m.Update(paneWriteMsg{Name: ui.OutputPaneName, Text: "hidden"})
	m = next.(*Model)
	wantScrollback(t, m, "visible", "hidden")

	shown, _, _ := m.layout.WithPaneVisibility(ui.OutputPaneName, true)
	next, _ = m.Update(updateLayoutMsg(shown))
	m = next.(*Model)
	if m.layoutPlan.output.Empty() {
		t.Fatal("shown output is unplaced")
	}

	var rows []string
	for i := 0; i < 40; i++ {
		rows = append(rows, fmt.Sprintf("row %02d", i))
	}
	next, _ = m.Update(paneWriteMsg{Name: ui.OutputPaneName, Text: strings.Join(rows, "\n")})
	m = next.(*Model)
	next, _ = m.Update(paneScrollToTopMsg{Name: ui.OutputPaneName})
	m = next.(*Model)
	if m.output.Mode() != widget.ModeScrolled {
		t.Fatal("output pane did not honor pane scroll-to-top")
	}
	next, _ = m.Update(paneScrollToBottomMsg{Name: ui.OutputPaneName})
	m = next.(*Model)
	if m.output.Mode() != widget.ModeLive {
		t.Fatal("output pane did not honor pane scroll-to-bottom")
	}
}

func TestOutputBeforeFirstWindowSizeUsesBoundedStartupWidth(t *testing.T) {
	m := NewModel(make(chan ui.UIEvent, 16))
	line := strings.Repeat("x", 80+7)

	next, _ := m.Update(printLineMsg(line))
	m = next.(*Model)
	if got := m.output.Scrollback().Count(); got != 2 {
		t.Fatalf("pre-size output rows = %d, want 2 at startup width", got)
	}
	if got := m.output.Scrollback().At(0); got != strings.Repeat("x", 80) {
		t.Fatalf("pre-size first row width = %d, want %d", len(got), 80)
	}
	if got := m.output.Scrollback().At(1); got != strings.Repeat("x", 7) {
		t.Fatalf("pre-size remainder = %q, want seven cells", got)
	}

	m = resizeModel(t, m, 120, 20)
	if got := m.output.Scrollback().Count(); got != 2 {
		t.Fatalf("first terminal size reflowed startup rows: got %d", got)
	}
}

func TestOutputPaneBuffersWhileHiddenOrUnplacedAtRetainedWidth(t *testing.T) {
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 64)), 20, 8)

	next, _ := m.Update(paneCreateMsg{Name: "side"})
	m = next.(*Model)
	next, _ = m.Update(updateLayoutMsg(ui.LayoutTree{Root: ui.LayoutNode{
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
	if got := m.output.Width(); got != 12 {
		t.Fatalf("append width after placement = %d, want 12", got)
	}

	next, _ = m.Update(paneClearMsg{Name: ui.OutputPaneName})
	m = next.(*Model)
	hiddenOutput, _, _ := m.layout.WithPaneVisibility(ui.OutputPaneName, false)
	next, _ = m.Update(updateLayoutMsg(hiddenOutput))
	m = next.(*Model)
	next, _ = m.Update(paneWriteMsg{Name: ui.OutputPaneName, Text: strings.Repeat("a", 14)})
	m = next.(*Model)
	if got := m.output.Width(); got != 12 {
		t.Fatalf("hidden output changed retained width to %d", got)
	}
	wantScrollback(t, m, strings.Repeat("a", 12), "aa")

	next, _ = m.Update(updateLayoutMsg(ui.LayoutTree{Root: ui.LayoutNode{
		Type: ui.LayoutTypeColumn,
		Children: []ui.LayoutNode{
			{Type: ui.LayoutTypeInput, Size: ui.AutoSize()},
		},
	}}))
	m = next.(*Model)
	if !m.layoutPlan.output.Empty() {
		t.Fatalf("layout without output placed it at %v", m.layoutPlan.output)
	}
	next, _ = m.Update(paneWriteMsg{Name: ui.OutputPaneName, Text: strings.Repeat("b", 14)})
	m = next.(*Model)
	if got := m.output.Width(); got != 12 {
		t.Fatalf("unplaced output changed retained width to %d", got)
	}
	wantScrollback(t, m, strings.Repeat("a", 12), "aa", strings.Repeat("b", 12), "bb")

	next, _ = m.Update(updateLayoutMsg(ui.LayoutTree{Root: ui.LayoutNode{
		Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Border: ui.PaneBorderNone,
	}}))
	m = next.(*Model)
	if got := m.output.Width(); got != 20 {
		t.Fatalf("new output placement retained dirty width %d, want 20", got)
	}
	next, _ = m.Update(paneWriteMsg{Name: ui.OutputPaneName, Text: strings.Repeat("c", 18)})
	m = next.(*Model)
	wantScrollback(t, m,
		strings.Repeat("a", 12), "aa", strings.Repeat("b", 12), "bb", strings.Repeat("c", 18))
}

func TestClearOutputPaneResetsSearchAndScrollingButPreservesPrompt(t *testing.T) {
	m := newBareModel(t)
	normalOutput := m.layoutPlan.output

	next, _ := m.Update(setPromptMsg("HP> "))
	m = next.(*Model)
	var rows []string
	for i := 0; i < 40; i++ {
		rows = append(rows, fmt.Sprintf("row %02d", i))
	}
	rows[8] = "hidden thief"
	next, _ = m.Update(paneWriteMsg{Name: ui.OutputPaneName, Text: strings.Join(rows, "\n")})
	m = next.(*Model)
	next, _ = m.Update(paneScrollToTopMsg{Name: ui.OutputPaneName})
	m = next.(*Model)
	if m.output.Mode() != widget.ModeScrolled {
		t.Fatal("test setup did not scroll output")
	}
	next, _ = m.Update(showSearchMsg{options: ui.SearchOptions{Query: "thief"}})
	m = next.(*Model)
	if !m.input.SearchActive() || m.searchView.focus == nil {
		t.Fatal("test setup did not establish an active output search")
	}
	if m.layoutPlan.output == normalOutput {
		t.Fatal("test setup did not resize output for search")
	}

	next, _ = m.Update(paneClearMsg{Name: ui.OutputPaneName})
	m = next.(*Model)
	if got := m.output.Scrollback().Count(); got != 0 {
		t.Fatalf("clear left %d scrollback rows", got)
	}
	if m.input.SearchActive() || m.searchView.focus != nil || m.searchView.priorFocus != nil {
		t.Fatalf("clear retained search state: active=%v state=%+v", m.input.SearchActive(), m.searchView)
	}
	if m.layoutPlan.output != normalOutput {
		t.Fatal("clear did not restore output geometry after closing search")
	}
	if mode := m.output.Mode(); mode != widget.ModeLive {
		t.Fatalf("clear left output window mode %v, want live", mode)
	}
	if got := m.output.NewLineCount(); got != 0 {
		t.Fatalf("clear left new-line count %d", got)
	}
	if got := m.output.Prompt(); got != "HP> " {
		t.Fatalf("clear changed prompt to %q", got)
	}
	if got := runetext.StripANSI(m.output.View()); !strings.Contains(got, "HP> ") {
		t.Fatalf("preserved prompt is absent from output view: %q", got)
	}
}

func TestOrdinaryPaneLifecycle(t *testing.T) {
	m := newBareModel(t)
	next, _ := m.Update(updateLayoutMsg(ui.LayoutTree{Root: ui.LayoutNode{
		Type: ui.LayoutTypeRow,
		Children: []ui.LayoutNode{
			{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Border: ui.PaneBorderNone},
			{Type: ui.LayoutTypePane, Name: "chat", Border: ui.PaneBorderNone},
		},
	}}))
	m = next.(*Model)

	next, _ = m.Update(paneCreateMsg{Name: "chat"})
	m = next.(*Model)
	chat, ok := m.panes["chat"]
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
	next, _ = m.Update(updateLayoutMsg(hiddenChat))
	m = next.(*Model)
	next, _ = m.Update(paneWriteMsg{Name: "chat", Text: "oldest\nmiddle\nnewest"})
	m = next.(*Model)
	chat.SetSize(20, 3)
	if got := chat.View(); got != "oldest\nmiddle\nnewest" {
		t.Fatalf("hidden ordinary pane did not buffer writes: %q", got)
	}
	for _, leaf := range m.layoutPlan.leaves {
		if leaf.node.Type == ui.LayoutTypePane && leaf.widget == chat {
			t.Fatal("hidden pane placement still resolved")
		}
	}

	shownChat, _, _ := m.layout.WithPaneVisibility("chat", true)
	next, _ = m.Update(updateLayoutMsg(shownChat))
	m = next.(*Model)
	findLeaf(t, m.layoutPlan, ui.LayoutTypePane, "chat")
	next, _ = m.Update(paneScrollToTopMsg{Name: "chat"})
	m = next.(*Model)
	if !strings.Contains(chat.Title(), "scroll") {
		t.Fatalf("ordinary pane title does not expose scrolled state: %q", chat.Title())
	}
	next, _ = m.Update(paneScrollToBottomMsg{Name: "chat"})
	m = next.(*Model)
	if got := chat.Title(); got != "chat" {
		t.Fatalf("ordinary pane did not return to live state: %q", got)
	}

	_, _ = m.Update(paneClearMsg{Name: "chat"})
	chat.SetSize(20, 1)
	if got := chat.View(); got != "" {
		t.Fatalf("ordinary pane clear left content %q", got)
	}
}

// TestReplaceOutputPaneIsOneClearAndWrite: a replace behaves like clear
// (search dropped, output window live, prompt kept) and
// then holds only the new rows, all within one Update.
func TestReplaceOutputPaneIsOneClearAndWrite(t *testing.T) {
	m := newBareModel(t)
	normalOutput := m.layoutPlan.output

	next, _ := m.Update(setPromptMsg("HP> "))
	m = next.(*Model)
	var rows []string
	for i := 0; i < 40; i++ {
		rows = append(rows, fmt.Sprintf("row %02d", i))
	}
	rows[8] = "hidden thief"
	next, _ = m.Update(paneWriteMsg{Name: ui.OutputPaneName, Text: strings.Join(rows, "\n")})
	m = next.(*Model)
	next, _ = m.Update(paneScrollToTopMsg{Name: ui.OutputPaneName})
	m = next.(*Model)
	next, _ = m.Update(showSearchMsg{options: ui.SearchOptions{Query: "thief"}})
	m = next.(*Model)
	if !m.input.SearchActive() || m.output.Mode() != widget.ModeScrolled {
		t.Fatal("test setup did not scroll and search output")
	}
	if m.layoutPlan.output == normalOutput {
		t.Fatal("test setup did not resize output for search")
	}
	next, _ = m.Update(paneReplaceMsg{Name: ui.OutputPaneName, Text: "first\nsecond"})
	m = next.(*Model)
	if got := m.output.Scrollback().Count(); got != 2 {
		t.Fatalf("replace left %d scrollback rows, want the two new rows", got)
	}
	if got := m.output.Scrollback().At(0) + "|" + m.output.Scrollback().At(1); got != "first|second" {
		t.Fatalf("replace rows = %q", got)
	}
	if m.input.SearchActive() || m.searchView.focus != nil {
		t.Fatal("replace retained search state")
	}
	if m.layoutPlan.output != normalOutput {
		t.Fatal("replace did not restore output geometry after closing search")
	}
	if mode := m.output.Mode(); mode != widget.ModeLive {
		t.Fatalf("replace left output window mode %v, want live", mode)
	}
	if got := m.output.Prompt(); got != "HP> " {
		t.Fatalf("replace dropped the live prompt: %q", got)
	}
}

// TestReplaceOrdinaryPaneCreatesAndSnapsToLive covers the non-output pane
// contract: replace creates a missing buffer like write and returns a
// scrolled pane to live tailing with only the new content.
func TestReplaceOrdinaryPaneCreatesAndSnapsToLive(t *testing.T) {
	m := newBareModel(t)
	next, _ := m.Update(paneReplaceMsg{Name: "status", Text: "HP 10"})
	m = next.(*Model)
	status, ok := m.panes["status"]
	if !ok {
		t.Fatal("replace did not create the pane")
	}
	status.SetSize(20, 1)
	if got := status.View(); got != "HP 10" {
		t.Fatalf("created pane content = %q", got)
	}

	next, _ = m.Update(paneWriteMsg{Name: "status", Text: "a\nb\nc"})
	m = next.(*Model)
	next, _ = m.Update(paneScrollToTopMsg{Name: "status"})
	m = next.(*Model)
	if !strings.Contains(status.Title(), "scroll") {
		t.Fatal("test setup did not scroll the pane")
	}
	m.Update(paneReplaceMsg{Name: "status", Text: "HP 11\nMP 5"})
	if got := status.Title(); got != "status" {
		t.Fatalf("replace left the pane scrolled: %q", got)
	}
	status.SetSize(20, 2)
	if got := status.View(); got != "HP 11\nMP 5" {
		t.Fatalf("replaced pane content = %q", got)
	}
}
