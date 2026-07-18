package util

import (
	"slices"
	"testing"
)

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
			if VisibleLen(r) > 5 {
				t.Errorf("row %d exceeds width: %q (%d cols)", i, r, VisibleLen(r))
			}
		}
	})
}
