package input

import "testing"

func TestClampByteCursor(t *testing.T) {
	text := "café"
	tests := []struct {
		pos  int
		want int
	}{
		{-1, 0},
		{0, 0},
		{3, 3},
		{4, 3},
		{5, 5},
		{6, 5},
	}

	for _, tt := range tests {
		if got := ClampByteCursor(text, tt.pos); got != tt.want {
			t.Errorf("ClampByteCursor(%q, %d) = %d, want %d", text, tt.pos, got, tt.want)
		}
	}
}

func TestRuneCursorToByte(t *testing.T) {
	text := "café"
	tests := []struct {
		pos  int
		want int
	}{
		{-1, 0},
		{0, 0},
		{1, 1},
		{3, 3},
		{4, 5},
		{5, 5},
	}

	for _, tt := range tests {
		if got := RuneCursorToByte(text, tt.pos); got != tt.want {
			t.Errorf("RuneCursorToByte(%q, %d) = %d, want %d", text, tt.pos, got, tt.want)
		}
	}
}

func TestByteCursorToRune(t *testing.T) {
	text := "café"
	tests := []struct {
		pos  int
		want int
	}{
		{-1, 0},
		{0, 0},
		{3, 3},
		{4, 3},
		{5, 4},
		{6, 4},
	}

	for _, tt := range tests {
		if got := ByteCursorToRune(text, tt.pos); got != tt.want {
			t.Errorf("ByteCursorToRune(%q, %d) = %d, want %d", text, tt.pos, got, tt.want)
		}
	}
}
