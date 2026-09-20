package widget

import (
	"testing"

	"charm.land/lipgloss/v2"
)

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
			s := NewSeparator(tc.char, lipgloss.NewStyle())
			s.SetSize(tc.width, 1)
			if got := s.View(); got != tc.want {
				t.Errorf("View() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSeparatorUsesSuppliedStyle(t *testing.T) {
	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	s := NewSeparator("═", borderStyle)
	s.SetSize(3, 1)
	if got, want := s.View(), borderStyle.Render("═══"); got != want {
		t.Fatalf("View() = %q, want supplied style %q", got, want)
	}
}
