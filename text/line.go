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

// StripANSI removes ANSI escape codes from a string.
func StripANSI(s string) string {
	var result strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}
