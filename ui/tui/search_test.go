package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	runetext "github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/widget"
)

func TestGrowingOutputOutputWindowPublishesLiveState(t *testing.T) {
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
		next, _ := m.Update(echoLineMsg(fmt.Sprintf("line %d", i)))
		m = next.(*Model)
	}
	next, _ := m.Update(paneScrollToTopMsg{Name: ui.OutputPaneName})
	m = next.(*Model)
	if m.output.Mode() != widget.ModeScrolled {
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
	if m.output.Mode() != widget.ModeLive || m.output.NewLineCount() != 0 {
		t.Fatalf("expanded output remained scrolled: mode=%v new=%d",
			m.output.Mode(), m.output.NewLineCount())
	}
	if view := runetext.StripANSI(m.View().Content); strings.Contains(view, "output · scroll") {
		t.Fatalf("expanded output retained dirty scroll title: %q", view)
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
		next, _ := m.Update(echoLineMsg(fmt.Sprintf("line %d", i)))
		m = next.(*Model)
	}
	next, _ := m.Update(showSearchMsg{options: ui.SearchOptions{Query: "line"}})
	m = next.(*Model)
	if !m.input.SearchActive() || m.layoutPlan.output.Dy() != 1 {
		t.Fatalf("search setup = active %v output height %d, want true and 1 (shared border)",
			m.input.SearchActive(), m.layoutPlan.output.Dy())
	}
	next, _ = m.Update(paneScrollToTopMsg{Name: ui.OutputPaneName})
	m = next.(*Model)
	if m.output.Mode() != widget.ModeScrolled {
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
	if m.output.Mode() != widget.ModeLive || m.output.NewLineCount() != 0 {
		t.Fatalf("post-search output = mode %v new %d, want live zero state",
			m.output.Mode(), m.output.NewLineCount())
	}
	if view := runetext.StripANSI(m.View().Content); strings.Contains(view, "output · scroll") {
		t.Fatalf("post-search output retained dirty title: %q", view)
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

// Search changes the input widget's intrinsic height as its result list
// grows and collapses. The output window must be sized for that final geometry
// before centering, or the selected source row can land just outside the
// visible window even though it is selected in the overlay.
func TestSearchFocusUsesFinalLayoutGeometry(t *testing.T) {
	events := make(chan ui.UIEvent, 64)
	m := NewModel(events)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = next.(*Model)
	next, _ = m.Update(updateBarsMsg{"status": {Left: "status"}})
	m = next.(*Model)

	for i := 0; i < 9; i++ {
		next, _ = m.Update(echoLineMsg(fmt.Sprintf("match %d thief", i)))
		m = next.(*Model)
	}
	next, _ = m.Update(echoLineMsg("SELECTED thief"))
	m = next.(*Model)
	for i := 0; i < 20; i++ {
		next, _ = m.Update(echoLineMsg(fmt.Sprintf("quiet %d", i)))
		m = next.(*Model)
	}

	// Establish the normal layout, then the shorter no-match navigator. Typing
	// the query expands it to its five-result maximum in one update.
	m.View()
	next, _ = m.Update(showSearchMsg{options: ui.SearchOptions{}})
	m = next.(*Model)
	m.View()
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyExtended, Text: "thief"})
	m = next.(*Model)
	m.View()

	assertOutputWindowRowCentered(t, m.output.View(), "SELECTED thief")

	// Enter removes the overlay and expands the output window. The accepted row
	// must be centered again using that post-close height.
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(*Model)
	m.View()
	assertOutputWindowRowCentered(t, m.output.View(), "SELECTED thief")
	if m.searchView.focus == nil {
		t.Fatal("accepted search should retain its active-result marker")
	}
	committedSeq := m.searchView.focus.Seq
	assertOutputWindowRowHighlighted(t, m.output.View(), "SELECTED thief")

	// A replacement search may preview another row, but cancelling it restores
	// the previously committed focus from the grouped search lifecycle state.
	next, _ = m.Update(showSearchMsg{options: ui.SearchOptions{}})
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
	assertOutputWindowRowHighlighted(t, m.output.View(), "SELECTED thief")

	// Deliberate output window navigation retires the accepted marker.
	next, _ = m.Update(updateConfigMsg{Mouse: true})
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

	next, _ := m.Update(showSearchMsg{options: ui.SearchOptions{}})
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

func TestManualOutputWindowEntryPointsClearCommittedSearchFocus(t *testing.T) {
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
			msg:  paneScrollUpMsg{Name: ui.OutputPaneName, Lines: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			if tt.mouse {
				next, _ := m.Update(updateConfigMsg{Mouse: true})
				m = next.(*Model)
			}
			focus := widget.SearchMatch{Seq: m.output.Scrollback().Seq(50)}
			m.searchView.focus = &focus
			m.output.SetHighlight(focus.Seq, focus.Ranges)

			next, _ := m.Update(tt.msg)
			m = next.(*Model)
			if m.searchView.focus != nil {
				t.Fatal("manual output window navigation retained committed search focus")
			}
		})
	}
}

func TestMouseWheelNavigatesActiveSearchMatches(t *testing.T) {
	m := newBareModel(t)
	next, _ := m.Update(updateConfigMsg{Mouse: true})
	m = next.(*Model)
	for _, line := range []string{"thief oldest", "quiet", "thief middle", "quiet", "thief newest"} {
		m.output.Write(line)
	}

	m.inputCtl.ShowSearch(ui.SearchOptions{Query: "thief"})
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

func assertOutputWindowRowCentered(t *testing.T, view, want string) {
	t.Helper()
	rows := strings.Split(view, "\n")
	for i, row := range rows {
		if runetext.StripANSI(row) != want {
			continue
		}
		center := (len(rows) - 1) / 2
		if i < center-1 || i > center+1 {
			t.Fatalf("row %q rendered at output window row %d of %d, want it centered near %d\n%s",
				want, i, len(rows), center, runetext.StripANSI(view))
		}
		return
	}
	t.Fatalf("row %q is outside the output window:\n%s", want, runetext.StripANSI(view))
}

func assertOutputWindowRowHighlighted(t *testing.T, view, want string) {
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
	t.Fatalf("row %q is outside the output window:\n%s", want, runetext.StripANSI(view))
}
