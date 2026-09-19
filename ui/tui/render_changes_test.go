package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mmcdole/rune/ui"
)

func TestOutputChangesDoNotResizeUnchangedWidgets(t *testing.T) {
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
		ui.PaneWriteMsg{Name: "chat", Text: "chat message"}, renderTick{},
	} {
		probe.applications = 0
		m.Update(msg)
		m.View()
		if probe.applications != 0 {
			t.Fatalf("%T resized unchanged widgets %d times", msg, probe.applications)
		}
	}
}

func TestContentSizedPaneStillGrowsOnAppend(t *testing.T) {
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
