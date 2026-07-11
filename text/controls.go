package text

import (
	"strings"
	"unicode"
)

// VisualizeTerminalControls replaces terminal-active control characters with
// inert, visible glyphs. Each source rune produces exactly one output rune so
// callers such as fuzzy pickers can retain source-index mappings. Tabs may be
// preserved when a later, terminal-safe renderer will expand them to spaces.
func VisualizeTerminalControls(s string, preserveTabs bool) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		b.WriteRune(VisualizeTerminalRune(r, preserveTabs))
	}
	return b.String()
}

// VisualizeTerminalRune is the allocation-free single-rune form of
// VisualizeTerminalControls.
func VisualizeTerminalRune(r rune, preserveTabs bool) rune {
	if r == '\t' && preserveTabs {
		return r
	}
	switch {
	case r >= 0 && r <= 0x1f:
		return rune(0x2400) + r
	case r == 0x7f:
		return '␡'
	case RequiresTerminalProjection(r):
		return '�'
	default:
		return r
	}
}

// RequiresTerminalProjection reports whether r must be replaced or escaped
// before it is rendered directly in a terminal. It is also the shared
// admission rule for Rune's lossless verbatim input path, keeping editing and
// display policy from drifting apart.
func RequiresTerminalProjection(r rune) bool {
	return unicode.IsControl(r) ||
		unicode.Is(unicode.Zl, r) ||
		unicode.Is(unicode.Zp, r) ||
		isBidiControl(r)
}

func isBidiControl(r rune) bool {
	return r == '\u061c' || r == '\u200e' || r == '\u200f' ||
		(r >= '\u202a' && r <= '\u202e') ||
		(r >= '\u2066' && r <= '\u2069')
}
