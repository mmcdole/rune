package widget

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui/tui/style"
	"github.com/mmcdole/rune/ui/tui/util"
)

// searchPageSize bounds the matches materialized on either side of the
// active search anchor. Reaching a loaded edge scans another page, so a
// common query cannot make typing allocate matches for the full 100k-row
// buffer and the cap never makes older history unreachable.
const searchPageSize = 250

// searchMaxVisible is the maximum number of match rows shown at once.
const searchMaxVisible = 5

// SearchScope fixes the temporal frame for one search session. OriginSeq is
// the viewport position from which a new query starts; ResumeSeq optionally
// identifies the previously committed result when reopening a preserved
// query.
type SearchScope struct {
	OriginSeq uint64
	OriginSet bool
	ResumeSeq uint64
	ResumeSet bool
}

// Search is the scrollback-search navigator: a query line plus a small
// chronological window of matching rows. It owns query, match paging, and
// selection state; Model owns the resulting viewport position.
type Search struct {
	buf        *ScrollbackBuffer
	styles     style.Styles
	query      string
	pristine   bool // reopened with a preserved query: first typed rune replaces it
	matches    []SearchMatch
	selected   int // index into chronological matches
	scrollOff  int
	olderMore  bool
	newerMore  bool
	originSeq  uint64
	originSet  bool
	frozenTail uint64
	frozenSet  bool
	notice     string
}

// NewSearch creates a search overlay over the given buffer.
func NewSearch(buf *ScrollbackBuffer, styles style.Styles) *Search {
	return &Search{buf: buf, styles: styles}
}

// Open (re)opens the overlay over a frozen scrollback snapshot. An empty
// query keeps the previous one; either way the first typed rune starts a
// fresh query.
func (s *Search) Open(query string, scope SearchScope) {
	preservingQuery := query == ""
	if query != "" {
		s.query = query
	}
	if s.buf.Count() > 0 {
		s.frozenTail = s.buf.Seq(s.buf.Count() - 1)
		s.frozenSet = true
	} else {
		s.frozenSet = false
	}
	if scope.OriginSet {
		s.originSeq = scope.OriginSeq
		s.originSet = true
	} else if s.frozenSet {
		s.originSeq = s.frozenTail
		s.originSet = true
	} else {
		s.originSet = false
	}
	s.pristine = true
	anchor, anchorSet := s.originSeq, s.originSet
	if preservingQuery && scope.ResumeSet {
		anchor, anchorSet = scope.ResumeSeq, true
	}
	s.rescan(anchor, anchorSet)
}

// Reopen updates an already-visible navigator without replacing its cancel
// snapshot or admitting rows appended after the session opened.
func (s *Search) Reopen(query string) {
	preservingQuery := query == ""
	if query != "" {
		s.query = query
	}
	s.pristine = true
	anchor, anchorSet := s.originSeq, s.originSet
	if preservingQuery {
		if selected, ok := s.Selected(); ok {
			anchor, anchorSet = selected.Seq, true
		}
	}
	s.rescan(anchor, anchorSet)
}

// Query returns the current query.
func (s *Search) Query() string {
	return s.query
}

// TypeRunes appends typed runes to the query, replacing a preserved
// query on the first edit after Open.
func (s *Search) TypeRunes(rs []rune) {
	selected, selectedSet := s.Selected()
	if s.pristine {
		s.query = ""
		s.pristine = false
		selectedSet = false
	}
	s.query += string(rs)
	if selectedSet {
		s.rescan(selected.Seq, true)
	} else {
		s.rescan(s.originSeq, s.originSet)
	}
}

// Backspace deletes the last rune of the query.
func (s *Search) Backspace() {
	selected, selectedSet := s.Selected()
	s.pristine = false
	if s.query == "" {
		return
	}
	_, sz := utf8.DecodeLastRuneInString(s.query)
	s.query = s.query[:len(s.query)-sz]
	if selectedSet {
		s.rescan(selected.Seq, true)
	} else {
		s.rescan(s.originSeq, s.originSet)
	}
}

func (s *Search) rescan(anchor uint64, anchorSet bool) {
	s.matches = nil
	s.olderMore = false
	s.newerMore = false
	s.selected = 0
	s.scrollOff = 0
	s.notice = ""
	if s.query == "" || !anchorSet || !s.frozenSet {
		return
	}

	match := substringMatcher(s.query)
	older, olderMore := scanOlder(s.buf, match, anchor, searchPageSize)
	newer, newerMore := scanNewer(s.buf, match, anchor, s.frozenTail, searchPageSize)
	s.matches = append(older, newer...)
	s.olderMore = olderMore
	s.newerMore = newerMore

	// Prefer the exact anchor when it still matches. Otherwise start at the
	// closest older match, falling forward only when none exists.
	for i := range s.matches {
		if s.matches[i].Seq == anchor {
			s.selected = i
			s.adjustScroll()
			return
		}
	}
	if len(older) > 0 {
		s.selected = len(older) - 1
	}
	s.adjustScroll()
}

// SelectOlder moves toward earlier scrollback, loading another result page
// on demand. It stops rather than wrapping at the oldest match.
func (s *Search) SelectOlder() {
	if len(s.matches) == 0 {
		return
	}
	s.notice = ""
	if s.selected > 0 {
		s.selected--
		s.adjustScroll()
		return
	}
	if !s.olderMore || s.matches[0].Seq == 0 {
		s.notice = "Oldest match"
		return
	}
	page, more := scanOlder(s.buf, substringMatcher(s.query), s.matches[0].Seq-1, searchPageSize)
	if len(page) == 0 {
		s.olderMore = false
		s.notice = "Oldest match"
		return
	}
	s.matches = append(page, s.matches...)
	s.selected = len(page) - 1
	s.olderMore = more
	s.adjustScroll()
}

