package widget

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui/tui/util"
)

// rowMatcher reports all non-overlapping occurrences in a stripped row
// as byte-offset pairs, left to right; empty means no match.
type rowMatcher func(stripped string) [][2]int

// SearchMatch is one matching scrollback row. The result unit is the
// row: every occurrence in it is carried (and highlighted), but
// navigation steps rows, since stepping between occurrences of the
// same row would re-center the same text.
type SearchMatch struct {
	Seq      uint64          // absolute row sequence number
	Ranges   []util.ColRange // occurrences as visible columns (viewport splice)
	ByteRuns [][2]int        // occurrences as byte offsets into Stripped (list render)
	Stripped string          // ANSI-stripped row text
}

// substringMatcher builds the substring matcher: case-insensitive plain
// substring. ASCII queries scan the row bytes directly (no allocation,
// offsets stay valid); non-ASCII queries fall back to a rune-wise
// simple-fold walk.
func substringMatcher(query string) rowMatcher {
	if query == "" {
		return func(string) [][2]int { return nil }
	}
	if isASCII(query) {
		lower := strings.ToLower(query)
		return func(stripped string) [][2]int {
			return asciiFoldRanges(stripped, lower)
		}
	}
	return func(stripped string) [][2]int {
		return foldRanges(stripped, query)
	}
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// asciiFoldRanges finds non-overlapping occurrences of an
// already-lowercased ASCII query, case-insensitively on ASCII letters.
func asciiFoldRanges(s, lowerQuery string) [][2]int {
	var ranges [][2]int
	n := len(lowerQuery)
	for i := 0; i+n <= len(s); {
		if asciiFoldPrefix(s[i:], lowerQuery) {
			ranges = append(ranges, [2]int{i, i + n})
			i += n
		} else {
			i++
		}
	}
	return ranges
}

func asciiFoldPrefix(s, lowerQuery string) bool {
	for j := 0; j < len(lowerQuery); j++ {
		c := s[j]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != lowerQuery[j] {
			return false
		}
	}
	return true
}

// foldRanges finds non-overlapping occurrences of query under Unicode
// simple case-folding, walking rune by rune.
func foldRanges(s, query string) [][2]int {
	var ranges [][2]int
	for i := 0; i < len(s); {
		if n, ok := foldPrefix(s[i:], query); ok {
			ranges = append(ranges, [2]int{i, i + n})
			i += n
		} else {
			_, sz := utf8.DecodeRuneInString(s[i:])
			i += sz
		}
	}
	return ranges
}

// foldPrefix reports whether s begins with query under simple case
// folding, returning the byte length of the matched prefix in s.
func foldPrefix(s, query string) (int, bool) {
	i := 0
	for _, qr := range query {
		if i >= len(s) {
			return 0, false
		}
		sr, sz := utf8.DecodeRuneInString(s[i:])
		if !foldEq(sr, qr) {
			return 0, false
		}
		i += sz
	}
	return i, true
}

// foldEq reports whether two runes are equal under Unicode simple
// case folding.
func foldEq(a, b rune) bool {
	if a == b {
		return true
	}
	for r := unicode.SimpleFold(a); r != a; r = unicode.SimpleFold(r) {
		if r == b {
			return true
		}
	}
	return false
}

func matchBufferRow(buf *ScrollbackBuffer, index int, match rowMatcher) (SearchMatch, bool) {
	row := buf.At(index)
	stripped := row
	if strings.IndexByte(row, 0x1b) >= 0 {
		stripped = text.StripANSI(row)
	}
	runs := match(stripped)
	if len(runs) == 0 {
		return SearchMatch{}, false
	}

	m := SearchMatch{Seq: buf.Seq(index), ByteRuns: runs, Stripped: stripped}
	for _, r := range runs {
		// Columns measured the same way the viewport splice cuts
		// (ansi.StringWidth), so wide runes stay aligned.
		start := ansi.StringWidth(stripped[:r[0]])
		end := start + ansi.StringWidth(stripped[r[0]:r[1]])
		m.Ranges = append(m.Ranges, util.ColRange{Start: start, End: end})
	}
	return m, true
}

func indexAtOrBefore(buf *ScrollbackBuffer, seq uint64) int {
	if buf.Count() == 0 || seq < buf.Seq(0) {
		return -1
	}
	newest := buf.Count() - 1
	if seq >= buf.Seq(newest) {
		return newest
	}
	index, ok := buf.IndexOf(seq)
	if !ok {
		return -1
	}
	return index
}

// scanOlder collects the nearest matches at-or-older-than throughSeq and
// returns them in chronological order. more reports a confirmed additional
// match beyond the returned page, not merely unscanned buffer rows.
func scanOlder(buf *ScrollbackBuffer, match rowMatcher, throughSeq uint64, limit int) (matches []SearchMatch, more bool) {
	for i := indexAtOrBefore(buf, throughSeq); i >= 0; i-- {
		m, ok := matchBufferRow(buf, i, match)
		if !ok {
			continue
		}
		if len(matches) == limit {
			more = true
			break
		}
		matches = append(matches, m)
	}
	// The scan walks backward for speed; the navigator renders the page in
	// the same oldest-to-newest order as the scrollback itself.
	for left, right := 0, len(matches)-1; left < right; left, right = left+1, right-1 {
		matches[left], matches[right] = matches[right], matches[left]
	}
	return matches, more
}

// scanNewer collects the nearest matches strictly newer than afterSeq,
// stopping at the frozen search-session tail.
func scanNewer(buf *ScrollbackBuffer, match rowMatcher, afterSeq, throughSeq uint64, limit int) (matches []SearchMatch, more bool) {
	end := indexAtOrBefore(buf, throughSeq)
	if end < 0 {
		return nil, false
	}
	start := 0
	if afterSeq >= buf.Seq(0) {
		index := indexAtOrBefore(buf, afterSeq)
		if index < 0 {
			return nil, false
		}
		start = index + 1
	}
	for i := start; i <= end; i++ {
		m, ok := matchBufferRow(buf, i, match)
		if !ok {
			continue
		}
		if len(matches) == limit {
			return matches, true
		}
		matches = append(matches, m)
	}
	return matches, false
}
