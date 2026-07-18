package input

import "unicode/utf8"

// ClampByteCursor clamps pos to text and snaps it to a UTF-8 rune boundary.
func ClampByteCursor(text string, pos int) int {
	if pos < 0 {
		return 0
	}
	if pos > len(text) {
		return len(text)
	}
	for pos > 0 && pos < len(text) && !utf8.RuneStart(text[pos]) {
		pos--
	}
	return pos
}

// RuneCursorToByte converts a zero-based rune offset to a byte offset.
func RuneCursorToByte(text string, pos int) int {
	if pos <= 0 {
		return 0
	}
	for bytePos := range text {
		if pos == 0 {
			return bytePos
		}
		pos--
	}
	return len(text)
}

// ByteCursorToRune converts a zero-based byte offset to a rune offset.
// Positions inside a UTF-8 sequence snap to the preceding rune boundary.
func ByteCursorToRune(text string, pos int) int {
	pos = ClampByteCursor(text, pos)
	return utf8.RuneCountInString(text[:pos])
}
