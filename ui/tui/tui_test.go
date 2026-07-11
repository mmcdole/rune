package tui

import "testing"

func TestNormalizeEditorTextPreservesWhitespace(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "indentation and trailing spaces", in: "  first  \n\tsecond \n", want: "  first  \n\tsecond "},
		{name: "one blank line remains", in: "first\n\n", want: "first\n"},
		{name: "CRLF", in: "first\r\n\tsecond\r\n", want: "first\n\tsecond"},
		{name: "bare CR", in: "first\rsecond\r", want: "first\nsecond"},
		{name: "empty", in: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeEditorText(tt.in); got != tt.want {
				t.Fatalf("normalizeEditorText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
