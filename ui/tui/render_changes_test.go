package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mmcdole/rune/ui"
)

func TestAppearanceChangesReuseLayout(t *testing.T) {
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 100)), 80, 24)
	probe := &countedPane{pane: m.pane("chat")}
	m.panes["chat"] = probe
	setLayout(m, ui.LayoutNode{Type: ui.LayoutTypeColumn, Children: []ui.LayoutNode{
		{Type: ui.LayoutTypePane, Name: "chat", Size: ui.Cells(4)},
		{Type: ui.LayoutTypePane, Name: ui.OutputPaneName},
		{Type: ui.LayoutTypeInput, Size: ui.AutoSize()},
	}})
	m.Update(setInputMsg("unchanged\nmultiline draft"))
	for _, msg := range []tea.Msg{
		printLineMsg("server text"), echoLineMsg("local echo"),
		setPromptMsg("HP>"), commitPromptMsg("HP>"),
		paneWriteMsg{Name: "chat", Text: "chat message"},
		paneReplaceMsg{Name: "chat", Text: "replacement"},
		paneClearMsg{Name: "chat"},
		printLineMsg(strings.Repeat("history\n", 40)),
		paneScrollUpMsg{Name: ui.OutputPaneName, Lines: 5},
		paneScrollDownMsg{Name: ui.OutputPaneName, Lines: 2},
		paneScrollToTopMsg{Name: ui.OutputPaneName},
		paneScrollToBottomMsg{Name: ui.OutputPaneName},
		tea.MouseWheelMsg{Button: tea.MouseWheelUp},
		tea.MouseWheelMsg{Button: tea.MouseWheelDown},
		updateBindsMsg{},
		updateConfigMsg{Mouse: true, Numpad: true},
		renderTick{},
	} {
		probe.applications = 0
		m.Update(msg)
		view := m.View()
		if probe.applications != 0 {
			t.Fatalf("%T resized unchanged widgets %d times", msg, probe.applications)
		}
		// Reusing geometry must paint exactly the same screen as a fresh
		// layout, including changed labels and erased pane content.
		m.applyLayout()
		m.render()
		if view.Content != m.screen {
			t.Fatalf("%T reused layout produced a stale or misplaced view", msg)
		}
		if _, ok := msg.(updateConfigMsg); ok && (view.MouseMode != tea.MouseModeCellMotion || !view.KeyboardEnhancements.ReportAllKeysAsEscapeCodes) {
			t.Fatal("config update did not apply terminal options")
		}
	}
}

func TestContentSizedPaneGrowsAndShrinks(t *testing.T) {
	for _, nested := range []bool{false, true} {
		t.Run(map[bool]string{false: "pane", true: "container"}[nested], func(t *testing.T) {
			m := resizeModel(t, NewModel(make(chan ui.UIEvent, 100)), 80, 24)
			auto := ui.LayoutNode{Type: ui.LayoutTypePane, Name: "chat", Size: ui.AutoSize()}
			if nested {
				auto.Size = ui.Fraction(1)
				auto = ui.LayoutNode{Type: ui.LayoutTypeColumn, Size: ui.AutoSize(), Children: []ui.LayoutNode{auto}}
			}
			setLayout(m, ui.LayoutNode{Type: ui.LayoutTypeColumn, Children: []ui.LayoutNode{
				auto,
				{Type: ui.LayoutTypePane, Name: ui.OutputPaneName},
				{Type: ui.LayoutTypeInput, Size: ui.AutoSize()},
			}})
			before := findLeaf(t, m.layoutPlan, ui.LayoutTypePane, "chat").outer.Dy()
			m.Update(paneWriteMsg{Name: "chat", Text: "one\ntwo\nthree"})
			after := findLeaf(t, m.layoutPlan, ui.LayoutTypePane, "chat").outer.Dy()
			if after <= before {
				t.Fatalf("auto-sized content did not grow: before=%d after=%d", before, after)
			}
			m.Update(paneReplaceMsg{Name: "chat", Text: "replacement"})
			replaced := findLeaf(t, m.layoutPlan, ui.LayoutTypePane, "chat").outer.Dy()
			if replaced >= after || !strings.Contains(m.View().Content, "replacement") {
				t.Fatalf("replacement did not shrink and repaint the pane: height=%d, previous=%d", replaced, after)
			}
			m.Update(paneClearMsg{Name: "chat"})
			cleared := findLeaf(t, m.layoutPlan, ui.LayoutTypePane, "chat").outer.Dy()
			if cleared != before || strings.Contains(m.View().Content, "replacement") {
				t.Fatalf("clear did not restore the empty pane: height=%d, want=%d", cleared, before)
			}
		})
	}
}

