package util

import "testing"

// tag makes expectations readable: the "style" wraps the match in
// visible markers instead of escape codes.
func tag(s string) string {
	return "<H>" + s + "</H>"
}

func TestHighlightRange(t *testing.T) {
	tests := []struct {
		name       string
		row        string
		start, end int
		want       string
	}{
		{
			name:  "plain ascii",
			row:   "the thief runs",
			start: 4, end: 9,
			want: "the \x1b[0m<H>thief</H> runs",
		},
		{
			name:  "match at column zero",
			row:   "thief!",
			start: 0, end: 5,
			want: "\x1b[0m<H>thief</H>!",
		},
		{
			name:  "match at end of row",
			row:   "a thief",
			start: 2, end: 7,
			want: "a \x1b[0m<H>thief</H>",
		},
		{
			// The suffix must re-emit the ambient red so text after the
			// match keeps its original color. ansi.Truncate also carries
			// every later escape into the prefix (the row's trailing
			// reset here) - a harmless doubled reset, encoded exactly.
			name:  "match inside ambient sgr span",
			row:   "\x1b[31mred thief red\x1b[0m",
			start: 4, end: 9,
			want: "\x1b[31mred \x1b[0m\x1b[0m<H>thief</H>\x1b[31m red\x1b[0m",
		},
		{
			// The match's own styling is stripped for render; the suffix
			// replays the escapes seen so far (net reset), so " here"
			// stays unstyled as in the original.
			name:  "styled match text",
			row:   "a \x1b[32mthief\x1b[0m here",
			start: 2, end: 7,
			want: "a \x1b[32m\x1b[0m\x1b[0m<H>thief</H>\x1b[32m\x1b[0m here",
		},
		{
			// Wide runes before the match: columns, not bytes or runes.
			name:  "wide runes before match",
			row:   "日本 thief",
			start: 5, end: 10,
			want: "日本 \x1b[0m<H>thief</H>",
		},
		{
			name:  "range clamped past row width",
			row:   "short",
			start: 2, end: 99,
			want: "sh\x1b[0m<H>ort</H>",
		},
		{
			name:  "empty range is a no-op",
			row:   "unchanged",
			start: 3, end: 3,
			want: "unchanged",
		},
		{
			name:  "inverted range is a no-op",
			row:   "unchanged",
			start: 5, end: 2,
			want: "unchanged",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HighlightRange(tt.row, tt.start, tt.end, tag)
			if got != tt.want {
				t.Errorf("HighlightRange(%q, %d, %d)\n got %q\nwant %q",
					tt.row, tt.start, tt.end, got, tt.want)
			}
		})
	}
}

func TestHighlightRanges(t *testing.T) {
	tests := []struct {
		name   string
		row    string
		ranges []ColRange
		want   string
	}{
		{
			// The doubled reset before the first match is the rightmost
			// splice's injected reset carried into the second splice's
			// prefix by ansi.Truncate.
			name:   "two occurrences in one row",
			row:    "The thief robbed another thief.",
			ranges: []ColRange{{4, 9}, {25, 30}},
			want:   "The \x1b[0m\x1b[0m<H>thief</H> robbed another \x1b[0m<H>thief</H>.",
		},
		{
			// Matches at the exact row edges: ansi.Cut drops the escapes
			// outside the cut (the leading 31m and trailing reset). Safe
			// because the real render (lipgloss) always ends in its own
			// reset; the text between matches keeps its ambient red.
			name:   "two occurrences inside ambient sgr span",
			row:    "\x1b[31mthief and thief\x1b[0m",
			ranges: []ColRange{{0, 5}, {10, 15}},
			want:   "\x1b[0m<H>thief</H>\x1b[31m and \x1b[0m\x1b[0m<H>thief</H>",
		},
		{
			name:   "adjacent ranges",
			row:    "aabb",
			ranges: []ColRange{{0, 2}, {2, 4}},
			want:   "\x1b[0m<H>aa</H>\x1b[0m<H>bb</H>",
		},
		{
			name:   "no ranges is a no-op",
			row:    "unchanged",
			ranges: nil,
			want:   "unchanged",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HighlightRanges(tt.row, tt.ranges, tag)
			if got != tt.want {
				t.Errorf("HighlightRanges(%q, %v)\n got %q\nwant %q",
					tt.row, tt.ranges, got, tt.want)
			}
		})
	}
}
