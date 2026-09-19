package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/mmcdole/rune/ui"
)

// newFramedModel builds a sized model with the production frame clock, then
// closes the frame its setup opened so each test starts idle.
func newFramedModel(t testing.TB) *Model {
	t.Helper()
	m := NewModel(make(chan ui.UIEvent, 4096))
	m.frameInterval = defaultFrameInterval
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(frameMsg{})
	if m.frameOpen || m.stale {
		t.Fatal("test setup did not settle the frame clock")
	}
	return m
}

// TestFirstChangeComposesImmediately verifies the idle->hot transition: a
// change arriving with no frame open is on screen right away (not parked
// until a tick) and opens a frame for what follows.
func TestFirstChangeComposesImmediately(t *testing.T) {
	m := newFramedModel(t)
	before := m.compositions

	_, cmd := m.Update(ui.PrintLineMsg("hello"))
	if m.compositions != before+1 || !strings.Contains(m.View().Content, "hello") {
		t.Fatal("first line was not composed immediately")
	}
	if cmd == nil || !m.frameOpen {
		t.Fatal("first change did not open a frame")
	}
}

// TestChangesInsideFrameComposeOnce verifies every message type is throttled
// by the same rule: state applies at once, the screen waits for the frame.
func TestChangesInsideFrameComposeOnce(t *testing.T) {
	m := newFramedModel(t)
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
			t.Fatalf("%T scheduled a second tick inside an open frame", msg)
		}
		if got := m.View().Content; got != first {
			t.Fatalf("%T changed the screen inside an open frame", msg)
		}
	}
	if m.compositions != composed {
		t.Fatalf("composed %d times inside one frame", m.compositions-composed)
	}
	wantScrollback(t, m, "first-visible", "server-line", "echo-line", "committed-prompt")

	_, cmd := m.Update(frameMsg{})
	if m.compositions != composed+1 {
		t.Fatalf("frame close composed %d times, want 1", m.compositions-composed)
	}
	if cmd == nil {
		t.Fatal("frame close with pending changes did not re-arm")
	}
	got := m.View().Content
	for _, want := range []string{"server-line", "echo-line", "committed-prompt", "typed-draft"} {
		if !strings.Contains(got, want) {
			t.Fatalf("frame close did not show %q", want)
		}
	}
}

// TestFrameChainStopsWhenIdle verifies that a frame closing over pending
// changes re-arms once, while the first frame with nothing pending ends the
// chain. An idle client must have no standing timer.
func TestFrameChainStopsWhenIdle(t *testing.T) {
	m := newFramedModel(t)
	m.Update(ui.PrintLineMsg("line 1"))
	m.Update(ui.PrintLineMsg("line 2"))

	if _, cmd := m.Update(frameMsg{}); cmd == nil {
		t.Fatal("frame with pending changes did not re-arm")
	}
	composed := m.compositions
	if _, cmd := m.Update(frameMsg{}); cmd != nil {
		t.Fatal("frame with nothing pending did not stop the chain")
	}
	if m.frameOpen || m.compositions != composed {
		t.Fatal("idle frame close left the clock running or composed again")
	}
}

// TestFrameNeverShowsStaleGeometry verifies a change that was never composed
// survives messages that arrive inside the same frame, including a resize
// and a clear.
func TestFrameNeverShowsStaleGeometry(t *testing.T) {
	m := newFramedModel(t)
	m.Update(ui.PrintLineMsg("first-visible"))
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 12})
	m.Update(ui.PrintLineMsg("after-resize"))
	m.Update(frameMsg{})
	assertExactBlock(t, m.View().Content, 40, 12)

	m.Update(ui.PaneClearMsg{Name: ui.OutputPaneName})
	m.Update(frameMsg{})
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

// TestScrollStateReportsOnlyChangesOncePerFrame pins the Session-facing
// contract: each report costs a Lua bar render, so an unchanged value posts
// nothing and a value changing on every line posts once per frame.
func TestScrollStateReportsOnlyChangesOncePerFrame(t *testing.T) {
	events := make(chan ui.UIEvent, 4096)
	m := NewModel(events)
	m.frameInterval = defaultFrameInterval
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	for range 100 {
		m.Update(ui.PrintLineMsg("history"))
	}
	m.Update(frameMsg{})
	m.Update(frameMsg{})
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
		t.Fatalf("lines inside a frame posted %d scroll reports, want 0", len(got))
	}
	m.Update(frameMsg{})
	if got := drainScrollReports(events); len(got) != 1 || got[0].NewLines != 50 {
		t.Fatalf("frame close reported %v, want one report of 50 new lines", got)
	}
}

// TestScrollStateRetriesAfterFullQueue verifies a report the Session queue
// rejected is not lost: the frame chain stays alive until it is accepted.
func TestScrollStateRetriesAfterFullQueue(t *testing.T) {
	events := make(chan ui.UIEvent, 1)
	m := NewModel(events)
	m.frameInterval = defaultFrameInterval
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24}) // fills the queue
	for range 100 {
		m.Update(ui.PrintLineMsg("history"))
	}
	m.Update(frameMsg{})
	m.Update(frameMsg{})

	m.Update(ui.PaneScrollUpMsg{Name: ui.OutputPaneName, Lines: 5})
	if _, cmd := m.Update(frameMsg{}); cmd == nil {
		t.Fatal("frame chain ended with scroll state still unreported")
	}
	<-events
	m.Update(frameMsg{})
	if got := drainScrollReports(events); len(got) != 1 || got[0].Mode != "scrolled" {
		t.Fatalf("retry reported %v, want one scrolled report", got)
	}
	m.Update(frameMsg{})
	if _, cmd := m.Update(frameMsg{}); cmd != nil {
		t.Fatal("frame chain kept running after the report was accepted")
	}
}

// TestUnthrottledViewComposesEveryCall covers the zero-interval mode used by
// in-process tests: no ticks, and View always reflects current state.
func TestUnthrottledViewComposesEveryCall(t *testing.T) {
	m := newBareModel(t)
	if _, cmd := m.Update(ui.PrintLineMsg("one")); cmd != nil {
		t.Fatal("zero frame interval scheduled a tick")
	}
	m.Update(ui.PrintLineMsg("two"))
	if got := m.View().Content; !strings.Contains(got, "one") || !strings.Contains(got, "two") {
		t.Fatal("unthrottled view is stale")
	}
}

// BenchmarkOutputFlood is the realistic flood shape: many server lines per
// frame, each network batch ending in a prompt update, one frame close.
func BenchmarkOutputFlood(b *testing.B) {
	m := newFramedModel(b)
	m.Update(tea.WindowSizeMsg{Width: 270, Height: 66})
	m.Update(frameMsg{})
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
			m.Update(frameMsg{})
			m.View()
		}
	}
}

// BenchmarkIdleMessage is the unthrottled cost: one change from idle, composed
// immediately.
func BenchmarkIdleMessage(b *testing.B) {
	m := newFramedModel(b)
	m.Update(tea.WindowSizeMsg{Width: 270, Height: 66})
	m.Update(frameMsg{})
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		m.Update(ui.PrintLineMsg("a line of MUD output"))
		m.View()
		m.Update(frameMsg{})
	}
}
