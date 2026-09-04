package widget

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mmcdole/rune/ui/tui/util"
)

func newTestPane(t *testing.T) *Pane {
	t.Helper()
	return NewPane("test")
}

func contentRows(t *testing.T, p *Pane, width, height int) []string {
	t.Helper()
	rows := p.ContentRows(width, height)
	if len(rows) != height {
		t.Fatalf("content height = %d rows, want %d", len(rows), height)
	}
	return rows
}

// TestPaneMultilineWriteWhileScrolled verifies a multi-line write
// counts each segment: the scrolled view stays anchored and the
// header indicator reflects every new line.
func TestPaneMultilineWriteWhileScrolled(t *testing.T) {
	p := newTestPane(t)
	for i := 1; i <= 6; i++ {
		p.Write(fmt.Sprintf("line %d", i))
	}
	p.ScrollUp(3)
	before := contentRows(t, p, 40, 2)

	p.Write("line 7\nline 8")

	after := contentRows(t, p, 40, 2)
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
	p := newTestPane(t)
	p.Write("a\rb\r\nc\nd")

	if len(p.lines) != 4 {
		t.Fatalf("expected 4 logical lines, got %d: %q", len(p.lines), p.lines)
	}
	for i, line := range p.lines {
		if strings.ContainsAny(line, "\r\n") {
			t.Fatalf("stored line %d contains a line break: %q", i, line)
		}
	}
	if rows := contentRows(t, p, 40, 5); len(rows) != 5 {
		t.Fatalf("view content height = %d rows, want the budgeted 5", len(rows))
	}
}

func TestPaneWrapsLongLines(t *testing.T) {
	p := newTestPane(t)
	p.Write("one two three four five six seven")

	rows := contentRows(t, p, 20, 4)
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
	p := newTestPane(t)
	for i := 1; i <= 10; i++ {
		p.Write(fmt.Sprintf("line %d", i))
	}

	rows := contentRows(t, p, 40, 3)
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
	p := newTestPane(t)
	p.Write("aaaa bbbb cccc dddd eeee")

	rows := contentRows(t, p, 10, 2)
	if strings.TrimSpace(rows[0]) == "" || strings.TrimSpace(rows[1]) == "" {
		t.Errorf("expected the newest wrapped rows, got %q", rows)
	}
	if !strings.Contains(rows[1], "eeee") {
		t.Errorf("last row should hold the end of the message, got %q", rows[1])
	}
}

func TestPaneScrollUpShowsHistoryAndIndicator(t *testing.T) {
	p := newTestPane(t)
	for i := 1; i <= 10; i++ {
		p.Write(fmt.Sprintf("line %d", i))
	}

	p.ScrollUp(5)
	rows := contentRows(t, p, 40, 2)
	if rows[0] != "line 4" || rows[1] != "line 5" {
		t.Errorf("scrolled view = %q, want lines 4-5", rows)
	}
	if title := p.Title(); title != "test · scroll" {
		t.Errorf("title = %q, want scroll indicator", title)
	}
}

func TestPaneScrollDistancesCannotInvertOrOverflow(t *testing.T) {
	p := newTestPane(t)
	for i := 1; i <= 5; i++ {
		p.Write(fmt.Sprintf("line %d", i))
	}
	p.ScrollUp(1)
	before := p.offset
	p.ScrollUp(-1)
	p.ScrollDown(-1)
	if p.offset != before {
		t.Fatalf("negative distance changed offset from %d to %d", before, p.offset)
	}
	p.ScrollUp(int(^uint(0) >> 1))
	if want := len(p.lines) - 1; p.offset != want {
		t.Fatalf("huge distance offset = %d, want maximum %d", p.offset, want)
	}
}

func TestPaneWritesWhileScrolledFreezeViewAndCount(t *testing.T) {
	p := newTestPane(t)
	for i := 1; i <= 6; i++ {
		p.Write(fmt.Sprintf("line %d", i))
	}
	p.ScrollUp(3)
	before := contentRows(t, p, 40, 2)

	p.Write("line 7")
	p.Write("line 8")

	after := contentRows(t, p, 40, 2)
	if before[0] != after[0] || before[1] != after[1] {
		t.Errorf("view should stay anchored while scrolled: %q -> %q", before, after)
	}
	if title := p.Title(); title != "test · scroll +2" {
		t.Errorf("title = %q, want new-line count", title)
	}

	p.ScrollToBottom()
	rows := contentRows(t, p, 40, 2)
	if rows[1] != "line 8" {
		t.Errorf("bottom should show the newest line, got %q", rows)
	}
	if title := p.Title(); title != "test" {
		t.Errorf("title should clear its scroll indicator at bottom, got %q", title)
	}
}

