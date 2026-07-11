package text

import (
	"testing"
	"unicode/utf8"
)

func TestVisualizeTerminalControls(t *testing.T) {
	input := "safe\tline\n\x1b[31m\x00\x7f\u0085\u2028\u202e"
	if got, want := VisualizeTerminalControls(input, false), "safe␉line␊␛[31m␀␡���"; got != want {
		t.Fatalf("VisualizeTerminalControls = %q, want %q", got, want)
	}
	if got, want := VisualizeTerminalControls(input, true), "safe\tline␊␛[31m␀␡���"; got != want {
		t.Fatalf("VisualizeTerminalControls(preserve tabs) = %q, want %q", got, want)
	}
	projected := VisualizeTerminalControls(input, false)
	if utf8.RuneCountInString(projected) != utf8.RuneCountInString(input) {
		t.Fatalf("projection changed rune cardinality: %q -> %q", input, projected)
	}
	if twice := VisualizeTerminalControls(projected, false); twice != projected {
		t.Fatalf("projection is not idempotent: %q -> %q", projected, twice)
	}
	ordinary := "café e\u0301 👩‍💻"
	if got := VisualizeTerminalControls(ordinary, false); got != ordinary {
		t.Fatalf("projection changed ordinary Unicode: %q -> %q", ordinary, got)
	}
}
