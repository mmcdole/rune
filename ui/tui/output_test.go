package tui

import (
	"strings"
	"testing"

	"github.com/mmcdole/rune/ui"
)

func TestStaleOutputBatchTickCannotDisturbPostClearBatch(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.PrintLineMsg("old immediate"))
	m = next.(*Model)
	next, _ = m.Update(ui.PrintLineMsg("old pending"))
	m = next.(*Model)
	staleGeneration := m.output.batchGeneration

	next, _ = m.Update(ui.PaneClearMsg{Name: ui.OutputPaneName})
	m = next.(*Model)
	next, _ = m.Update(ui.PrintLineMsg("new immediate"))
	m = next.(*Model)
	next, _ = m.Update(ui.PrintLineMsg("new pending"))
	m = next.(*Model)
	currentGeneration := m.output.batchGeneration
	if currentGeneration == staleGeneration {
		t.Fatal("clear did not invalidate the in-flight batch generation")
	}
	wantScrollback(t, m, "new immediate")

	next, cmd := m.Update(tickMsg{generation: staleGeneration})
	m = next.(*Model)
	if cmd != nil {
		t.Fatal("stale tick re-armed output batching")
	}
	if !m.output.flushScheduled || len(m.output.pendingRows) != 1 {
		t.Fatalf("stale tick disturbed current batch: scheduled=%v pending=%v", m.output.flushScheduled, m.output.pendingRows)
	}
	wantScrollback(t, m, "new immediate")

	next, cmd = m.Update(tickMsg{generation: currentGeneration})
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("current tick did not re-arm after flushing pending output")
	}
	wantScrollback(t, m, "new immediate", "new pending")
}

// TestFirstLineRendersImmediately verifies the idle->hot transition: a
// server line arriving with no batch window open is appended right
// away (not parked until a tick) and opens a window for what follows.
func TestFirstLineRendersImmediately(t *testing.T) {
	m := newBareModel(t)

	next, cmd := m.Update(ui.PrintLineMsg("hello"))
	m = next.(*Model)

	if got := m.output.buffer.Count(); got != 1 {
		t.Fatalf("expected first line appended immediately, scrollback has %d lines", got)
	}
	if cmd == nil {
		t.Fatal("expected first line to open a batch window (tick cmd)")
	}
}

// TestBurstCoalescesInBatchWindow verifies lines arriving inside an
// open batch window are held and flushed together on the tick.
func TestBurstCoalescesInBatchWindow(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.PrintLineMsg("line 1"))
	m = next.(*Model)
	next, _ = m.Update(ui.PrintLineMsg("line 2"))
	m = next.(*Model)
	next, _ = m.Update(ui.PrintLineMsg("line 3"))
	m = next.(*Model)

	if got := m.output.buffer.Count(); got != 1 {
		t.Fatalf("expected burst lines batched, scrollback has %d lines", got)
	}

	next, _ = m.Update(tickMsg{generation: m.output.batchGeneration})
	m = next.(*Model)

	if got := m.output.buffer.Count(); got != 3 {
		t.Fatalf("expected tick to flush the batch, scrollback has %d lines", got)
	}
}

// TestTickStopsWhenOutputGoesQuiet verifies that a flush re-arms the batching
// window once, while the first tick with no pending lines ends the chain. An
// idle client must have no standing timer.
func TestTickStopsWhenOutputGoesQuiet(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.PrintLineMsg("line 1"))
	m = next.(*Model)
	next, _ = m.Update(ui.PrintLineMsg("line 2"))
	m = next.(*Model)

	next, cmd := m.Update(tickMsg{generation: m.output.batchGeneration})
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("expected tick with pending lines to re-arm the window")
	}

	_, cmd = m.Update(tickMsg{generation: m.output.batchGeneration})
	if cmd != nil {
		t.Fatal("expected tick with nothing pending to stop the chain")
	}
}

// TestEchoFlushesPendingServerLines verifies a local echo cannot render
// ahead of server output that arrived before it: batched PrintLineMsg
// lines must be flushed to the scrollback before the echo is appended,
// and the now-empty trailing tick must not re-arm.
func TestEchoFlushesPendingServerLines(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.PrintLineMsg("line 1")) // immediate, opens window
	m = next.(*Model)
	next, _ = m.Update(ui.PrintLineMsg("line 2")) // batched
	m = next.(*Model)
	next, _ = m.Update(ui.EchoLineMsg("> look"))
	m = next.(*Model)

	if got := m.output.buffer.Count(); got != 3 {
		t.Fatalf("expected 3 scrollback lines, got %d", got)
	}
	for i, want := range []string{"line 1", "line 2", "> look"} {
		if got := m.output.buffer.At(i); got != want {
			t.Fatalf("scrollback[%d] = %q, want %q (echo reordered?)", i, got, want)
		}
	}

	next, cmd := m.Update(tickMsg{generation: m.output.batchGeneration})
	m = next.(*Model)
	if cmd != nil {
		t.Fatal("expected trailing tick after eager echo flush to stop the chain")
	}
	if got := m.output.buffer.Count(); got != 3 {
		t.Fatalf("trailing tick changed scrollback, got %d lines", got)
	}
}

func TestPromptCommitPrecedesFollowingRows(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.PrintLineMsg("line 1")) // immediate, opens window
	m = next.(*Model)
	next, _ = m.Update(ui.PrintLineMsg("line 2")) // batched
	m = next.(*Model)
	next, _ = m.Update(ui.SetPromptMsg("Username:"))
	m = next.(*Model)
	next, _ = m.Update(ui.CommitPromptMsg("Username:"))
	m = next.(*Model)
	next, _ = m.Update(ui.EchoLineMsg("> player"))
	m = next.(*Model)
	next, _ = m.Update(ui.PrintLineMsg("login hook sent username"))
	m = next.(*Model)
	next, _ = m.Update(tickMsg{generation: m.output.batchGeneration})
	m = next.(*Model)

	wantScrollback(t, m,
		"line 1", "line 2", "Username:", "> player", "login hook sent username")
	if m.output.promptText != "" {
		t.Fatalf("prompt overlay = %q after commit, want empty", m.output.promptText)
	}
}

