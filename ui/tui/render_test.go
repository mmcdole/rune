package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mmcdole/rune/ui"
)

// newThrottledModel builds a sized model with the production render interval,
// then ends the throttle window its setup started so each test starts idle.
func newThrottledModel(t testing.TB) *Model {
	t.Helper()
	m := NewModel(make(chan ui.UIEvent, 4096))
	m.renderInterval = defaultRenderInterval
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(renderTick{})
	if m.throttled || m.dirty {
		t.Fatal("test setup did not settle the render throttle")
	}
	return m
}

// TestFirstChangeRendersImmediately verifies the idle->hot transition: a
// change arriving while unthrottled is on screen right away (not parked
// until a tick) and starts a throttle window for what follows.
func TestFirstChangeRendersImmediately(t *testing.T) {
	m := newThrottledModel(t)
	before := m.renders

	_, cmd := m.Update(ui.PrintLineMsg("hello"))
	if m.renders != before+1 || !strings.Contains(m.View().Content, "hello") {
		t.Fatal("first line was not rendered immediately")
	}
	if cmd == nil || !m.throttled {
		t.Fatal("first change did not start a throttle window")
	}
}

// TestThrottledChangesRenderOnce verifies every message type is throttled
// by the same rule: state applies at once, the screen waits for the tick.
func TestThrottledChangesRenderOnce(t *testing.T) {
	m := newThrottledModel(t)
	m.Update(ui.PrintLineMsg("first-visible"))
	first := m.View().Content
	rendered := m.renders

	for _, msg := range []tea.Msg{
		ui.PrintLineMsg("server-line"),
		ui.EchoLineMsg("echo-line"),
		ui.SetPromptMsg("prompt-text"),
		ui.CommitPromptMsg("committed-prompt"),
		ui.SetInputMsg("typed-draft"),
		ui.PaneWriteMsg{Name: "unplaced", Text: "pane-row"},
		ui.UpdateBarsMsg{},
	} {
		if _, cmd := m.Update(msg); cmd != nil {
			t.Fatalf("%T scheduled a second tick while throttled", msg)
		}
		if got := m.View().Content; got != first {
			t.Fatalf("%T changed the screen while throttled", msg)
		}
	}
	if m.renders != rendered {
		t.Fatalf("rendered %d times inside one interval", m.renders-rendered)
	}
	wantScrollback(t, m, "first-visible", "server-line", "echo-line", "committed-prompt")

	_, cmd := m.Update(renderTick{})
	if m.renders != rendered+1 {
		t.Fatalf("tick rendered %d times, want 1", m.renders-rendered)
	}
	if cmd == nil {
		t.Fatal("tick with pending changes did not re-arm")
	}
	got := m.View().Content
	for _, want := range []string{"server-line", "echo-line", "committed-prompt", "typed-draft"} {
		if !strings.Contains(got, want) {
			t.Fatalf("tick did not show %q", want)
		}
	}
}

// TestTickChainStopsWhenIdle verifies that a tick finding pending changes
// re-arms once, while the first tick with nothing pending ends the
// chain. An idle client must have no standing timer.
func TestTickChainStopsWhenIdle(t *testing.T) {
	m := newThrottledModel(t)
	m.Update(ui.PrintLineMsg("line 1"))
	m.Update(ui.PrintLineMsg("line 2"))

	if _, cmd := m.Update(renderTick{}); cmd == nil {
		t.Fatal("tick with pending changes did not re-arm")
	}
	rendered := m.renders
	if _, cmd := m.Update(renderTick{}); cmd != nil {
		t.Fatal("tick with nothing pending did not stop the chain")
	}
	if m.throttled || m.renders != rendered {
		t.Fatal("idle tick left the throttle on or rendered again")
	}
}

// TestThrottleNeverShowsStaleGeometry verifies a change that was never rendered
// survives messages that arrive inside the same interval, including a resize
// and a clear.
func TestThrottleNeverShowsStaleGeometry(t *testing.T) {
	m := newThrottledModel(t)
	m.Update(ui.PrintLineMsg("first-visible"))
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 12})
	m.Update(ui.PrintLineMsg("after-resize"))
	m.Update(renderTick{})
	assertExactBlock(t, m.View().Content, 40, 12)

	m.Update(ui.PaneClearMsg{Name: ui.OutputPaneName})
	m.Update(renderTick{})
	if strings.Contains(m.View().Content, "first-visible") {
		t.Fatal("clear left dirty rows on screen")
	}
}

// drainScrollReports empties the event queue and returns its scroll reports.
func drainScrollReports(events chan ui.UIEvent) (reports []ui.ScrollStateChangedMsg) {
	for {
		select {
		case event := <-events:
			if report, ok := event.(ui.ScrollStateChangedMsg); ok {
				reports = append(reports, report)
			}
		default:
			return reports
		}
	}
}

