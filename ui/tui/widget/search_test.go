package widget

import (
	"fmt"
	"strings"
	"testing"

	runetext "github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui/tui/style"
	"github.com/mmcdole/rune/ui/tui/util"
)

func newTestBuffer(lines ...string) *ScrollbackBuffer {
	buf := NewScrollbackBuffer(1000)
	for _, l := range lines {
		buf.Append(l)
	}
	return buf
}

func TestScanOlderReturnsChronologicalWindow(t *testing.T) {
	buf := newTestBuffer("a thief", "quiet", "the THIEF again")
	matches, more := scanOlder(buf, substringMatcher("thief"), buf.Seq(2), searchPageSize)
	if more {
		t.Error("small scan must not report more matches")
	}
	if len(matches) != 2 {
		t.Fatalf("got %d matches, want 2", len(matches))
	}
	if matches[0].Stripped != "a thief" || matches[1].Stripped != "the THIEF again" {
		t.Errorf("matches out of order: %q then %q", matches[0].Stripped, matches[1].Stripped)
	}
	if matches[0].Seq != buf.Seq(0) || matches[1].Seq != buf.Seq(2) {
		t.Error("match seq numbers must identify the source rows")
	}
}

func TestScanBackwardStripsANSIBeforeMatching(t *testing.T) {
	// The escape sequence splits the word in the raw row; matching must
	// run on stripped text, and columns must index the stripped row.
	buf := newTestBuffer("xx \x1b[31mthi\x1b[1mef\x1b[0m yy")
	matches, _ := scanOlder(buf, substringMatcher("thief"), buf.Seq(0), searchPageSize)
	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1", len(matches))
	}
	m := matches[0]
	if m.Stripped != "xx thief yy" {
		t.Errorf("Stripped = %q", m.Stripped)
	}
	want := []util.ColRange{{Start: 3, End: 8}}
	if len(m.Ranges) != 1 || m.Ranges[0] != want[0] {
		t.Errorf("Ranges = %v, want %v", m.Ranges, want)
	}
}

func TestScanBackwardWideRuneColumns(t *testing.T) {
	buf := newTestBuffer("日本 thief")
	matches, _ := scanOlder(buf, substringMatcher("thief"), buf.Seq(0), searchPageSize)
	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1", len(matches))
	}
	// Two double-width runes + space = column 5.
	want := util.ColRange{Start: 5, End: 10}
	if matches[0].Ranges[0] != want {
		t.Errorf("Ranges[0] = %v, want %v", matches[0].Ranges[0], want)
	}
}

func TestScanBackwardMultipleOccurrencesOneRow(t *testing.T) {
	buf := newTestBuffer("The thief robbed another thief.")
	matches, _ := scanOlder(buf, substringMatcher("thief"), buf.Seq(0), searchPageSize)
	if len(matches) != 1 {
		t.Fatalf("one row must yield one match, got %d", len(matches))
	}
	want := []util.ColRange{{Start: 4, End: 9}, {Start: 25, End: 30}}
	if len(matches[0].Ranges) != 2 || matches[0].Ranges[0] != want[0] || matches[0].Ranges[1] != want[1] {
		t.Errorf("Ranges = %v, want %v", matches[0].Ranges, want)
	}
}

func TestScanBackwardNonOverlapping(t *testing.T) {
	buf := newTestBuffer("aaa")
	matches, _ := scanOlder(buf, substringMatcher("aa"), buf.Seq(0), searchPageSize)
	if len(matches) != 1 || len(matches[0].Ranges) != 1 {
		t.Fatalf("overlapping occurrences must not double-count: %+v", matches)
	}
}

