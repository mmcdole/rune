package widget

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mmcdole/rune/ui/tui/util"
)

func newTestViewport(width, height int, lines ...string) (*Viewport, *ScrollbackBuffer) {
	buf := NewScrollbackBuffer(1000)
	v := NewViewport(buf)
	v.SetSize(width, height)
	for _, l := range lines {
		buf.Append(l)
		v.OnNewRows(1)
	}
	return v, buf
}

func viewRows(v *Viewport) []string {
	return strings.Split(v.View(), "\n")
}

func TestViewportLiveShowsNewestLines(t *testing.T) {
	var lines []string
	for i := 1; i <= 10; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	v, _ := newTestViewport(40, 3, lines...)

	rows := viewRows(v)
	want := []string{"line 8", "line 9", "line 10"}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d: %q", len(rows), rows)
	}
	for i, w := range want {
		if rows[i] != w {
			t.Errorf("row %d = %q, want %q", i, rows[i], w)
		}
	}
}

func TestViewportPadsTopWhenContentShort(t *testing.T) {
	v, _ := newTestViewport(40, 4, "only line")

	rows := viewRows(v)
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows, got %d: %q", len(rows), rows)
	}
	for i := 0; i < 3; i++ {
		if rows[i] != "" {
			t.Errorf("row %d should be blank padding, got %q", i, rows[i])
		}
	}
	if rows[3] != "only line" {
		t.Errorf("bottom row = %q, want %q", rows[3], "only line")
	}
}

func TestViewportPromptTakesBottomRowInLiveMode(t *testing.T) {
	v, _ := newTestViewport(40, 3, "one", "two", "three", "four")
	v.SetPrompt("HP:100> ")

	rows := viewRows(v)
	if rows[len(rows)-1] != "HP:100> " {
		t.Errorf("bottom row = %q, want the prompt", rows[len(rows)-1])
	}
	// Prompt displaces one content row: only the two newest lines fit.
	if rows[0] != "three" || rows[1] != "four" {
		t.Errorf("content rows = %q, want [three four]", rows[:2])
	}

	// Scrolled mode hides the prompt overlay.
	v.ScrollUp(2)
	for _, row := range viewRows(v) {
		if strings.Contains(row, "HP:100>") {
			t.Errorf("prompt should not render while scrolled, got %q", row)
		}
	}
}

func TestViewportScrollAnchorsWhileNewLinesArrive(t *testing.T) {
	v, buf := newTestViewport(40, 2, "one", "two", "three", "four", "five", "six")

	v.ScrollUp(3)
	if v.Mode() != ModeScrolled {
		t.Fatal("expected scrolled mode")
	}
	before := viewRows(v)

	buf.Append("seven")
	v.OnNewRows(1)
	buf.Append("eight")
	v.OnNewRows(1)

	after := viewRows(v)
	for i := range before {
		if before[i] != after[i] {
			t.Errorf("scrolled view moved: %q -> %q", before, after)
		}
	}
	if v.NewLineCount() != 2 {
		t.Errorf("NewLineCount = %d, want 2", v.NewLineCount())
	}

	v.GotoBottom()
	rows := viewRows(v)
	if rows[len(rows)-1] != "eight" {
		t.Errorf("GotoBottom should land on the newest line, got %q", rows)
	}
	if v.Mode() != ModeLive || v.NewLineCount() != 0 {
		t.Error("GotoBottom must restore live mode and clear the counter")
	}
}

func TestViewportPagingClampsAndRestoresLive(t *testing.T) {
	var lines []string
	for i := 1; i <= 10; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	v, _ := newTestViewport(40, 4, lines...)

	// Page up far past the top: clamps to the oldest full window.
	for i := 0; i < 10; i++ {
		v.PageUp()
	}
	rows := viewRows(v)
	if rows[0] != "line 1" {
		t.Errorf("over-paging should clamp to the top, got %q", rows)
	}

	// One page down at a time eventually returns to live mode.
	for i := 0; i < 10; i++ {
		v.PageDown()
	}
	if v.Mode() != ModeLive {
		t.Error("paging to the bottom must restore live mode")
	}
	rows = viewRows(v)
	if rows[len(rows)-1] != "line 10" {
		t.Errorf("live view should end at the newest line, got %q", rows)
	}
}

func TestViewportGotoTop(t *testing.T) {
	v, _ := newTestViewport(40, 2, "one", "two", "three", "four")
	v.GotoTop()
	rows := viewRows(v)
	if rows[0] != "one" || rows[1] != "two" {
		t.Errorf("GotoTop view = %q, want the two oldest lines", rows)
	}
	if v.Mode() != ModeScrolled {
		t.Error("GotoTop with history should enter scrolled mode")
	}
}

func TestViewportClipsOverlongRows(t *testing.T) {
	long := strings.Repeat("x", 100)
	styledLong := "\x1b[1;31m" + strings.Repeat("y", 100) + "\x1b[m"
	v, _ := newTestViewport(20, 2, long, styledLong)
	v.SetPrompt("")

	for i, row := range viewRows(v) {
		if got := util.VisibleLen(row); got > 20 {
			t.Errorf("row %d is %d cols wide, must be <= 20", i, got)
		}
	}
}

func TestViewportEmptyBufferRendersBlankRows(t *testing.T) {
	v, _ := newTestViewport(40, 3)
	rows := viewRows(v)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d: %q", len(rows), rows)
	}
	for i, r := range rows {
		if r != "" {
			t.Errorf("row %d should be blank, got %q", i, r)
		}
	}
}

func TestScrollbackBufferWrapsAtCapacity(t *testing.T) {
	buf := NewScrollbackBuffer(3)
	for i := 1; i <= 5; i++ {
		buf.Append(fmt.Sprintf("line %d", i))
	}
	if buf.Count() != 3 {
		t.Fatalf("Count = %d, want 3", buf.Count())
	}
	want := []string{"line 3", "line 4", "line 5"}
	for i, w := range want {
		if buf.At(i) != w {
			t.Errorf("At(%d) = %q, want %q", i, buf.At(i), w)
		}
	}
	if buf.At(-1) != "" || buf.At(3) != "" {
		t.Error("out-of-range At should return empty string")
	}
}
