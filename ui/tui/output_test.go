package tui

import (
	"strings"
	"testing"
)

func TestOrderedPromptCommitThenLocalSubmissionOutput(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(setPromptMsg("HP>"))
	m = next.(*Model)
	next, _ = m.Update(commitPromptMsg("HP>"))
	m = next.(*Model)
	next, _ = m.Update(echoLineMsg("> /help"))
	m = next.(*Model)
	next, _ = m.Update(printLineMsg("local help"))
	m = next.(*Model)

	wantScrollback(t, m, "HP>", "> /help", "local help")
	if got := m.output.Prompt(); got != "" {
		t.Fatalf("prompt overlay = %q after commit, want empty", got)
	}
}

func TestPromptClearClearsOverlay(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(setPromptMsg("User"))
	m = next.(*Model)
	next, _ = m.Update(setPromptMsg("Username:"))
	m = next.(*Model)

	wantScrollback(t, m)
	if got := m.output.Prompt(); got != "Username:" {
		t.Fatalf("prompt overlay = %q, want %q", got, "Username:")
	}

	next, _ = m.Update(setPromptMsg(""))
	m = next.(*Model)

	wantScrollback(t, m)
	if m.output.Prompt() != "" {
		t.Fatalf("prompt overlay = %q after clear, want empty", m.output.Prompt())
	}
}

// TestMultiLinePrintSplitsIntoRows pins issue #49: a Print carrying
// embedded newlines must become one scrollback row per line, with
// lone CR and CRLF treated as line breaks.
func TestMultiLinePrintSplitsIntoRows(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(printLineMsg("row 1\rrow 2\r\nrow 3"))
	m = next.(*Model)

	wantScrollback(t, m, "row 1", "row 2", "row 3")
}

// TestOverlongPrintWordWrapsToWidth pins issue #49: a line wider than
// the terminal word-wraps into multiple rows at the last space rather
// than being clipped. The model is 80 columns wide (newBareModel).
func TestOverlongPrintWordWrapsToWidth(t *testing.T) {
	m := newBareModel(t)

	head := strings.Repeat("x", 60)
	tail := strings.Repeat("y", 30)
	next, _ := m.Update(printLineMsg(head + " " + tail))
	m = next.(*Model)

	wantScrollback(t, m, head, tail)
}

// TestOverlongUnbreakableWordHardWraps verifies a single word wider
// than the terminal is broken at the width rather than clipped.
func TestOverlongUnbreakableWordHardWraps(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(echoLineMsg(strings.Repeat("z", 100)))
	m = next.(*Model)

	wantScrollback(t, m, strings.Repeat("z", 80), strings.Repeat("z", 20))
}

// TestMultiLineEchoSplitsIntoRows verifies the echo path splits like
// Print, and that tab columns restart on each row rather than carrying
// across the whole message.
func TestMultiLineEchoSplitsIntoRows(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(echoLineMsg("> dump\na\tb"))
	m = next.(*Model)

	wantScrollback(t, m, "> dump", "a       b")
}

func TestEchoExpandsPreservedTabsBeforeScrollback(t *testing.T) {
	m := newBareModel(t)

	next, _ := m.Update(echoLineMsg("> a\tb"))
	m = next.(*Model)

	got := m.output.Scrollback().At(0)
	if strings.ContainsRune(got, '\t') {
		t.Fatalf("raw tab reached scrollback: %q", got)
	}
	if !strings.Contains(got, "b") || len(got) <= len("> a b") {
		t.Fatalf("tab was not expanded for display: %q", got)
	}
}

// Regression #16: raw tabs must never reach the renderer. Bubbletea
// rerenders only changed rows; a row starting with \t makes the terminal
// skip cells without erasing them, resurrecting the previous frame
// (ghost columns). True render verification is the manual tmux route -
// this pins the model-layer guarantee that scrollback rows are tab-free.
func TestPrintedTabsAreExpanded(t *testing.T) {
	m := newTestModel(t)
	next, _ := m.Update(printLineMsg("\tDead-file cleanup"))
	m = next.(*Model)
	found := false
	for i := 0; i < m.output.Scrollback().Count(); i++ {
		row := m.output.Scrollback().At(i)
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
	next, _ = m.Update(setPromptMsg("HP\t> "))
	m = next.(*Model)
	if got := m.output.Prompt(); got != "HP      > " {
		t.Errorf("prompt = %q, want tab expanded", got)
	}
}