func TestOrderedPromptCommitThenLocalSubmissionOutput(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.SetPromptMsg("HP>"))
	m = next.(*Model)
	next, _ = m.Update(ui.CommitPromptMsg("HP>"))
	m = next.(*Model)
	next, _ = m.Update(ui.EchoLineMsg("> /help"))
	m = next.(*Model)
	next, _ = m.Update(ui.PrintLineMsg("local help"))
	m = next.(*Model)

	wantScrollback(t, m, "HP>", "> /help", "local help")
	if got := m.output.promptText; got != "" {
		t.Fatalf("prompt overlay = %q after commit, want empty", got)
	}
}

func TestPromptClearClearsOverlay(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.SetPromptMsg("User"))
	m = next.(*Model)
	next, _ = m.Update(ui.SetPromptMsg("Username:"))
	m = next.(*Model)

	wantScrollback(t, m)
	if got := m.output.promptText; got != "Username:" {
		t.Fatalf("prompt overlay = %q, want %q", got, "Username:")
	}

	next, _ = m.Update(ui.SetPromptMsg(""))
	m = next.(*Model)

	wantScrollback(t, m)
	if m.output.promptText != "" {
		t.Fatalf("prompt overlay = %q after clear, want empty", m.output.promptText)
	}
}

// TestMultiLinePrintSplitsIntoRows pins issue #49: a Print carrying
// embedded newlines must become one scrollback row per line, with
// lone CR and CRLF treated as line breaks.
func TestMultiLinePrintSplitsIntoRows(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.PrintLineMsg("row 1\rrow 2\r\nrow 3"))
	m = next.(*Model)

	wantScrollback(t, m, "row 1", "row 2", "row 3")
}

// TestMultiLinePrintSplitsInsideBatchWindow verifies the batched path
// splits too: a multi-line Print arriving inside an open window lands
// as individual rows when the tick flushes.
func TestMultiLinePrintSplitsInsideBatchWindow(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.PrintLineMsg("first")) // immediate, opens window
	m = next.(*Model)
	next, _ = m.Update(ui.PrintLineMsg("row 1\nrow 2")) // batched
	m = next.(*Model)
	next, _ = m.Update(tickMsg{generation: m.output.batchGeneration})
	m = next.(*Model)

	wantScrollback(t, m, "first", "row 1", "row 2")
}

// TestOverlongPrintWordWrapsToWidth pins issue #49: a line wider than
// the terminal word-wraps into multiple rows at the last space rather
// than being clipped. The model is 80 columns wide (newBareModel).
func TestOverlongPrintWordWrapsToWidth(t *testing.T) {
	m := newBareModel(t)

	head := strings.Repeat("x", 60)
	tail := strings.Repeat("y", 30)
	next, _ := m.Update(ui.PrintLineMsg(head + " " + tail))
	m = next.(*Model)

	wantScrollback(t, m, head, tail)
}

// TestOverlongUnbreakableWordHardWraps verifies a single word wider
// than the terminal is broken at the width rather than clipped.
func TestOverlongUnbreakableWordHardWraps(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.EchoLineMsg(strings.Repeat("z", 100)))
	m = next.(*Model)

	wantScrollback(t, m, strings.Repeat("z", 80), strings.Repeat("z", 20))
}

// TestMultiLineEchoSplitsIntoRows verifies the echo path splits like
// Print, and that tab columns restart on each row rather than carrying
// across the whole message.
func TestMultiLineEchoSplitsIntoRows(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.EchoLineMsg("> dump\na\tb"))
	m = next.(*Model)

	wantScrollback(t, m, "> dump", "a       b")
}

func TestEchoExpandsPreservedTabsBeforeScrollback(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(ui.EchoLineMsg("> a\tb"))
	m = next.(*Model)

	got := m.output.buffer.At(0)
	if strings.ContainsRune(got, '\t') {
		t.Fatalf("raw tab reached scrollback: %q", got)
	}
	if !strings.Contains(got, "b") || len(got) <= len("> a b") {
		t.Fatalf("tab was not expanded for display: %q", got)
	}
}

// Regression #16: raw tabs must never reach the renderer. Bubbletea
// repaints only changed rows; a row starting with \t makes the terminal
// skip cells without erasing them, resurrecting the previous frame
// (ghost columns). True paint verification is the manual tmux route -
// this pins the model-layer guarantee that scrollback rows are tab-free.
func TestPrintedTabsAreExpanded(t *testing.T) {
	m := newTestModel(t)
	next, _ := m.Update(ui.PrintLineMsg("\tDead-file cleanup"))
	m = next.(*Model)
	found := false
	for i := 0; i < m.output.buffer.Count(); i++ {
		row := m.output.buffer.At(i)
		if row == "        Dead-file cleanup" {
			found = true
		}
		if strings.Contains(row, "\t") {
			t.Errorf("raw tab reached scrollback row %d: %q", i, row)
		}
	}
	if !found {
		t.Errorf("expanded row not found in scrollback")
	}
	next, _ = m.Update(ui.SetPromptMsg("HP\t> "))
	m = next.(*Model)
	if got := m.output.promptText; got != "HP      > " {
		t.Errorf("prompt = %q, want tab expanded", got)
	}
}
