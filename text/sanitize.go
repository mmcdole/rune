package text

import "strings"

// SanitizeDisplay makes server text safe for a row-based display: it
// keeps printable text and SGR color sequences (CSI ... 'm' with plain
// numeric parameters) and drops every other escape sequence and
// terminal-active control character.
//
// MUD-side editors (e.g. Aardwolf's OLC editor) send clear-screen and
// cursor-addressing sequences. Rune's scrollback is an append-only row
// model, so replaying such a sequence from inside a rendered row would
// wipe UI chrome (separators, input line, bars) instead of "clearing
// the screen"; like CMUD and MUSHclient, Rune ignores them. Tabs, CR,
// and LF pass through - later display stages own tab expansion and
// line splitting.
//
// The state machine mirrors StripANSI (line.go), which owns the notes
// on why each sequence class is parsed the way it is; this variant
// re-emits SGR sequences instead of dropping them.
func SanitizeDisplay(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	state := stText
	var params strings.Builder // parameter bytes of the CSI being scanned
	sgr := false               // CSI still qualifies as plain SGR
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch state {
		case stText:
			switch {
			case c == 0x1b:
				state = stEsc
			case c == '\t' || c == '\r' || c == '\n':
				b.WriteByte(c)
			case c < 0x20 || c == 0x7f:
				// Terminal-active C0 control (BEL, BS, FF, ...): drop
			default:
				b.WriteByte(c)
			}

		case stEsc:
			switch {
			case c == '[':
				state = stCSI
				params.Reset()
				sgr = true
			case c == ']' || c == 'P' || c == 'X' || c == '^' || c == '_':
				state = stString
			case c == 0x1b:
				// ESC ESC: restart sequence detection
			case c >= 0x20 && c <= 0x2f:
				// Intermediate byte: stay until the final byte arrives
			default:
				// Two-character escape (ESC c, ESC 7, ...): dropped
				state = stText
			}

		case stCSI:
			switch {
			case c >= 0x40 && c <= 0x7e:
				// Final byte. Only a plain SGR survives: private-mode
				// markers, intermediates, and embedded controls all
				// disqualify (e.g. "CSI > 4 m" is XTMODKEYS, not color).
				if c == 'm' && sgr {
					b.WriteString("\x1b[")
					b.WriteString(params.String())
					b.WriteByte('m')
				}
				state = stText
			case c == 0x1b:
				state = stEsc
			case c >= '0' && c <= '9' || c == ';' || c == ':':
				params.WriteByte(c)
			default:
				// Non-SGR parameter/intermediate byte, or an embedded
				// C0 control: the sequence is not a color
				sgr = false
			}

		case stString:
			switch c {
			case 0x07:
				state = stText
			case 0x1b:
				state = stStringEsc
			}

		case stStringEsc:
			switch c {
			case '\\':
				state = stText
			case 0x1b:
				// Still a candidate ST terminator
			default:
				state = stString
			}
		}
	}

	return b.String()
}