func TestUnchangedMessagesDoNotScheduleRender(t *testing.T) {
	m := newThrottledModel(t)
	m.Update(setPromptMsg("HP>"))
	m.Update(updateBarsMsg{"status": {Left: "ready"}})
	m.Update(renderTick{})
	m.Update(renderTick{})
	before := m.renders
	for _, msg := range []tea.Msg{
		setPromptMsg("HP>"), updateBarsMsg{"status": {Left: "ready"}},
		tea.MouseClickMsg{Button: tea.MouseLeft}, renderTick{},
	} {
		if _, cmd := m.Update(msg); cmd != nil {
			t.Fatalf("%T scheduled a render for unchanged pixels", msg)
		}
		m.View()
	}
	if m.renders != before {
		t.Fatal("unchanged pixels were rerendered")
	}
}

func TestLayoutChangeErasesRemovedPane(t *testing.T) {
	m := renderFixture(80, 24, true)
	m.Update(paneReplaceMsg{Name: "chat", Text: "REMOVED-PANE"})
	if !strings.Contains(m.View().Content, "REMOVED-PANE") {
		t.Fatal("setup did not render the pane")
	}
	m.Update(updateLayoutMsg(ui.DefaultLayoutTree()))
	if strings.Contains(m.View().Content, "REMOVED-PANE") {
		t.Fatal("removed pane left pixels on the reused canvas")
	}
}

func TestOnlyAutoPaneContentRequestsLayout(t *testing.T) {
	for _, nested := range []bool{false, true} {
		for _, tc := range []struct {
			name string
			msg  tea.Msg
			want bool
		}{
			{"output", printLineMsg("server text"), false},
			{"echo", echoLineMsg("local echo"), false},
			{"prompt", setPromptMsg("HP>"), false},
			{"commit", commitPromptMsg("HP>"), false},
			{"auto", paneWriteMsg{Name: "chat", Text: "one\ntwo\nthree"}, true},
			{"replace_auto", paneReplaceMsg{Name: "chat", Text: "replacement"}, true},
			{"clear_auto", paneClearMsg{Name: "chat"}, true},
			{"hidden", paneWriteMsg{Name: "hidden", Text: "one\ntwo"}, false},
			{"replace_hidden", paneReplaceMsg{Name: "hidden", Text: "replacement"}, false},
			{"clear_hidden", paneClearMsg{Name: "hidden"}, false},
		} {
			t.Run(fmt.Sprintf("%s/nested=%t", tc.name, nested), func(t *testing.T) {
				m := resizeModel(t, NewModel(make(chan ui.UIEvent, 100)), 80, 24)
				// A fixed neighbour remains measurable when clearing the
				// auto pane leaves it with an empty content rectangle.
				probe := &countedPane{pane: m.pane("fixed")}
				m.panes["fixed"] = probe
				auto := ui.LayoutNode{Type: ui.LayoutTypePane, Name: "chat", Size: ui.AutoSize()}
				if nested {
					auto.Size = ui.Fraction(1)
					auto = ui.LayoutNode{Type: ui.LayoutTypeColumn, Size: ui.AutoSize(), Children: []ui.LayoutNode{auto}}
				}
				setLayout(m, ui.LayoutNode{Type: ui.LayoutTypeColumn, Children: []ui.LayoutNode{
					{Type: ui.LayoutTypePane, Name: "fixed", Size: ui.Cells(3)},
					auto, {Type: ui.LayoutTypePane, Name: ui.OutputPaneName}, {Type: ui.LayoutTypeInput, Size: ui.AutoSize()},
				}})
				probe.applications = 0
				m.Update(tc.msg)
				if got := probe.applications > 0; got != tc.want {
					t.Fatalf("layout applied = %t, want %t", got, tc.want)
				}
			})
		}
	}
}

