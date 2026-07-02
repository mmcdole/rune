package text

import "strings"

// Line represents a server line with both raw (ANSI) and clean (stripped) versions.
type Line struct {
	Raw   string // Original line with ANSI codes
	Clean string // ANSI-stripped version
}

// NewLine creates a Line from raw text, automatically stripping ANSI codes.
func NewLine(raw string) Line {
	return Line{Raw: raw, Clean: StripANSI(raw)}
}

// Stripper states. ANSI sequences are pure ASCII, so the machine can
// operate on bytes; UTF-8 continuation bytes (>= 0x80) only appear in
// text state and pass through untouched.
const (
	stText      = iota
	stEsc       // saw ESC, deciding sequence type
	stCSI       // in CSI (ESC [), consuming until final byte 0x40-0x7E
	stString    // in OSC/DCS/SOS/PM/APC string, consuming until BEL or ST
	stStringEsc // in string, saw ESC (possible ST = ESC \)
)

// StripANSI removes ANSI escape sequences from a string.
//
// Clean text is what triggers match against, so this must handle more
// than color codes: CSI sequences whose final byte is not a letter
// (e.g. "ESC[1~"), string sequences like OSC window titles (terminated
// by BEL or ESC \), and charset designations ("ESC ( B"). A naive
// "consume until a letter" stripper swallows real text after any of
// those.
func StripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	state := stText
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch state {
		case stText:
			if c == 0x1b {
				state = stEsc
			} else {
				b.WriteByte(c)
			}

		case stEsc:
			switch {
			case c == '[':
				state = stCSI
			case c == ']' || c == 'P' || c == 'X' || c == '^' || c == '_':
				// OSC, DCS, SOS, PM, APC: consumes until BEL or ST
				state = stString
			case c == 0x1b:
				// ESC ESC: restart sequence detection
			case c >= 0x20 && c <= 0x2f:
				// Intermediate byte (e.g. charset designation "ESC ( B"):
				// stay until the final byte arrives
			default:
				// Final byte of a two-character escape (ESC 7, ESC c, ...)
				// or a malformed sequence: either way the escape is over.
				state = stText
			}

		case stCSI:
			switch {
			case c >= 0x40 && c <= 0x7e:
				// Final byte terminates the sequence
				state = stText
			case c == 0x1b:
				// Malformed: a new escape starts mid-sequence
				state = stEsc
			case c < 0x20:
				// Terminals execute C0 controls embedded in CSI
				b.WriteByte(c)
			default:
				// Parameter (0x30-0x3F) or intermediate (0x20-0x2F) byte
			}

		case stString:
			switch c {
			case 0x07: // BEL terminates OSC
				state = stText
			case 0x1b:
				state = stStringEsc
			}

		case stStringEsc:
			switch c {
			case '\\': // ST (ESC \) terminates the string
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
