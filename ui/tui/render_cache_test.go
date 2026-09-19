package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/mmcdole/rune/ui"
)

type renderCountPane struct {
	paneResource
	views int
}

func (p *renderCountPane) View() string { p.views++; return p.paneResource.View() }

func TestBufferedOutputReusesScreen(t *testing.T) {
	m := newBareModel(t)
	probe := &renderCountPane{paneResource: m.panes.Create("probe")}
	m.panes.byName["probe"] = probe
	m.Update(ui.UpdateLayoutMsg{Root: ui.LayoutNode{Type: ui.LayoutTypeColumn, Children: []ui.LayoutNode{
		{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Border: ui.PaneBorderNone},
		{Type: ui.LayoutTypePane, Name: "probe", Size: ui.Cells(2), Border: ui.PaneBorderNone},
		{Type: ui.LayoutTypeInput, Size: ui.AutoSize()},
	}}})
	m.Update(ui.PrintLineMsg("first-visible"))
	first := m.View().Content
	if !strings.Contains(first, "first-visible") {
		t.Fatal("first line was delayed")
	}
	views := probe.views
	if views == 0 {
		t.Fatal("probe pane was not rendered")
	}
	for range 30 {
		m.Update(ui.PrintLineMsg("pending-output"))
		if got := m.View().Content; got != first {
			t.Fatal("buffered output changed the visible screen")
		}
	}
	if probe.views != views {
		t.Fatalf("buffered output recomposed screen %d times", probe.views-views)
	}
	// Input remains responsive while output waits for its batch.
	m.Update(ui.SetInputMsg("typing-now"))
	if !strings.Contains(m.View().Content, "typing-now") {
		t.Fatal("input did not redraw during batch")
	}
	m.Update(tickMsg{generation: m.output.batchGeneration})
	if !strings.Contains(m.View().Content, "pending-output") {
		t.Fatal("batch did not redraw")
	}
}

func TestOutputBatchViewInvalidation(t *testing.T) {
	for _, tt := range []struct {
		name string
		msg  tea.Msg
		want string
	}{
		{"echo", ui.EchoLineMsg("echo-now"), "echo-now"},
		{"prompt", ui.SetPromptMsg("prompt-now"), "prompt-now"},
		{"commit", ui.CommitPromptMsg("commit-now"), "commit-now"},
		{"replace", ui.PaneReplaceMsg{Name: ui.OutputPaneName, Text: "replacement-now"}, "replacement-now"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := newBareModel(t)
			m.Update(ui.PrintLineMsg("first"))
			m.View()
			m.Update(ui.PrintLineMsg("pending"))
			m.View()
			m.Update(tt.msg)
			if !strings.Contains(m.View().Content, tt.want) {
				t.Fatal("visible update reused stale content")
			}
		})
	}
}

func BenchmarkBufferedOutputView(b *testing.B) {
	m := NewModel(make(chan ui.UIEvent, 4096))
	m.Update(tea.WindowSizeMsg{Width: 270, Height: 66})
	m.Update(ui.PrintLineMsg("first"))
	m.View()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		m.Update(ui.PrintLineMsg("a pending line of MUD output"))
		m.View()
		// Bound retained test data without flushing the displayed batch.
		m.output.pendingRows = m.output.pendingRows[:0]
	}
}

func TestBufferedOutputPreservesUnrenderedChanges(t *testing.T) {
	m := newBareModel(t)
	m.Update(ui.PrintLineMsg("first-visible"))
	m.Update(ui.PrintLineMsg("pending-output"))
	// No View occurred before the buffered message: there is no cache to reuse.
	if got := m.View().Content; !strings.Contains(got, "first-visible") || strings.Contains(got, "pending-output") {
		t.Fatal("first view did not reflect the displayed buffer")
	}
	m.Update(ui.SetInputMsg("new-draft"))
	m.Update(ui.PrintLineMsg("more-pending"))
	if !strings.Contains(m.View().Content, "new-draft") {
		t.Fatal("buffered output hid an unrendered input change")
	}
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 12})
	m.Update(ui.PrintLineMsg("pending-after-resize"))
	assertExactBlock(t, m.View().Content, 40, 12)
	m.Update(ui.PaneClearMsg{Name: ui.OutputPaneName})
	if strings.Contains(m.View().Content, "first-visible") {
		t.Fatal("clear reused stale screen")
	}
}
