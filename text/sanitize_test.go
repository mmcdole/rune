package text

import "testing"

func TestSanitizeDisplay(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain text", "hello world", "hello world"},
		{"sgr kept", "\x1b[31mred\x1b[0m text", "\x1b[31mred\x1b[0m text"},
		{"multi-param sgr kept", "\x1b[1;32;40mbold green\x1b[0m", "\x1b[1;32;40mbold green\x1b[0m"},
		{"empty sgr kept", "\x1b[mreset", "\x1b[mreset"},
		{"colon sgr kept", "\x1b[38:2::255:0:0mtruecolor\x1b[0m", "\x1b[38:2::255:0:0mtruecolor\x1b[0m"},

		// The Aardwolf editor-clear repertoire and friends.
		{"clear screen", "\x1b[2Jtext", "text"},
		{"cursor home", "\x1b[Htext", "text"},
		{"home then clear", "\x1b[H\x1b[2Jtext", "text"},
		{"erase below", "before\x1b[Jafter", "beforeafter"},
		{"erase line", "before\x1b[Kafter", "beforeafter"},
		{"cursor addressing", "\x1b[3;1Htext", "text"},
		{"scrollback clear", "\x1b[3Jtext", "text"},
		{"full reset", "text\x1bcmore", "textmore"},
		{"cursor movement", "a\x1b[2Ab", "ab"},
		{"private-mode csi", "\x1b[?25htext", "text"},

		// Non-color sequences that end in 'm' must not slip through.
		{"xtmodkeys not sgr", "\x1b[>4;1mtext", "text"},
		{"private marker with m final", "\x1b[?4mtext", "text"},
		{"intermediate with m final", "\x1b[0 mtext", "text"},

		// String sequences and charset designations drop cleanly.
		{"osc title bel", "\x1b]0;window title\x07visible", "visible"},
		{"osc title st", "\x1b]0;window title\x1b\\visible", "visible"},
		{"dcs string", "\x1bPsome payload\x1b\\visible", "visible"},
		{"charset designation", "\x1b(Btext", "text"},
		{"two-char escape", "\x1b7saved\x1b8", "saved"},

		// C0 policy: structural whitespace passes, terminal-active drops.
		{"tab kept", "a\tb", "a\tb"},
		{"crlf kept", "a\r\nb", "a\r\nb"},
		{"bel dropped", "ding\x07dong", "dingdong"},
		{"backspace dropped", "ab\x08c", "abc"},
		{"form feed dropped", "a\x0cb", "ab"},
		{"del dropped", "a\x7fb", "ab"},
		{"c0 inside csi dropped with sequence", "\x1b[31\x07mtext", "text"},

		// Malformed input stays inert.
		{"bare esc at end", "text\x1b", "text"},
		{"unterminated csi at end", "text\x1b[31", "text"},
		{"esc restarts esc", "\x1b\x1b[31mred", "\x1b[31mred"},
		{"esc inside csi restarts", "\x1b[3\x1b[32mgreen", "\x1b[32mgreen"},
		{"utf8 preserved", "\x1b[36m→ östlich\x1b[0m", "\x1b[36m→ östlich\x1b[0m"},
		{"empty", "", ""},

		{"editor burst", "\x1b[H\x1b[2J\x1b[3;1H\x1b[Jedit \x1b[31mred\x1b[0m\x1b[K done", "edit \x1b[31mred\x1b[0m done"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SanitizeDisplay(tc.in); got != tc.want {
				t.Errorf("SanitizeDisplay(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