func TestPaneScrollClamps(t *testing.T) {
	p := newTestPane(t)
	for i := 1; i <= 5; i++ {
		p.Write(fmt.Sprintf("line %d", i))
	}

	p.ScrollUp(1000)
	rows := contentRows(t, p, 40, 3)
	if rows[0] != "line 1" {
		t.Errorf("over-scroll should clamp to the top, got %q", rows)
	}
	// Deep scroll keeps the window full by extending forward.
	if rows[1] != "line 2" || rows[2] != "line 3" {
		t.Errorf("scrolled-to-top window should stay full, got %q", rows)
	}

	p.ScrollDown(1000)
	rows = contentRows(t, p, 40, 3)
	if rows[2] != "line 5" {
		t.Errorf("scroll down past the end should return to live, got %q", rows)
	}
}

// Writes never touch scroll state: a scrolled pane stays anchored on the
// same history as new lines land, so a placement hidden and later re-shown
// renders exactly where it was.
func TestPaneWritesWhileScrolledKeepPosition(t *testing.T) {
	p := newTestPane(t)
	for i := 1; i <= 6; i++ {
		p.Write(fmt.Sprintf("line %d", i))
	}
	p.ScrollUp(3)
	p.Write("line 7")

	rows := contentRows(t, p, 40, 2)
	if rows[0] != "line 2" {
		t.Errorf("scrolled pane should keep its anchor across writes, got %q", rows)
	}
}

// A pane on the live tail stays in follow mode, so writes that land while
// its placement is hidden show up when it is re-shown.
func TestPaneOnTailFollowsWrites(t *testing.T) {
	p := newTestPane(t)
	for i := 1; i <= 7; i++ {
		p.Write(fmt.Sprintf("line %d", i))
	}

	rows := contentRows(t, p, 40, 2)
	if rows[1] != "line 7" {
		t.Errorf("live pane should follow the tail, got %q", rows)
	}
}

// If trimming removes the history a scrolled pane was anchored on, the
// anchor clamps to the oldest remaining line instead of jumping to
// the tail.
func TestPaneAnchorClampsWhenTrimmed(t *testing.T) {
	p := newTestPane(t)
	for i := 1; i <= 6; i++ {
		p.Write(fmt.Sprintf("line %d", i))
	}
	p.ScrollUp(5)
	for i := 7; i <= 1001; i++ {
		p.Write(fmt.Sprintf("line %d", i))
	}

	rows := contentRows(t, p, 40, 2)
	if rows[0] != "line 502" {
		t.Errorf("trimmed anchor should clamp to the oldest remaining line, got %q", rows)
	}
}

func TestPaneEmptyAndClear(t *testing.T) {
	p := newTestPane(t)
	rows := contentRows(t, p, 40, 3)
	for i, r := range rows {
		if r != "" {
			t.Errorf("empty pane row %d should be blank, got %q", i, r)
		}
	}

	p.Write("something")
	p.ScrollUp(1)
	p.Clear()
	rows = contentRows(t, p, 40, 3)
	if strings.TrimSpace(strings.Join(rows, "")) != "" {
		t.Errorf("cleared pane should be blank, got %q", rows)
	}
}

func TestPaneContentRowsUseRequestedGeometry(t *testing.T) {
	p := newTestPane(t)
	for i := 1; i <= 5; i++ {
		p.Write(fmt.Sprintf("line %d", i))
	}

	short := contentRows(t, p, 40, 2)
	if short[0] != "line 4" || short[1] != "line 5" {
		t.Fatalf("two-row view = %q, want lines 4-5", short)
	}

	tall := contentRows(t, p, 40, 4)
	if tall[0] != "line 2" || tall[3] != "line 5" {
		t.Fatalf("four-row view = %q, want lines 2-5", tall)
	}

	again := contentRows(t, p, 40, 2)
	if again[0] != "line 4" || again[1] != "line 5" {
		t.Fatalf("second two-row view = %q, want lines 4-5", again)
	}
	if rows := p.ContentRows(40, 0); rows != nil {
		t.Fatalf("zero-height content = %q, want nil", rows)
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
