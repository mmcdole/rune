package util

import (
	"slices"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestTerminalCellWidths(t *testing.T) {
	for _, value := range []string{"❤️", "1️⃣", "👩‍💻", "🇺🇸", "界", "\x1b[31m❤️\x1b[0m"} {
		if got := ansi.StringWidth(value); got != 2 {
			t.Errorf("ansi.StringWidth(%q) = %d, want 2", value, got)
		}
		if got := ExpandTabs(value + "\tx"); got != value+"      x" {
			t.Errorf("tab after %q: %q", value, got)
		}
		for _, row := range WrapLine(value+value+"x", 3) {
			if ansi.StringWidth(row) > 3 {
				t.Errorf("wrapped row exceeds renderer width: %q", row)
			}
		}
	}
}

func TestExpandTabs(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"NoTab", "plain line", "plain line"},
		{"LeadingTab", "\tindented", "        indented"},
		{"MidLine", "ab\tcd", "ab      cd"},
		{"AtStop", "12345678\tx", "12345678        x"},
		{"Consecutive", "\t\tx", "                x"},
		{"MultiStops", "a\tb\tc", "a       b       c"},
		// ANSI sequences are zero-width: the tab stop is computed from
		// visible cells, not byte position.
		{"ANSIColored", "\x1b[31mab\x1b[0m\tc", "\x1b[31mab\x1b[0m      c"},
		{"TabOnly", "\t", "        "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExpandTabs(tt.in); got != tt.want {
				t.Errorf("ExpandTabs(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"Single", "abc", []string{"abc"}},
		{"LF", "a\nb", []string{"a", "b"}},
		{"CRLF", "a\r\nb", []string{"a", "b"}},
		{"LoneCR", "a\rb", []string{"a", "b"}},
		{"Mixed", "a\rb\r\nc\nd", []string{"a", "b", "c", "d"}},
		{"TrailingLF", "a\n", []string{"a", ""}},
		{"Empty", "", []string{""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SplitLines(tt.in); !slices.Equal(got, tt.want) {
				t.Errorf("SplitLines(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestWrapLine(t *testing.T) {
	// 10 visible columns in 19 bytes: the byte-length fast path must
	// not decide for a styled line, only the visible width may.
	styled := "\x1b[31mxxxxxxxxxx\x1b[0m"
	tests := []struct {
		name  string
		line  string
		width int
		want  []string
	}{
		{"Fits", "abc", 10, []string{"abc"}},
		{"ExactFit", "abcde", 5, []string{"abcde"}},
		{"WidthUnknown", "aaaaaaaaaa", 0, []string{"aaaaaaaaaa"}},
		{"StyledFitsDespiteBytes", styled, 12, []string{styled}},
		{"WrapsAtSpace", "aaaa bbb", 5, []string{"aaaa", "bbb"}},
		{"HardBreaksWord", "aaaaaa", 4, []string{"aaaa", "aa"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WrapLine(tt.line, tt.width); !slices.Equal(got, tt.want) {
				t.Errorf("WrapLine(%q, %d) = %q, want %q", tt.line, tt.width, got, tt.want)
			}
		})
	}

	t.Run("StyledOverlongWrapsWithinWidth", func(t *testing.T) {
		rows := WrapLine("\x1b[31maaaa bbb\x1b[0m", 5)
		if len(rows) != 2 {
			t.Fatalf("expected 2 rows, got %d: %q", len(rows), rows)
		}
		for i, r := range rows {
			if ansi.StringWidth(r) > 5 {
				t.Errorf("row %d exceeds width: %q (%d cols)", i, r, ansi.StringWidth(r))
			}
		}
	})
}

func TestWrapSpans(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		width int
		want  []ColRange
	}{
		{"Fits", "short", 10, []ColRange{{0, 5}}},
		{"DropsBreakSpace", "hello world", 6, []ColRange{{0, 5}, {6, 11}}},
		{"DropsSpaceRun", "aaaa    bbbb", 6, []ColRange{{0, 4}, {8, 12}}},
		{"HardBreak", "abcdefghij", 4, []ColRange{{0, 4}, {4, 8}, {8, 10}}},
		{"Styled", "\x1b[31mhello \x1b[1mworld\x1b[0m", 6, []ColRange{{0, 5}, {6, 11}}},
		{"Wide", "界界界界", 4, []ColRange{{0, 4}, {4, 8}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WrapSpans(tt.line, tt.width)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("WrapSpans(%q, %d) = %v, want %v", tt.line, tt.width, got, tt.want)
			}
			// Each span cut from the line holds the same visible text as
			// the WrapLine row it describes.
			rows := WrapLine(tt.line, tt.width)
			for i, span := range got {
				cut := ansi.Strip(ansi.Cut(tt.line, span.Start, span.End))
				if want := ansi.Strip(rows[i]); cut != want {
					t.Errorf("span %d cuts %q, row is %q", i, cut, want)
				}
			}
		})
	}
}
