package util

// FindWordBoundaries finds the start and end of the word at or before cursor.
// Returns (start, end) indices into the text string.
func FindWordBoundaries(text string, cursor int) (int, int) {
	if cursor > len(text) {
		cursor = len(text)
	}

	if cursor == 0 {
		return 0, 0
	}

	// Check if we're right after a space (no word at cursor)
	if text[cursor-1] == ' ' {
		return cursor, cursor
	}

	// Scan back for word start
	start := cursor
	for start > 0 && text[start-1] != ' ' {
		start--
	}

	// Scan forward for word end (from cursor)
	end := cursor
	for end < len(text) && text[end] != ' ' {
		end++
	}

	return start, end
}