func TestScanBackwardNonASCIIQuery(t *testing.T) {
	buf := newTestBuffer("der STRASSE entlang", "die straße")
	matches, _ := scanOlder(buf, substringMatcher("straße"), buf.Seq(1), searchPageSize)
	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1 (simple fold does not equate ß/SS)", len(matches))
	}
	if matches[0].Stripped != "die straße" {
		t.Errorf("matched %q", matches[0].Stripped)
	}

	matches, _ = scanOlder(buf, substringMatcher("STRAßE"), buf.Seq(1), searchPageSize)
	if len(matches) != 1 {
		t.Errorf("case-folded non-ASCII query should match, got %d", len(matches))
	}
}

func TestScanOlderPagesNearestMatches(t *testing.T) {
	buf := NewScrollbackBuffer(1000)
	for i := 0; i < 300; i++ {
		buf.Append(fmt.Sprintf("thief %d", i))
	}
	matches, more := scanOlder(buf, substringMatcher("thief"), buf.Seq(299), searchPageSize)
	if len(matches) != searchPageSize {
		t.Fatalf("got %d matches, want page %d", len(matches), searchPageSize)
	}
	if !more {
		t.Error("page with older matches remaining must report more")
	}
	if matches[0].Stripped != "thief 50" || matches[len(matches)-1].Stripped != "thief 299" {
		t.Errorf("page = %q ... %q, want chronological nearest window",
			matches[0].Stripped, matches[len(matches)-1].Stripped)
	}

	// Exactly one page, all scanned: no older matches remain.
	buf2 := NewScrollbackBuffer(1000)
	for i := 0; i < searchPageSize; i++ {
		buf2.Append("thief")
	}
	_, more = scanOlder(buf2, substringMatcher("thief"), buf2.Seq(searchPageSize-1), searchPageSize)
	if more {
		t.Error("scan that reached the oldest row must not report more")
	}
}

func newTestSearch(lines ...string) *Search {
	return NewSearch(newTestBuffer(lines...), style.DefaultStyles())
}

// Preserve the search state while exercising its production Input renderer.
func searchInput(s *Search, width, height int) *Input {
	in := NewInput(s.styles, s)
	in.overlay = overlaySearch
	in.SetSize(width, height)
	return in
}

func TestSearchOpenPreservesQueryAndTypingReplaces(t *testing.T) {
	s := newTestSearch("a thief", "a guard")

	s.Open("thief", SearchScope{})
	if len(s.matches) != 1 {
		t.Fatalf("open with query should scan: %d matches", len(s.matches))
	}

	// Reopen without a query: last query kept.
	s.Open("", SearchScope{})
	if s.Query() != "thief" {
		t.Errorf("Query = %q, want preserved %q", s.Query(), "thief")
	}

	// First rune typed after open replaces the preserved query.
	s.TypeRunes([]rune("g"))
	if s.Query() != "g" {
		t.Errorf("Query = %q, want %q (pristine replace)", s.Query(), "g")
	}
	s.TypeRunes([]rune("uard"))
	if s.Query() != "guard" {
		t.Errorf("Query = %q, want %q", s.Query(), "guard")
	}
	if len(s.matches) != 1 || s.matches[0].Stripped != "a guard" {
		t.Errorf("typing must rescan: %+v", s.matches)
	}

	s.Backspace()
	if s.Query() != "guar" {
		t.Errorf("Backspace: Query = %q, want %q", s.Query(), "guar")
	}
}

func TestSearchSelectionUsesChronologicalOlderNewerWithoutWrapping(t *testing.T) {
	s := newTestSearch("thief 1", "thief 2", "thief 3")
	s.Open("thief", SearchScope{})

	if m, ok := s.Selected(); !ok || m.Stripped != "thief 3" {
		t.Fatalf("initial selection should be the newest match, got %+v", m)
	}
	s.SelectOlder()
	if m, _ := s.Selected(); m.Stripped != "thief 2" {
		t.Errorf("SelectOlder should step backward, got %q", m.Stripped)
	}
	s.SelectOlder()
	s.SelectOlder() // stops at oldest
	if m, _ := s.Selected(); m.Stripped != "thief 1" {
		t.Errorf("selection should stop at oldest, got %q", m.Stripped)
	}
	s.SelectNewer()
	if m, _ := s.Selected(); m.Stripped != "thief 2" {
		t.Errorf("SelectNewer should step forward, got %q", m.Stripped)
	}
}

