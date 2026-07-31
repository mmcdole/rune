package widget

import "testing"

// The separator's contract: the rule character comes from the "char"
// option, and anything that isn't exactly one display cell falls back
// to the default so the rule always spans the width it was given.
func TestSeparatorCharOption(t *testing.T) {
	cases := []struct {
		name  string
		opts  map[string]string
		width int
		want  string
	}{
		{"no options", nil, 5, "─────"},
		{"double rule", map[string]string{"char": "═"}, 5, "═════"},
		{"ascii equals", map[string]string{"char": "="}, 3, "==="},
		{"unknown keys ignored", map[string]string{"color": "red"}, 3, "───"},
		{"wide char falls back", map[string]string{"char": "全"}, 4, "────"},
		{"multi-char falls back", map[string]string{"char": "=="}, 4, "────"},
		{"empty char falls back", map[string]string{"char": ""}, 4, "────"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewSeparator()
			s.SetOptions(tc.opts)
			s.SetSize(tc.width, 1)
			want := "\x1b[90m" + tc.want + "\x1b[0m"
			if got := s.View(); got != want {
				t.Errorf("View() = %q, want %q", got, want)
			}
		})
	}
}

// A later entry without options must reset a char set earlier in the
// same render pass — the widget instance is shared across entries.
func TestSeparatorOptionsResetBetweenEntries(t *testing.T) {
	s := NewSeparator()
	s.SetSize(3, 1)

	s.SetOptions(map[string]string{"char": "═"})
	if got, want := s.View(), "\x1b[90m═══\x1b[0m"; got != want {
		t.Fatalf("configured View() = %q, want %q", got, want)
	}

	s.SetOptions(nil)
	if got, want := s.View(), "\x1b[90m───\x1b[0m"; got != want {
		t.Errorf("reset View() = %q, want %q", got, want)
	}
}