// TestScrollStateReportsOnlyChangesOncePerInterval pins the Session-facing
// contract: each report costs a Lua bar render, so an unchanged value posts
// nothing and a value changing on every line posts once per interval.
func TestScrollStateReportsOnlyChangesOncePerInterval(t *testing.T) {
	events := make(chan ui.UIEvent, 4096)
	m := NewModel(events)
	m.renderInterval = defaultRenderInterval
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	for range 100 {
		m.Update(ui.PrintLineMsg("history"))
	}
	m.Update(renderTick{})
	m.Update(renderTick{})
	if got := drainScrollReports(events); len(got) != 0 {
		t.Fatalf("live output posted unchanged scroll state: %v", got)
	}

	m.Update(ui.PaneScrollUpMsg{Name: ui.OutputPaneName, Lines: 5})
	if got := drainScrollReports(events); len(got) != 1 || got[0].Mode != "scrolled" {
		t.Fatalf("scrolling from idle reported %v, want one scrolled report", got)
	}
	for range 50 {
		m.Update(ui.PrintLineMsg("flood"))
	}
	if got := drainScrollReports(events); len(got) != 0 {
		t.Fatalf("lines posted %d scroll reports while throttled, want 0", len(got))
	}
	m.Update(renderTick{})
	if got := drainScrollReports(events); len(got) != 1 || got[0].NewLines != 50 {
		t.Fatalf("tick reported %v, want one report of 50 new lines", got)
	}
}

// TestScrollStateRetriesAfterFullQueue verifies a report the Session queue
// rejected is not lost: the tick chain stays alive until it is accepted.
func TestScrollStateRetriesAfterFullQueue(t *testing.T) {
	events := make(chan ui.UIEvent, 1)
	m := NewModel(events)
	m.renderInterval = defaultRenderInterval
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24}) // fills the queue
	for range 100 {
		m.Update(ui.PrintLineMsg("history"))
	}
	m.Update(renderTick{})
	m.Update(renderTick{})

	m.Update(ui.PaneScrollUpMsg{Name: ui.OutputPaneName, Lines: 5})
	if _, cmd := m.Update(renderTick{}); cmd == nil {
		t.Fatal("tick chain ended with scroll state still unreported")
	}
	<-events
	m.Update(renderTick{})
	if got := drainScrollReports(events); len(got) != 1 || got[0].Mode != "scrolled" {
		t.Fatalf("retry reported %v, want one scrolled report", got)
	}
	m.Update(renderTick{})
	if _, cmd := m.Update(renderTick{}); cmd != nil {
		t.Fatal("tick chain kept running after the report was accepted")
	}
}

// TestUnthrottledViewRendersEveryCall covers the zero-interval mode used by
// in-process tests: no ticks, and View always reflects current state.
func TestUnthrottledViewRendersEveryCall(t *testing.T) {
	m := newBareModel(t)
	if _, cmd := m.Update(ui.PrintLineMsg("one")); cmd != nil {
		t.Fatal("zero render interval scheduled a tick")
	}
	m.Update(ui.PrintLineMsg("two"))
	if got := m.View().Content; !strings.Contains(got, "one") || !strings.Contains(got, "two") {
		t.Fatal("unthrottled view is dirty")
	}
}

// BenchmarkOutputFlood is the realistic flood shape: many server lines per
// interval, each network batch ending in a prompt update, one tick.
func BenchmarkOutputFlood(b *testing.B) {
	m := newThrottledModel(b)
	m.Update(tea.WindowSizeMsg{Width: 270, Height: 66})
	m.Update(renderTick{})
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		m.Update(ui.PrintLineMsg("a line of MUD output"))
		m.View()
		if i%10 == 9 {
			m.Update(ui.SetPromptMsg("HP:100 >"))
			m.View()
		}
		if i%100 == 99 {
			m.Update(renderTick{})
			m.View()
		}
	}
}

// BenchmarkIdleMessage is the unthrottled cost: one change from idle, rendered
// immediately.
func BenchmarkIdleMessage(b *testing.B) {
	m := newThrottledModel(b)
	m.Update(tea.WindowSizeMsg{Width: 270, Height: 66})
	m.Update(renderTick{})
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		m.Update(ui.PrintLineMsg("a line of MUD output"))
		m.View()
		m.Update(renderTick{})
	}
}

// BenchmarkScreen is one full-screen render at a large terminal size: colored
// scrollback, a framed side pane with a title, dividers, and a prompt.
func BenchmarkScreen(b *testing.B) {
	m := NewModel(make(chan ui.UIEvent, 4096))
	m.Update(tea.WindowSizeMsg{Width: 270, Height: 66})
	m.Update(ui.UpdateLayoutMsg{Root: ui.LayoutNode{Type: ui.LayoutTypeColumn, Children: []ui.LayoutNode{
		{Type: ui.LayoutTypeRow, Dividers: true, Children: []ui.LayoutNode{
			{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Border: ui.PaneBorderNone},
			{Type: ui.LayoutTypePane, Name: "chat", Size: ui.Cells(60)},
		}},
		{Type: ui.LayoutTypeInput, Size: ui.AutoSize()},
	}}})
	for i := range 200 {
		line := fmt.Sprintf("\x1b[32mline %d\x1b[0m some \x1b[1;31mcolored\x1b[0m output that is long enough to be realistic", i)
		m.Update(ui.PrintLineMsg(line))
		m.Update(ui.PaneWriteMsg{Name: "chat", Text: line})
	}
	m.Update(ui.SetPromptMsg("HP:100 >"))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		m.render()
	}
}