func TestSearchStartsAtScrolledOriginRatherThanNewestPage(t *testing.T) {
	buf := NewScrollbackBuffer(1000)
	for i := 0; i < 10; i++ {
		text := "quiet"
		if i == 5 {
			text = "nearby thief"
		}
		buf.Append(text)
	}
	origin := buf.Seq(9)
	for i := 0; i < searchPageSize+20; i++ {
		buf.Append(fmt.Sprintf("newer thief %d", i))
	}

	s := NewSearch(buf, style.DefaultStyles())
	s.Open("thief", SearchScope{OriginSeq: origin, OriginSet: true})
	m, ok := s.Selected()
	if !ok || m.Stripped != "nearby thief" {
		t.Fatalf("selected %+v, want closest older match at the viewport origin", m)
	}
}

func TestSearchLoadsAnotherOlderPageWithoutWrapping(t *testing.T) {
	buf := NewScrollbackBuffer(1000)
	for i := 0; i < 300; i++ {
		buf.Append(fmt.Sprintf("thief %d", i))
	}
	s := NewSearch(buf, style.DefaultStyles())
	s.Open("thief", SearchScope{})

	// The initial page covers 50..299 with 299 selected. The 250th older
	// step crosses that loaded edge and selects 49 from the next page.
	for i := 0; i < searchPageSize; i++ {
		s.SelectOlder()
	}
	m, ok := s.Selected()
	if !ok || m.Stripped != "thief 49" {
		t.Fatalf("selected %+v, want first selection from the next older page", m)
	}
	if len(s.matches) != 300 || s.olderMore {
		t.Fatalf("loaded matches/more = %d/%v, want complete 300/false", len(s.matches), s.olderMore)
	}
}

func TestSearchFreezesCorpusUntilReopen(t *testing.T) {
	buf := newTestBuffer("thief before open")
	s := NewSearch(buf, style.DefaultStyles())
	s.Open("thief", SearchScope{})
	buf.Append("thief after open")

	// First typing replaces the preserved query and forces a rescan. The row
	// appended after Open must remain outside this search session.
	s.TypeRunes([]rune("thief"))
	if len(s.matches) != 1 || s.matches[0].Stripped != "thief before open" {
		t.Fatalf("frozen search included later output: %+v", s.matches)
	}
}

func TestSearchReopenResumesCommittedSequence(t *testing.T) {
	buf := newTestBuffer("thief 1", "quiet", "thief 2", "thief 3")
	s := NewSearch(buf, style.DefaultStyles())
	s.Open("thief", SearchScope{})
	s.SelectOlder()
	committed, _ := s.Selected()

	s.Open("", SearchScope{OriginSeq: buf.Seq(3), OriginSet: true, ResumeSeq: committed.Seq, ResumeSet: true})
	resumed, ok := s.Selected()
	if !ok || resumed.Seq != committed.Seq {
		t.Fatalf("resumed %+v, want committed sequence %d", resumed, committed.Seq)
	}
}

func TestSearchViewHeaderAndRows(t *testing.T) {
	s := newTestSearch("one thief", "two", "THIEF two")
	s.Open("thief", SearchScope{})

	view := runetext.StripANSI(searchInput(s, 60, 0).View())
	if !strings.Contains(view, "2/2") {
		t.Errorf("header should show newest as the final chronological match, view:\n%s", view)
	}
	if !strings.Contains(view, "Search:") || !strings.Contains(view, "thief█") {
		t.Errorf("header should show the query with cursor, view:\n%s", view)
	}
	if !strings.Contains(view, "THIEF two") {
		t.Errorf("view should list the newest match, view:\n%s", view)
	}

	if !strings.Contains(view, "↑ older") || !strings.Contains(view, "↓ newer") {
		t.Errorf("footer should explain temporal navigation, view:\n%s", view)
	}
	s.SelectOlder()
	if view := runetext.StripANSI(searchInput(s, 60, 0).View()); !strings.Contains(view, "1/2") {
		t.Errorf("count should follow selection, view:\n%s", view)
	}
}

