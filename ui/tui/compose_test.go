package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/mmcdole/rune/ui"
)

// newThrottledModel builds a sized model with the production compose interval,
// then ends the throttle window its setup started so each test starts idle.
func newThrottledModel(t testing.TB) *Model {
	t.Helper()
	m := NewModel(make(chan ui.UIEvent, 4096))
	m.composeInterval = defaultComposeInterval
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(composeTick{})
	if m.throttled || m.stale {
		t.Fatal("test setup did not settle the compose throttle")
	}
	return m
}

// TestFirstChangeComposesImmediately verifies the idle->hot transition: a
// change arriving while unthrottled is on screen right away (not parked
// until a tick) and starts a throttle window for what follows.
func TestFirstChangeComposesImmediately(t *testing.T) {
	m := newThrottledModel(t)
	before := m.compositions

	_, cmd := m.Update(ui.PrintLineMsg("hello"))
	if m.compositions != before+1 || !strings.Contains(m.View().Content, "hello") {
		t.Fatal("first line was not composed immediately")
	}
	if cmd == nil || !m.throttled {
		t.Fatal("first change did not start a throttle window")
	}
}

// TestThrottledChangesComposeOnce verifies every message type is throttled
// by the same rule: state applies at once, the screen waits for the tick.
func TestThrottledChangesComposeOnce(t *testing.T) {
	m := newThrottledModel(t)
	m.Update(ui.PrintLineMsg("first-visible"))
	first := m.View().Content
	composed := m.compositions

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
	if m.compositions != composed {
		t.Fatalf("composed %d times inside one interval", m.compositions-composed)
	}
	wantScrollback(t, m, "first-visible", "server-line", "echo-line", "committed-prompt")

	_, cmd := m.Update(composeTick{})
	if m.compositions != composed+1 {
		t.Fatalf("tick composed %d times, want 1", m.compositions-composed)
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

	if _, cmd := m.Update(composeTick{}); cmd == nil {
		t.Fatal("tick with pending changes did not re-arm")
	}
	composed := m.compositions
	if _, cmd := m.Update(composeTick{}); cmd != nil {
		t.Fatal("tick with nothing pending did not stop the chain")
	}
	if m.throttled || m.compositions != composed {
		t.Fatal("idle tick left the throttle on or composed again")
	}
}

// TestThrottleNeverShowsStaleGeometry verifies a change that was never composed
// survives messages that arrive inside the same interval, including a resize
// and a clear.
func TestThrottleNeverShowsStaleGeometry(t *testing.T) {
	m := newThrottledModel(t)
	m.Update(ui.PrintLineMsg("first-visible"))
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 12})
	m.Update(ui.PrintLineMsg("after-resize"))
	m.Update(composeTick{})
	assertExactBlock(t, m.View().Content, 40, 12)

	m.Update(ui.PaneClearMsg{Name: ui.OutputPaneName})
	m.Update(composeTick{})
	if strings.Contains(m.View().Content, "first-visible") {
		t.Fatal("clear left stale rows on screen")
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
	m.composeInterval = defaultComposeInterval
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	for range 100 {
		m.Update(ui.PrintLineMsg("history"))
	}
	m.Update(composeTick{})
	m.Update(composeTick{})
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
	m.Update(composeTick{})
	if got := drainScrollReports(events); len(got) != 1 || got[0].NewLines != 50 {
		t.Fatalf("tick reported %v, want one report of 50 new lines", got)
	}
}

// TestScrollStateRetriesAfterFullQueue verifies a report the Session queue
// rejected is not lost: the tick chain stays alive until it is accepted.
func TestScrollStateRetriesAfterFullQueue(t *testing.T) {
	events := make(chan ui.UIEvent, 1)
	m := NewModel(events)
	m.composeInterval = defaultComposeInterval
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24}) // fills the queue
	for range 100 {
		m.Update(ui.PrintLineMsg("history"))
	}
	m.Update(composeTick{})
	m.Update(composeTick{})

	m.Update(ui.PaneScrollUpMsg{Name: ui.OutputPaneName, Lines: 5})
	if _, cmd := m.Update(composeTick{}); cmd == nil {
		t.Fatal("tick chain ended with scroll state still unreported")
	}
	<-events
	m.Update(composeTick{})
	if got := drainScrollReports(events); len(got) != 1 || got[0].Mode != "scrolled" {
		t.Fatalf("retry reported %v, want one scrolled report", got)
	}
	m.Update(composeTick{})
	if _, cmd := m.Update(composeTick{}); cmd != nil {
		t.Fatal("tick chain kept running after the report was accepted")
	}
}

// TestUnthrottledViewComposesEveryCall covers the zero-interval mode used by
// in-process tests: no ticks, and View always reflects current state.
func TestUnthrottledViewComposesEveryCall(t *testing.T) {
	m := newBareModel(t)
	if _, cmd := m.Update(ui.PrintLineMsg("one")); cmd != nil {
		t.Fatal("zero compose interval scheduled a tick")
	}
	m.Update(ui.PrintLineMsg("two"))
	if got := m.View().Content; !strings.Contains(got, "one") || !strings.Contains(got, "two") {
		t.Fatal("unthrottled view is stale")
	}
}

// BenchmarkOutputFlood is the realistic flood shape: many server lines per
// interval, each network batch ending in a prompt update, one tick.
func BenchmarkOutputFlood(b *testing.B) {
	m := newThrottledModel(b)
	m.Update(tea.WindowSizeMsg{Width: 270, Height: 66})
	m.Update(composeTick{})
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
			m.Update(composeTick{})
			m.View()
		}
	}
}

// BenchmarkIdleMessage is the unthrottled cost: one change from idle, composed
// immediately.
func BenchmarkIdleMessage(b *testing.B) {
	m := newThrottledModel(b)
	m.Update(tea.WindowSizeMsg{Width: 270, Height: 66})
	m.Update(composeTick{})
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		m.Update(ui.PrintLineMsg("a line of MUD output"))
		m.View()
		m.Update(composeTick{})
	}
}

// BenchmarkCompose is one full-screen paint at a large terminal size: colored
// scrollback, a framed side pane with a title, dividers, and a prompt.
func BenchmarkCompose(b *testing.B) {
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
		m.compose()
	}
}