// SelectNewer moves toward the live tail, loading another result page on
// demand. It stops rather than wrapping at the newest match.
func (s *Search) SelectNewer() {
	if len(s.matches) == 0 {
		return
	}
	s.notice = ""
	if s.selected < len(s.matches)-1 {
		s.selected++
		s.adjustScroll()
		return
	}
	if !s.newerMore {
		s.notice = "Newest match"
		return
	}
	page, more := scanNewer(s.buf, substringMatcher(s.query), s.matches[len(s.matches)-1].Seq, s.frozenTail, searchPageSize)
	if len(page) == 0 {
		s.newerMore = false
		s.notice = "Newest match"
		return
	}
	s.matches = append(s.matches, page...)
	s.selected++
	s.newerMore = more
	s.adjustScroll()
}

func (s *Search) adjustScroll() {
	visible := min(searchMaxVisible, len(s.matches))
	if visible == 0 {
		s.scrollOff = 0
		return
	}
	s.scrollOff = s.selected - visible/2
	if s.scrollOff < 0 {
		s.scrollOff = 0
	}
	if maxOffset := len(s.matches) - visible; s.scrollOff > maxOffset {
		s.scrollOff = maxOffset
	}
}

// Selected returns the currently selected match.
func (s *Search) Selected() (SearchMatch, bool) {
	if len(s.matches) == 0 || s.selected < 0 || s.selected >= len(s.matches) {
		return SearchMatch{}, false
	}
	return s.matches[s.selected], true
}

// resultHeight measures matching rows only; Input owns query, help, and separators.
func (s *Search) resultHeight() int {
	return max(1, min(len(s.matches), searchMaxVisible))
}

func (s *Search) resultLines(width, limit int) []string {
	if limit <= 0 {
		return nil
	}
	if len(s.matches) == 0 {
		empty := "  " + "No matches"
		return []string{clipRow(s.styles.Muted.Render(empty), width)}
	}
	visible := min(limit, len(s.matches))
	start := s.scrollOff
	// scrollOff tracks the normal result window. A constrained layout can
	// offer fewer rows, so tighten that window around the current selection
	// instead of rendering a different match from the one being previewed.
	if s.selected < start {
		start = s.selected
	} else if s.selected >= start+visible {
		start = s.selected - visible + 1
	}
	start = max(0, min(start, len(s.matches)-visible))
	end := min(start+limit, len(s.matches))
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		lines = append(lines, s.renderMatch(s.matches[i], width, i == s.selected))
	}
	return lines
}

// queryLine renders "Search: query█        cur/total", count right-aligned.
func (s *Search) queryLine(width int) string {
	count := "0/0"
	if len(s.matches) > 0 {
		if s.olderMore || s.newerMore {
			count = fmt.Sprintf("%d+ matches", len(s.matches))
		} else {
			count = fmt.Sprintf("%d/%d", s.selected+1, len(s.matches))
		}
	}

	label := "Search: "
	query := text.VisualizeTerminalControls(s.query, false)
	// Keep the insertion point visible before spending cells on decoration.
	if width <= len(label)+1 {
		label = ""
	}
	queryWidth := max(0, width-len(label)-1)
	query = ansi.TruncateLeft(query, max(0, ansi.StringWidth(query)-queryWidth), "")
	left := s.styles.Muted.Render(label) + query + "█"
	leftWidth := len(label) + util.VisibleLen(query) + 1

	pad := width - leftWidth - len(count)
	if pad < 1 {
		pad = 1
	}
	return clipRow(left+strings.Repeat(" ", pad)+s.styles.Muted.Render(count), width)
}

func (s *Search) footerLine(width int) string {
	help := "↑ older  ↓ newer  Enter keep  Esc cancel"
	if s.notice != "" {
		help = s.notice + "  ·  " + help
	}
	return clipRow(s.styles.Muted.Render(help), width)
}

// renderMatch renders one match row, highlighting every occurrence.
// Byte runs index into Stripped; render walks runes tracking their
// byte offsets, so multi-byte text cannot misalign the highlight.
func (s *Search) renderMatch(m SearchMatch, width int, selected bool) string {
	prefix := "  "
	if selected {
		prefix = "> "
	}

	var b strings.Builder
	runIdx := 0
	for off, r := range m.Stripped {
		for runIdx < len(m.ByteRuns) && off >= m.ByteRuns[runIdx][1] {
			runIdx++
		}
		inMatch := runIdx < len(m.ByteRuns) &&
			off >= m.ByteRuns[runIdx][0] && off < m.ByteRuns[runIdx][1]

		ch := text.VisualizeTerminalControls(string(r), false)
		switch {
		case inMatch && selected:
			b.WriteString(s.styles.OverlayMatchSelected.Render(ch))
		case inMatch:
			b.WriteString(s.styles.OverlayMatch.Render(ch))
		case selected:
			b.WriteString(s.styles.OverlaySelected.Render(ch))
		default:
			b.WriteString(s.styles.OverlayNormal.Render(ch))
		}
	}

	var prefixStyled string
	if selected {
		prefixStyled = s.styles.OverlaySelected.Render(prefix)
	} else {
		prefixStyled = s.styles.OverlayNormal.Render(prefix)
	}
	return clipRow(prefixStyled+b.String(), max(1, width))
}