func TestSearchViewNoMatches(t *testing.T) {
	s := newTestSearch("nothing here")
	s.Open("zzz", SearchScope{})

	view := runetext.StripANSI(searchInput(s, 60, 0).View())
	if !strings.Contains(view, "0/0") || !strings.Contains(view, "No matches") {
		t.Errorf("empty result view wrong:\n%s", view)
	}
	if _, ok := s.Selected(); ok {
		t.Error("Selected must report none for an empty result")
	}
}

func TestConstrainedSearchAlwaysRendersSelectedMatch(t *testing.T) {
	lines := make([]string, 8)
	for i := range lines {
		lines[i] = fmt.Sprintf("thief %d", i+1)
	}
	s := newTestSearch(lines...)
	s.Open("thief", SearchScope{})
	if selected, ok := s.Selected(); !ok || selected.Stripped != "thief 8" {
		t.Fatalf("test setup selected %+v, want newest match", selected)
	}

	for _, height := range []int{2, 3, 4} {
		view := runetext.StripANSI(searchInput(s, 40, height).View())
		if !strings.Contains(view, "thief 8") {
			t.Fatalf("height %d hid selected match:\n%s", height, view)
		}
	}
}

func TestSearchViewPartialCount(t *testing.T) {
	buf := NewScrollbackBuffer(1000)
	for i := 0; i < 300; i++ {
		buf.Append("thief")
	}
	s := NewSearch(buf, style.DefaultStyles())
	s.Open("thief", SearchScope{})

	if view := runetext.StripANSI(searchInput(s, 60, 0).View()); !strings.Contains(view, "250+ matches") {
		t.Errorf("partial scan should avoid a false ordinal, view:\n%s", view)
	}
}

func TestSearchInputMeasuredHeight(t *testing.T) {
	s := newTestSearch("thief 1", "thief 2")
	s.Open("thief", SearchScope{})
	// 2 results + help + editor + three separators
	if got := searchInput(s, 60, 0).MeasureHeight(60, 100); got != 7 {
		t.Errorf("input height = %d, want 7", got)
	}
	s.Open("", SearchScope{})
	s.TypeRunes([]rune("zzz"))
	// Placeholder + help + editor + three separators
	if got := searchInput(s, 60, 0).MeasureHeight(60, 100); got != 6 {
		t.Errorf("input height = %d, want 6", got)
	}
}

func TestSearchInputMeasuredHeightCapsAtFiveResults(t *testing.T) {
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = fmt.Sprintf("thief %d", i)
	}
	s := newTestSearch(lines...)
	s.Open("thief", SearchScope{})
	// Five results + help + editor + three separators
	if got := searchInput(s, 60, 0).MeasureHeight(60, 100); got != 10 {
		t.Errorf("input height = %d, want 10", got)
	}
}

func TestSearchSeqStableAcrossEviction(t *testing.T) {
	buf := NewScrollbackBuffer(5)
	buf.Append("the thief")
	for i := 0; i < 4; i++ {
		buf.Append("filler")
	}
	s := NewSearch(buf, style.DefaultStyles())
	s.Open("thief", SearchScope{})
	m, ok := s.Selected()
	if !ok {
		t.Fatal("expected a match")
	}
	// Evict the matched row: its seq no longer resolves, matches are a
	// snapshot and simply point at a gone row.
	buf.Append("evictor")
	if _, ok := buf.IndexOf(m.Seq); ok {
		t.Error("matched row should be evicted in this scenario")
	}
}
