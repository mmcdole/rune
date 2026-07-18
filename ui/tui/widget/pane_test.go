package widget

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mmcdole/rune/ui/tui/style"
	"github.com/mmcdole/rune/ui/tui/util"
)

// newTestPane returns a visible pane sized to width x (content+2),
// matching how the layout sizes docked panes.
func newTestPane(t *testing.T, width, contentHeight int) *Pane {
	t.Helper()
	p := NewPane("test", style.DefaultStyles())
	p.Visible = true
	p.SetSize(width, contentHeight+2)
	return p
}

// contentRows strips the header and bottom border from View.
func contentRows(t *testing.T, p *Pane) []string {
	t.Helper()
	rows := strings.Split(p.View(), "\n")
	if len(rows) < 3 {
		t.Fatalf("view too short: %d rows", len(rows))
	}
	return rows[1 : len(rows)-1]
}

// TestPaneMultilineWriteWhileScrolled verifies a multi-line write
// counts each segment: the scrolled view stays anchored and the
// header indicator reflects every new line.
func TestPaneMultilineWriteWhileScrolled(t *testing.T) {
	p := newTestPane(t, 40, 2)
	for i := 1; i <= 6; i++ {
		p.Write(fmt.Sprintf("line %d", i))
	}
	p.ScrollUp(3)
	before := contentRows(t, p)

	p.Write("line 7\nline 8")

	after := contentRows(t, p)
	if before[0] != after[0] || before[1] != after[1] {
		t.Fatalf("scrolled view moved: before %q, after %q", before, after)
	}
	if p.newLines != 2 {
		t.Fatalf("newLines = %d, want 2", p.newLines)
	}
}

// TestPaneMultilineWriteSplitsIntoLines pins issue #49 for panes: a
// write containing newlines stores one logical line per segment, and
// the rendered view keeps its budgeted height.
func TestPaneMultilineWriteSplitsIntoLines(t *testing.T) {
	p := newTestPane(t, 40, 5)
	p.Write("a\rb\r\nc\nd")

	if len(p.Lines) != 4 {
		t.Fatalf("expected 4 logical lines, got %d: %q", len(p.Lines), p.Lines)
	}
	for i, line := range p.Lines {
		if strings.ContainsAny(line, "\r\n") {
			t.Fatalf("stored line %d contains a line break: %q", i, line)
		}
	}
	if rows := contentRows(t, p); len(rows) != 5 {
		t.Fatalf("view content height = %d rows, want the budgeted 5", len(rows))
	}
}

func TestPaneWrapsLongLines(t *testing.T) {
	p := newTestPane(t, 20, 4)
	p.Write("one two three four five six seven")

	rows := contentRows(t, p)
	for i, r := range rows {
		if util.VisibleLen(r) > 20 {
			t.Errorf("row %d exceeds width: %q (%d cols)", i, r, util.VisibleLen(r))
		}
	}
	joined := strings.Join(rows, " ")
	joined = strings.Join(strings.Fields(joined), " ")
	if joined != "one two three four five six seven" {
		t.Errorf("wrapped content mangled: %q", joined)
	}
}

func TestPaneTailShowsNewestRows(t *testing.T) {
	p := newTestPane(t, 40, 3)
	for i := 1; i <= 10; i++ {
		p.Write(fmt.Sprintf("line %d", i))
	}

	rows := contentRows(t, p)
	want := []string{"line 8", "line 9", "line 10"}
	for i, w := range want {
		if rows[i] != w {
			t.Errorf("row %d = %q, want %q", i, rows[i], w)
		}
	}
}

func TestPaneWrappedTailCountsVisualRows(t *testing.T) {
	// One long line wraps to more rows than the pane height: the pane
	// must show the newest rows of it, not blank out.
	p := newTestPane(t, 10, 2)
	p.Write("aaaa bbbb cccc dddd eeee")

	rows := contentRows(t, p)
	if strings.TrimSpace(rows[0]) == "" || strings.TrimSpace(rows[1]) == "" {
		t.Errorf("expected the newest wrapped rows, got %q", rows)
	}
	if !strings.Contains(rows[1], "eeee") {
		t.Errorf("last row should hold the end of the message, got %q", rows[1])
	}
}

func TestPaneScrollUpShowsHistoryAndIndicator(t *testing.T) {
	p := newTestPane(t, 40, 2)
	for i := 1; i <= 10; i++ {
		p.Write(fmt.Sprintf("line %d", i))
	}

	p.ScrollUp(5)
	rows := contentRows(t, p)
	if rows[0] != "line 4" || rows[1] != "line 5" {
		t.Errorf("scrolled view = %q, want lines 4-5", rows)
	}
	if header := strings.Split(p.View(), "\n")[0]; !strings.Contains(header, "scroll") {
		t.Errorf("header should show scroll indicator, got %q", header)
	}
}

