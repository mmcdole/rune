package util

import "testing"

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
