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

func TestPaneToggleOffResetsScroll(t *testing.T) {
	p := newTestPane(t, 40, 2)
	for i := 1; i <= 6; i++ {
		p.Write(fmt.Sprintf("line %d", i))
	}
	p.ScrollUp(3)
	p.Toggle() // hide
	p.Toggle() // show again

	rows := contentRows(t, p)
	if rows[1] != "line 6" {
		t.Errorf("re-shown pane should be at the live tail, got %q", rows)
	}
}

func TestPaneSetVisibleHideResetsScroll(t *testing.T) {
	p := newTestPane(t, 40, 2)
	for i := 1; i <= 6; i++ {
		p.Write(fmt.Sprintf("line %d", i))
	}
	p.ScrollUp(3)
	p.SetVisible(false)
	p.SetVisible(true)

	rows := contentRows(t, p)
	if rows[1] != "line 6" {
		t.Errorf("re-shown pane should be at the live tail, got %q", rows)
	}
}

func TestPaneSetVisibleIsIdempotent(t *testing.T) {
	p := newTestPane(t, 40, 2)
	for i := 1; i <= 6; i++ {
		p.Write(fmt.Sprintf("line %d", i))
	}
	p.ScrollUp(3)
	p.SetVisible(true) // already visible: must not touch scroll

	rows := contentRows(t, p)
	if rows[0] != "line 2" {
		t.Errorf("show on a visible pane should keep its scroll position, got %q", rows)
	}

	p.SetVisible(false)
	p.SetVisible(false) // already hidden: still hidden, still live
	if p.Visible {
		t.Error("pane should remain hidden")
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