func TestPaneWritesWhileScrolledFreezeViewAndCount(t *testing.T) {
	p := newTestPane(t, 40, 2)
	for i := 1; i <= 6; i++ {
		p.Write(fmt.Sprintf("line %d", i))
	}
	p.ScrollUp(3)
	before := contentRows(t, p)

	p.Write("line 7")
	p.Write("line 8")

	after := contentRows(t, p)
	if before[0] != after[0] || before[1] != after[1] {
		t.Errorf("view should stay anchored while scrolled: %q -> %q", before, after)
	}
	if header := strings.Split(p.View(), "\n")[0]; !strings.Contains(header, "+2") {
		t.Errorf("header should count new lines, got %q", header)
	}

	p.ScrollToBottom()
	rows := contentRows(t, p)
	if rows[1] != "line 8" {
		t.Errorf("bottom should show the newest line, got %q", rows)
	}
	if header := strings.Split(p.View(), "\n")[0]; strings.Contains(header, "scroll") {
		t.Errorf("indicator should clear at bottom, got %q", header)
	}
}

func TestPaneScrollClamps(t *testing.T) {
	p := newTestPane(t, 40, 3)
	for i := 1; i <= 5; i++ {
		p.Write(fmt.Sprintf("line %d", i))
	}

	p.ScrollUp(1000)
	rows := contentRows(t, p)
	if rows[0] != "line 1" {
		t.Errorf("over-scroll should clamp to the top, got %q", rows)
	}
	// Deep scroll keeps the window full by extending forward.
	if rows[1] != "line 2" || rows[2] != "line 3" {
		t.Errorf("scrolled-to-top window should stay full, got %q", rows)
	}

	p.ScrollDown(1000)
	rows = contentRows(t, p)
	if rows[2] != "line 5" {
		t.Errorf("scroll down past the end should return to live, got %q", rows)
	}
}

// Visibility never touches scroll state. A pane hidden while scrolled
// reopens on the same history, even as writes land while it is hidden.
func TestPaneHiddenWhileScrolledKeepsPosition(t *testing.T) {
	p := newTestPane(t, 40, 2)
	for i := 1; i <= 6; i++ {
		p.Write(fmt.Sprintf("line %d", i))
	}
	p.ScrollUp(3)
	p.SetVisible(false)
	p.Write("line 7")
	p.SetVisible(true)

	rows := contentRows(t, p)
	if rows[0] != "line 2" {
		t.Errorf("re-shown pane should keep its scroll anchor, got %q", rows)
	}

	p.Toggle() // hide
	p.Toggle() // show again
	rows = contentRows(t, p)
	if rows[0] != "line 2" {
		t.Errorf("toggle must not touch scroll state either, got %q", rows)
	}
}

// A pane on the live tail when hidden stays in follow mode, so
// reopening shows the newest lines.
func TestPaneHiddenOnTailReopensLive(t *testing.T) {
	p := newTestPane(t, 40, 2)
	for i := 1; i <= 6; i++ {
		p.Write(fmt.Sprintf("line %d", i))
	}
	p.SetVisible(false)
	p.Write("line 7")
	p.SetVisible(true)

	rows := contentRows(t, p)
	if rows[1] != "line 7" {
		t.Errorf("pane hidden on the tail should reopen live, got %q", rows)
	}
}

// If trimming removes the history a hidden pane was anchored on, the
// anchor clamps to the oldest remaining line instead of jumping to
// the tail.
func TestPaneHiddenAnchorClampsWhenTrimmed(t *testing.T) {
	p := newTestPane(t, 40, 2)
	for i := 1; i <= 6; i++ {
		p.Write(fmt.Sprintf("line %d", i))
	}
	p.ScrollUp(5)
	p.SetVisible(false)
	for i := 7; i <= 1001; i++ {
		p.Write(fmt.Sprintf("line %d", i))
	}
	p.SetVisible(true)

	rows := contentRows(t, p)
	if rows[0] != "line 502" {
		t.Errorf("trimmed anchor should clamp to the oldest remaining line, got %q", rows)
	}
}

func TestPaneEmptyAndClear(t *testing.T) {
	p := newTestPane(t, 40, 3)
	rows := contentRows(t, p)
	for i, r := range rows {
		if r != "" {
			t.Errorf("empty pane row %d should be blank, got %q", i, r)
		}
	}

	p.Write("something")
	p.ScrollUp(1)
	p.Clear()
	rows = contentRows(t, p)
	if strings.TrimSpace(strings.Join(rows, "")) != "" {
		t.Errorf("cleared pane should be blank, got %q", rows)
	}
}

func TestClipRowTruncatesOverlongRows(t *testing.T) {
	long := strings.Repeat("x", 50)
	clipped := clipRow(long, 20)
	if util.VisibleLen(clipped) != 20 {
		t.Errorf("clipped to %d cols, want 20", util.VisibleLen(clipped))
	}
	if clipRow("short", 20) != "short" {
		t.Error("short rows must pass through untouched")
	}
	styled := "\x1b[1;32m" + strings.Repeat("y", 50) + "\x1b[m"
	if got := util.VisibleLen(clipRow(styled, 20)); got != 20 {
		t.Errorf("ANSI row clipped to %d cols, want 20", got)
	}
}
