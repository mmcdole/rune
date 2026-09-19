package widget

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/mmcdole/rune/ui/tui/style"
	"github.com/mmcdole/rune/ui/tui/util"
)

func newTestOutput(width, height int, lines ...string) (*Output, *Scrollback) {
	v := NewOutput(1000, style.DefaultStyles())
	buf := v.Scrollback()
	v.SetSize(width, height)
	for _, l := range lines {
		buf.Append(l)
		v.onNewRows(1)
	}
	return v, buf
}

func viewRows(v *Output) []string {
	return strings.Split(v.View(), "\n")
}

func TestOutputPromptFitsSingleRow(t *testing.T) {
	v, _ := newTestOutput(20, 1, "history")
	v.SetPrompt("HP>")
	if got := v.View(); got != "HP>" {
		t.Fatalf("one-row output = %q, want prompt without an extra row", got)
	}
}

func TestOutputResizeClampsScrolledOffsetToVisibleHistory(t *testing.T) {
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i+1)
	}
	v, buffer := newTestOutput(40, 2, lines...)
	v.ScrollToTop()
	if v.offset != 8 {
		t.Fatalf("top offset = %d, want 8", v.offset)
	}

	v.SetSize(40, 5)
	if v.offset != 5 {
		t.Fatalf("grown output window offset = %d, want clamped maximum 5", v.offset)
	}
	rows := viewRows(v)
	for i := 0; i < 5; i++ {
		if rows[i] != lines[i] {
			t.Fatalf("grown output window row %d = %q, want %q (all rows %q)", i, rows[i], lines[i], rows)
		}
	}

	buffer.Append("line 11")
	v.onNewRows(1)
	if v.NewLineCount() != 1 {
		t.Fatalf("new line count = %d, want 1 while scrolled", v.NewLineCount())
	}
	v.SetSize(40, 20)
	if v.Mode() != ModeLive || v.offset != 0 || v.NewLineCount() != 0 {
		t.Fatalf("fully expanded output window = mode %v offset %d new %d, want live zero state",
			v.Mode(), v.offset, v.NewLineCount())
	}
}

func TestOutputScrollDistancesCannotInvertOrOverflow(t *testing.T) {
	v, _ := newTestOutput(40, 2, "one", "two", "three", "four", "five")
	v.ScrollUp(1)
	before := v.offset
	v.ScrollUp(-1)
	v.ScrollDown(-1)
	if v.offset != before {
		t.Fatalf("negative distance changed offset from %d to %d", before, v.offset)
	}
	v.ScrollUp(int(^uint(0) >> 1))
	if v.offset != v.maxOffset() {
		t.Fatalf("huge distance offset = %d, want maximum %d", v.offset, v.maxOffset())
	}
}

