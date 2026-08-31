package widget

import "testing"

// The separator's contract: anything that is not exactly one display cell
// falls back to the default so the rule always spans its assigned width.
func TestSeparatorChar(t *testing.T) {
	cases := []struct {
		name  string
		char  string
		width int
		want  string
	}{
		{"default", "", 5, "─────"},
		{"double rule", "═", 5, "═════"},
		{"ascii equals", "=", 3, "==="},
		{"wide char falls back", "全", 4, "────"},
		{"multi-char falls back", "==", 4, "────"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewSeparator()
			s.SetChar(tc.char)
			s.SetSize(tc.width, 1)
			want := "\x1b[90m" + tc.want + "\x1b[0m"
			if got := s.View(); got != want {
				t.Errorf("View() = %q, want %q", got, want)
			}
		})
	}
}

func TestSeparatorCharCanResetToDefault(t *testing.T) {
	s := NewSeparator()
	s.SetSize(3, 1)

	s.SetChar("═")
	if got, want := s.View(), "\x1b[90m═══\x1b[0m"; got != want {
		t.Fatalf("configured View() = %q, want %q", got, want)
	}

	s.SetChar("")
	if got, want := s.View(), "\x1b[90m───\x1b[0m"; got != want {
		t.Errorf("reset View() = %q, want %q", got, want)
	}
}
