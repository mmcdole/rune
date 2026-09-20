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
	m.Update(ui.SetInputMsg("unchanged\nmultiline draft"))
	for _, msg := range []tea.Msg{
		ui.PrintLineMsg("server text"), ui.EchoLineMsg("local echo"),
		ui.SetPromptMsg("HP>"), ui.CommitPromptMsg("HP>"),
		ui.PaneWriteMsg{Name: "chat", Text: "chat message"},
		ui.PaneReplaceMsg{Name: "chat", Text: "replacement"},
		ui.PaneClearMsg{Name: "chat"},
		ui.PrintLineMsg(strings.Repeat("history\n", 40)),
		ui.PaneScrollUpMsg{Name: ui.OutputPaneName, Lines: 5},
		ui.PaneScrollDownMsg{Name: ui.OutputPaneName, Lines: 2},
		ui.PaneScrollToTopMsg{Name: ui.OutputPaneName},
		ui.PaneScrollToBottomMsg{Name: ui.OutputPaneName},
		tea.MouseWheelMsg{Button: tea.MouseWheelUp},
		tea.MouseWheelMsg{Button: tea.MouseWheelDown},
		ui.UpdateBindsMsg{},
		ui.UpdateConfigMsg{Mouse: true, Numpad: true},
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
		if _, ok := msg.(ui.UpdateConfigMsg); ok && (view.MouseMode != tea.MouseModeCellMotion || !view.KeyboardEnhancements.ReportAllKeysAsEscapeCodes) {
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
			m.Update(ui.PaneWriteMsg{Name: "chat", Text: "one\ntwo\nthree"})
			after := findLeaf(t, m.layoutPlan, ui.LayoutTypePane, "chat").outer.Dy()
			if after <= before {
				t.Fatalf("auto-sized content did not grow: before=%d after=%d", before, after)
			}
			m.Update(ui.PaneReplaceMsg{Name: "chat", Text: "replacement"})
			replaced := findLeaf(t, m.layoutPlan, ui.LayoutTypePane, "chat").outer.Dy()
			if replaced >= after || !strings.Contains(m.View().Content, "replacement") {
				t.Fatalf("replacement did not shrink and repaint the pane: height=%d, previous=%d", replaced, after)
			}
			m.Update(ui.PaneClearMsg{Name: "chat"})
			cleared := findLeaf(t, m.layoutPlan, ui.LayoutTypePane, "chat").outer.Dy()
			if cleared != before || strings.Contains(m.View().Content, "replacement") {
				t.Fatalf("clear did not restore the empty pane: height=%d, want=%d", cleared, before)
			}
		})
	}
}

func TestUnchangedMessagesDoNotScheduleRender(t *testing.T) {
	m := newThrottledModel(t)
	m.Update(ui.SetPromptMsg("HP>"))
	m.Update(ui.UpdateBarsMsg{"status": {Left: "ready"}})
	m.Update(renderTick{})
	m.Update(renderTick{})
	before := m.renders
	for _, msg := range []tea.Msg{
		ui.SetPromptMsg("HP>"), ui.UpdateBarsMsg{"status": {Left: "ready"}},
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
	m.Update(ui.PaneReplaceMsg{Name: "chat", Text: "REMOVED-PANE"})
	if !strings.Contains(m.View().Content, "REMOVED-PANE") {
		t.Fatal("setup did not render the pane")
	}
	m.Update(ui.UpdateLayoutMsg(ui.DefaultLayoutTree()))
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
			{"output", ui.PrintLineMsg("server text"), false},
			{"echo", ui.EchoLineMsg("local echo"), false},
			{"prompt", ui.SetPromptMsg("HP>"), false},
			{"commit", ui.CommitPromptMsg("HP>"), false},
			{"auto", ui.PaneWriteMsg{Name: "chat", Text: "one\ntwo\nthree"}, true},
			{"replace_auto", ui.PaneReplaceMsg{Name: "chat", Text: "replacement"}, true},
			{"clear_auto", ui.PaneClearMsg{Name: "chat"}, true},
			{"hidden", ui.PaneWriteMsg{Name: "hidden", Text: "one\ntwo"}, false},
			{"replace_hidden", ui.PaneReplaceMsg{Name: "hidden", Text: "replacement"}, false},
			{"clear_hidden", ui.PaneClearMsg{Name: "hidden"}, false},
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