func TestOutputLiveShowsNewestLines(t *testing.T) {
	var lines []string
	for i := 1; i <= 10; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	v, _ := newTestOutput(40, 3, lines...)

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

func TestOutputPadsTopWhenContentShort(t *testing.T) {
	v, _ := newTestOutput(40, 4, "only line")

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

func TestOutputPromptTakesBottomRowInLiveMode(t *testing.T) {
	v, _ := newTestOutput(40, 3, "one", "two", "three", "four")
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

func TestOutputScrollAnchorsWhileNewLinesArrive(t *testing.T) {
	v, buf := newTestOutput(40, 2, "one", "two", "three", "four", "five", "six")

	v.ScrollUp(3)
	if v.Mode() != ModeScrolled {
		t.Fatal("expected scrolled mode")
	}
	before := viewRows(v)

	buf.Append("seven")
	v.onNewRows(1)
	buf.Append("eight")
	v.onNewRows(1)

	after := viewRows(v)
	for i := range before {
		if before[i] != after[i] {
			t.Errorf("scrolled view moved: %q -> %q", before, after)
		}
	}
	if v.NewLineCount() != 2 {
		t.Errorf("NewLineCount = %d, want 2", v.NewLineCount())
	}

	v.ScrollToBottom()
	rows := viewRows(v)
	if rows[len(rows)-1] != "eight" {
		t.Errorf("ScrollToBottom should land on the newest line, got %q", rows)
	}
	if v.Mode() != ModeLive || v.NewLineCount() != 0 {
		t.Error("ScrollToBottom must restore live mode and clear the counter")
	}
}

func TestOutputPagingClampsAndRestoresLive(t *testing.T) {
	var lines []string
	for i := 1; i <= 10; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	v, _ := newTestOutput(40, 4, lines...)

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

func TestOutputScrollToTop(t *testing.T) {
	v, _ := newTestOutput(40, 2, "one", "two", "three", "four")
	v.ScrollToTop()
	rows := viewRows(v)
	if rows[0] != "one" || rows[1] != "two" {
		t.Errorf("ScrollToTop view = %q, want the two oldest lines", rows)
	}
	if v.Mode() != ModeScrolled {
		t.Error("ScrollToTop with history should enter scrolled mode")
	}
}

func TestOutputClipsOverlongRows(t *testing.T) {
	long := strings.Repeat("x", 100)
	styledLong := "\x1b[1;31m" + strings.Repeat("y", 100) + "\x1b[m"
	v, _ := newTestOutput(20, 2, long, styledLong)
	v.SetPrompt("")

	for i, row := range viewRows(v) {
		if got := ansi.StringWidth(row); got > 20 {
			t.Errorf("row %d is %d cols wide, must be <= 20", i, got)
		}
	}
}

func TestOutputEmptyBufferRendersBlankRows(t *testing.T) {
	v, _ := newTestOutput(40, 3)
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

// A output window left scrolled while the full ring buffer evicts its anchor
// rows must pin to the oldest surviving window and keep its exact
// geometry. The offset used to grow past the buffer unclamped, and View
// then emitted more rows than its assigned height, corrupting the frame
// (issue #60).
func TestOutputScrolledSurvivesRingBufferEviction(t *testing.T) {
	v := NewOutput(8, style.DefaultStyles())
	buf := v.Scrollback()
	v.SetSize(40, 3)
	for i := 1; i <= 8; i++ {
		buf.Append(fmt.Sprintf("line %d", i))
		v.onNewRows(1)
	}

	v.ScrollToTop() // anchored on lines 1-3
	if v.Mode() != ModeScrolled {
		t.Fatal("expected scrolled mode after ScrollToTop")
	}

	// Push far past the anchor: lines 1-32 are evicted, 33-40 survive.
	for i := 9; i <= 40; i++ {
		buf.Append(fmt.Sprintf("line %d", i))
		v.onNewRows(1)
	}

	rows := viewRows(v)
	if len(rows) != 3 {
		t.Fatalf("View emitted %d rows, want exactly the assigned 3: %q", len(rows), rows)
	}
	want := []string{"line 33", "line 34", "line 35"}
	for i, w := range want {
		if rows[i] != w {
			t.Errorf("row %d = %q, want %q (view should pin to the oldest surviving rows)", i, rows[i], w)
		}
	}
	if v.Mode() != ModeScrolled {
		t.Error("eviction must not silently return the output window to live mode")
	}

	v.ScrollToBottom()
	rows = viewRows(v)
	if rows[len(rows)-1] != "line 40" {
		t.Errorf("ScrollToBottom after eviction should land on the newest line, got %q", rows)
	}
}

// View must never emit more rows than its height, whatever state the
// offset is in - the frame-geometry backstop behind the onNewRows clamp.
func TestOutputViewNeverExceedsHeight(t *testing.T) {
	v, _ := newTestOutput(40, 3, "a", "b", "c", "d", "e")
	v.offset = 999

	rows := viewRows(v)
	if len(rows) != 3 {
		t.Fatalf("View emitted %d rows with a corrupt offset, want 3: %q", len(rows), rows)
	}
}

func TestScrollbackBufferWrapsAtCapacity(t *testing.T) {
	buf := NewScrollback(3)
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

func TestScrollbackBufferSeqSurvivesEviction(t *testing.T) {
	buf := NewScrollback(5)
	for i := 1; i <= 8; i++ {
		buf.Append(fmt.Sprintf("line %d", i))
	}
	// Lines 4-8 survive; seq numbers are 3-7 (0-based from first append).
	if got := buf.Seq(0); got != 3 {
		t.Errorf("Seq(0) = %d, want 3", got)
	}
	for i := 0; i < buf.Count(); i++ {
		idx, ok := buf.IndexOf(buf.Seq(i))
		if !ok || idx != i {
			t.Errorf("IndexOf(Seq(%d)) = %d,%v, want %d,true", i, idx, ok, i)
		}
	}
	if _, ok := buf.IndexOf(2); ok {
		t.Error("IndexOf of an evicted seq must report ok=false")
	}
	if _, ok := buf.IndexOf(8); ok {
		t.Error("IndexOf of a not-yet-appended seq must report ok=false")
	}
}

func TestOutputCenterOn(t *testing.T) {
	var lines []string
	for i := 1; i <= 20; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	v, buf := newTestOutput(40, 5, lines...)

	// Center on line 10 (seq 9): it should sit on the middle row.
	v.CenterOn(buf.Seq(9))
	rows := viewRows(v)
	if rows[2] != "line 10" {
		t.Errorf("center row = %q, want %q (rows %q)", rows[2], "line 10", rows)
	}
	if v.Mode() != ModeScrolled {
		t.Error("centering into history should enter scrolled mode")
	}

	// Near the top: clamps so the frame stays full.
	v.CenterOn(buf.Seq(0))
	rows = viewRows(v)
	if rows[0] != "line 1" {
		t.Errorf("top clamp: rows = %q, want line 1 first", rows)
	}

	// Near the live edge: clamps to offset 0 and returns to live.
	v.CenterOn(buf.Seq(19))
	if v.Mode() != ModeLive {
		t.Error("centering on the newest row should land live")
	}

	// Buffer smaller than the output window: always live, never negative.
	small, sbuf := newTestOutput(40, 10, "a", "b")
	small.CenterOn(sbuf.Seq(0))
	if small.Mode() != ModeLive {
		t.Error("centering within a short buffer should stay live")
	}
}

func TestOutputHighlightRendersOnRightRowOnly(t *testing.T) {
	v, buf := newTestOutput(40, 3, "one thief", "two", "three", "four")
	v.ScrollUp(1) // showing "one thief", "two", "three"
	v.SetHighlight(buf.Seq(0), []util.ColRange{{Start: 4, End: 9}})

	rows := viewRows(v)
	if !strings.Contains(rows[0], "\x1b[") || !strings.Contains(rows[0], "thief") {
		t.Errorf("highlighted row should carry escape codes: %q", rows[0])
	}
	for i := 1; i < len(rows); i++ {
		if strings.Contains(rows[i], "\x1b[") {
			t.Errorf("row %d should be unstyled, got %q", i, rows[i])
		}
	}

	// The highlight follows its row: while scrolled, appends re-anchor
	// the view and the highlight must stay on the same text.
	buf.Append("five")
	v.onNewRows(1)
	rows = viewRows(v)
	if !strings.Contains(rows[0], "\x1b[") || !strings.Contains(rows[0], "thief") {
		t.Errorf("after append the highlight must stay on its row; rows = %q", rows)
	}

	v.ClearHighlight()
	for i, row := range viewRows(v) {
		if strings.Contains(row, "\x1b[") {
			t.Errorf("row %d still styled after ClearHighlight: %q", i, row)
		}
	}
}

func TestOutputSetHighlightInvalidatesCache(t *testing.T) {
	v, buf := newTestOutput(40, 2, "alpha", "beta")
	before := v.View()
	v.SetHighlight(buf.Seq(1), []util.ColRange{{Start: 0, End: 4}})
	if v.View() == before {
		t.Error("SetHighlight must invalidate the cached view")
	}
}

// Esc-restore must return to the same text, not the same distance from
// live: appends during search re-anchor a scrolled output window (onNewRows),
// and a raw saved offset would land newer by the rows that arrived.
func TestOutputRestoreScrollSameTextAfterAppends(t *testing.T) {
	var lines []string
	for i := 1; i <= 10; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	v, buf := newTestOutput(40, 2, lines...)

	v.ScrollUp(5) // showing lines 4,5
	before := viewRows(v)
	saved := v.SaveScroll()

	v.CenterOn(buf.Seq(0)) // search preview moves the output window
	for i := 11; i <= 15; i++ {
		buf.Append(fmt.Sprintf("line %d", i))
		v.onNewRows(1)
	}

	v.RestoreScroll(saved)
	after := viewRows(v)
	for i := range before {
		if before[i] != after[i] {
			t.Errorf("restore changed the visible text: %q -> %q", before, after)
		}
	}
	if v.Mode() != ModeScrolled {
		t.Error("restore of a scrolled snapshot should stay scrolled")
	}
	if v.NewLineCount() != 5 {
		t.Errorf("NewLineCount = %d, want 5 (rows appended during search)", v.NewLineCount())
	}
}

func TestOutputRestoreScrollFromLiveReturnsToLive(t *testing.T) {
	v, buf := newTestOutput(40, 2, "one", "two", "three")
	saved := v.SaveScroll()

	v.CenterOn(buf.Seq(0))
	buf.Append("four")
	v.onNewRows(1)

	v.RestoreScroll(saved)
	if v.Mode() != ModeLive || v.NewLineCount() != 0 {
		t.Error("live snapshot must restore to live/tailing")
	}
	rows := viewRows(v)
	if rows[len(rows)-1] != "four" {
		t.Errorf("restored live view should show the newest line, got %q", rows)
	}
}

func TestOutputRestoreScrollClampsAfterEviction(t *testing.T) {
	v := NewOutput(6, style.DefaultStyles())
	buf := v.Scrollback()
	v.SetSize(40, 2)
	for i := 1; i <= 6; i++ {
		buf.Append(fmt.Sprintf("line %d", i))
		v.onNewRows(1)
	}

	v.ScrollToTop() // anchored on lines 1,2
	saved := v.SaveScroll()

	// Evict the anchor entirely.
	for i := 7; i <= 20; i++ {
		buf.Append(fmt.Sprintf("line %d", i))
		v.onNewRows(1)
	}

	v.RestoreScroll(saved)
	rows := viewRows(v)
	if len(rows) != 2 {
		t.Fatalf("View emitted %d rows, want 2: %q", len(rows), rows)
	}
	if rows[0] != "line 15" {
		t.Errorf("evicted anchor should pin to the oldest surviving view, got %q", rows)
	}
	if v.Mode() != ModeScrolled {
		t.Error("clamped restore should remain scrolled")
	}
}

func TestScrollbackAndOutputClearResetHistoryButKeepLivePrompt(t *testing.T) {
	v, buf := newTestOutput(20, 3, "one", "two", "three", "four")
	lastSeq := buf.Seq(buf.Count() - 1)
	v.ScrollUp(1)
	v.SetHighlight(lastSeq, []util.ColRange{{Start: 0, End: 3}})
	v.SetPrompt("HP> ")

	buf.Clear()
	v.Clear()
	if buf.Count() != 0 {
		t.Fatalf("buffer count after clear = %d, want 0", buf.Count())
	}
	if v.Mode() != ModeLive || v.NewLineCount() != 0 {
		t.Fatalf("output window after clear = (%v, %d), want live with no unseen rows", v.Mode(), v.NewLineCount())
	}
	if got := v.View(); !strings.HasSuffix(got, "HP> ") {
		t.Fatalf("clear removed prompt: %q", got)
	}

	buf.Append("new")
	v.onNewRows(1)
	if seq := buf.Seq(0); seq <= lastSeq {
		t.Fatalf("sequence after clear = %d, want greater than old sequence %d", seq, lastSeq)
	}
	if _, ok := buf.IndexOf(lastSeq); ok {
		t.Fatalf("old sequence %d resolved after clear", lastSeq)
	}
}
