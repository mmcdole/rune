package text

import "testing"

func TestStripANSI(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain text", "hello world", "hello world"},
		{"sgr color", "\x1b[31mred\x1b[0m text", "red text"},
		{"multi-param sgr", "\x1b[1;32;40mbold green\x1b[0m", "bold green"},
		{"cursor movement", "a\x1b[2Ab", "ab"},
		{"private-mode csi", "\x1b[?25htext", "text"},

		// Sequences a naive letter-terminated stripper gets wrong:
		{"csi tilde final", "\x1b[1~after", "after"},
		{"csi at-sign final", "\x1b[4@after", "after"},
		{"osc title bel", "\x1b]0;window title\x07visible", "visible"},
		{"osc title st", "\x1b]0;window title\x1b\\visible", "visible"},
		{"dcs string", "\x1bPsome payload\x1b\\visible", "visible"},
		{"charset designation", "\x1b(Btext", "text"},
		{"two-char escape", "\x1b7saved\x1b8", "saved"},

		{"c0 inside csi executes", "\x1b[31\tmtext", "\ttext"},
		{"bare esc at end", "text\x1b", "text"},
		{"unterminated csi at end", "text\x1b[31", "text"},
		{"esc restarts esc", "\x1b\x1b[31mred", "red"},
		{"utf8 preserved", "\x1b[36m→ östlich\x1b[0m", "→ östlich"},
		{"empty", "", ""},
		{"only escape", "\x1b[0m", ""},
		{"back to back", "\x1b[1m\x1b[31mX\x1b[0m", "X"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := StripANSI(tc.in); got != tc.want {
				t.Errorf("StripANSI(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestHasContent(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"empty", "", false},
		{"text", "HP> ", true},
		{"space", " ", true},
		{"tab", "\t", true},
		{"sgr reset", "\x1b[0m", false},
		{"sgr run", "\x1b[0m\x1b[1;31m", false},
		{"text after sgr", "\x1b[31mred", true},
		{"cursor and clear", "\x1b[2J\x1b[H", false},
		{"osc title", "\x1b]0;title\x07", false},
		{"unterminated csi", "\x1b[0", false},
		{"bare esc", "\x1b", false},
		{"bell", "\x07", false},
		{"utf8 after sgr", "\x1b[0m→", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasContent(tc.in); got != tc.want {
				t.Errorf("HasContent(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestNewLine(t *testing.T) {
	l := NewLine("\x1b[32mYou see a rat.\x1b[0m")
	if l.Raw != "\x1b[32mYou see a rat.\x1b[0m" {
		t.Errorf("Raw altered: %q", l.Raw)
	}
	if l.Clean != "You see a rat." {
		t.Errorf("Clean = %q", l.Clean)
	}
}