func TestInputNavigationReusesLayout(t *testing.T) {
	for _, mode := range []string{"normal", "draft", "picker", "search"} {
		t.Run(mode, func(t *testing.T) {
			m := resizeModel(t, NewModel(make(chan ui.UIEvent, 100)), 40, 16)
			probe := &countedPane{pane: m.pane("probe")}
			m.panes["probe"] = probe
			setLayout(m, ui.LayoutNode{Type: ui.LayoutTypeColumn, Children: []ui.LayoutNode{
				{Type: ui.LayoutTypePane, Name: "probe", Size: ui.Cells(3)},
				{Type: ui.LayoutTypePane, Name: ui.OutputPaneName},
				{Type: ui.LayoutTypeInput, Size: ui.AutoSize()},
			}})
			m.Update(printLineMsg("north one\nnorth two\nnorth three"))
			switch mode {
			case "normal":
				m.Update(setInputMsg("look north"))
			case "draft":
				m.Update(setInputMsg(strings.Repeat("one two three\n", 20)))
			case "picker":
				m.Update(showPickerMsg{options: ui.PickerOptions{Items: pickerTestItems}})
			case "search":
				m.Update(showSearchMsg{options: ui.SearchOptions{Query: "north"}})
			}
			for _, key := range []rune{tea.KeyLeft, tea.KeyRight, tea.KeyUp, tea.KeyDown, tea.KeyF12} {
				probe.applications = 0
				m.Update(keyPress(key))
				if probe.applications != 0 {
					t.Fatalf("%v rebuilt layout", key)
				}
				view := m.View().Content
				m.applyLayout()
				m.render()
				if view != m.View().Content {
					t.Fatalf("%v differed from a fresh layout", key)
				}
			}
		})
	}
}

func TestInputTransitionsMatchFreshLayout(t *testing.T) {
	for _, size := range [][2]int{{40, 16}, {20, 3}, {1, 1}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			m := resizeModel(t, NewModel(make(chan ui.UIEvent, 100)), size[0], size[1])
			fresh := resizeModel(t, NewModel(make(chan ui.UIEvent, 100)), size[0], size[1])
			// The second model reapplies layout after every message, including
			// cursor movement, to preserve the old navigation and scroll behavior.
			messages := []tea.Msg{
				printLineMsg("north one\nsouth two\nnorth three"),
				setInputMsg("look"), textPress("!"), tea.PasteMsg{Content: " north"},
				ctrlPress('j'), textPress("界"),
				setInputMsg(strings.Repeat("numbered draft line\n", 20)),
				ctrlPress(tea.KeyHome), keyPress(tea.KeyDown), keyPress(tea.KeyDown),
				ctrlPress(tea.KeyEnd), keyPress(tea.KeyUp), keyPress(tea.KeyUp),
				inputSetCursorMsg(0), keyPress(tea.KeyDown), keyPress(tea.KeyUp),
				keyPress(tea.KeyBackspace), keyPress(tea.KeyEsc), keyPress(tea.KeyEsc),
				showPickerMsg{options: ui.PickerOptions{Items: pickerTestItems}}, textPress("disconnect"),
				keyPress(tea.KeyBackspace), keyPress(tea.KeyEsc),
				showSearchMsg{options: ui.SearchOptions{Query: "north"}}, keyPress(tea.KeyUp),
				textPress(" missing"), keyPress(tea.KeyEsc),
			}
			for _, msg := range messages {
				m.Update(msg)
				fresh.Update(msg)
				fresh.applyLayout()
				fresh.applySearchPosition(true)
				fresh.render()
				if m.View().Content != fresh.View().Content {
					t.Fatalf("%T (%v) differed from a fresh layout", msg, msg)
				}
			}
		})
	}
}

func TestBarTextReusesLayoutAndVisibilityResizes(t *testing.T) {
	m := resizeModel(t, NewModel(make(chan ui.UIEvent, 100)), 40, 16)
	probe := &countedPane{pane: m.pane("probe")}
	m.panes["probe"] = probe
	setLayout(m, ui.LayoutNode{Type: ui.LayoutTypeColumn, Children: []ui.LayoutNode{
		{Type: ui.LayoutTypePane, Name: "probe", Size: ui.Cells(3)},
		{Type: ui.LayoutTypePane, Name: ui.OutputPaneName},
		{Type: ui.LayoutTypeBar, Name: "status", Size: ui.AutoSize()},
		{Type: ui.LayoutTypeInput, Size: ui.AutoSize()},
	}})
	for _, tc := range []struct {
		content updateBarsMsg
		layout  bool
	}{
		{updateBarsMsg{"status": {Left: "ready"}}, true},
		{updateBarsMsg{"status": {Center: "busy"}}, false},
		{updateBarsMsg{"status": {Right: "界"}}, false},
		{updateBarsMsg{"status": {Left: "\x1b[31m"}}, true},
		{updateBarsMsg{"status": {}}, false},
		{updateBarsMsg{}, false},
		{updateBarsMsg{"status": {Left: "back"}}, true},
		{updateBarsMsg{}, true},
	} {
		probe.applications = 0
		m.Update(tc.content)
		if got := probe.applications > 0; got != tc.layout {
			t.Fatalf("%v: layout = %t, want %t", tc.content, got, tc.layout)
		}
		view := m.View().Content
		m.applyLayout()
		m.render()
		if view != m.View().Content {
			t.Fatalf("%v differed from a fresh layout", tc.content)
		}
	}
}
